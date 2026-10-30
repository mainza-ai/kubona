// Package rates loads the rate-card contract. The embedded card is a
// bootstrap/fallback; production cloud pricing should use a runtime card
// supplied by the operator via KUBONA_RATE_CARD_PATH or KUBONA_RATE_CARD_URL.
//
// The card is data, never model IDs in code. Model IDs/rates can be replaced
// without rebuilding; a watcher reloads a file/URL source. A stale card is a
// hard error: a spend-control plane that prices from an expired guess is a
// fiction with a budget.
package rates

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

//go:embed rate-cards.json
var embedded []byte

// ErrStale means the card's effective window has elapsed.
var ErrStale = errors.New("rate card is stale")

type ModelRate struct {
	InputPer1KUSD  float64 `json:"input_per_1k_usd"`
	OutputPer1KUSD float64 `json:"output_per_1k_usd"`
	// Provider is the provider namespace this rate applies to (for example
	// "opencode" or "nvidia"). A provider-qualified key wins over a bare
	// model key when both exist.
	Provider string `json:"provider,omitempty"`
	// Status is "active" (default), "deprecated", or "retired". Retired
	// models are never priced; a caller must migrate or supply a live model.
	Status string `json:"status,omitempty"`
	// Aliases are alternate model IDs accepted by the provider (for example
	// legacy names that route to the current model).
	Aliases []string `json:"aliases,omitempty"`
	// Source and RetrievedAt are evidence for this row; never secrets.
	Source      string `json:"source,omitempty"`
	RetrievedAt string `json:"retrieved_at,omitempty"`
	// Confidence is "vendor" for a vendor list/invoice, "aggregator" for a
	// third-party catalog, or "heuristic". It does not change pricing.
	Confidence string `json:"confidence,omitempty"`
}

type LensRate struct {
	CostUSD     float64 `json:"cost_usd"`
	Unit        string  `json:"unit"`
	Provider    string  `json:"provider,omitempty"`
	Model       string  `json:"model,omitempty"`
	Status      string  `json:"status,omitempty"`
	Source      string  `json:"source,omitempty"`
	RetrievedAt string  `json:"retrieved_at,omitempty"`
}

type Card struct {
	Version        int                  `json:"version"`
	Source         string               `json:"source"`
	EffectiveDate  string               `json:"effective_date"`
	StaleAfterDays int                  `json:"stale_after_days"`
	Heuristic      bool                 `json:"heuristic"`
	Notes          string               `json:"notes"`
	Lens           map[string]LensRate  `json:"lens"`
	Models         map[string]ModelRate `json:"models"`
}

type state struct {
	card     *Card
	source   string
	loadedAt time.Time
	hash     string
}

var current atomic.Pointer[state]
var loadMu sync.Mutex

func init() {
	c, err := parseCard(embedded)
	if err != nil {
		panic(fmt.Sprintf("rates: embedded card is invalid JSON: %v", err))
	}
	current.Store(&state{card: c, source: "embedded", loadedAt: time.Now().UTC(),
		hash: digest(embedded)})
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func parseCard(data []byte) (*Card, error) {
	var c Card
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// Load atomically installs a parsed card. Validation is separate so callers
// can reject a stale/invalid update without replacing the last good card.
func Load(data []byte, source string) error {
	c, err := parseCard(data)
	if err != nil {
		return fmt.Errorf("rate card JSON: %w", err)
	}
	if source == "" {
		source = "runtime"
	}
	current.Store(&state{card: c, source: source, loadedAt: time.Now().UTC(), hash: digest(data)})
	return nil
}

// LoadFile reads and validates a card from an absolute path before replacing
// the active card. A failed load never clobbers the last good card.
func LoadFile(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	c, err := parseCard(raw)
	if err != nil {
		return fmt.Errorf("rate card JSON: %w", err)
	}
	if err := validateCard(c, time.Now().UTC()); err != nil {
		return err
	}
	current.Store(&state{card: c, source: "file:" + path, loadedAt: time.Now().UTC(), hash: digest(raw)})
	return nil
}

// LoadURL fetches and validates an HTTPS (or loopback HTTP) card. The body is
// bounded; redirects are followed by the default client but TLS is required
// off-loopback.
func LoadURL(ctx context.Context, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("rate card URL: %w", err)
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && isLoopbackHost(u.Hostname())) {
		return fmt.Errorf("rate card URL must be https (or loopback http), got %q", u.Scheme)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme == "https" || (req.URL.Scheme == "http" && isLoopbackHost(req.URL.Hostname())) {
			return nil
		}
		return fmt.Errorf("rate card redirect to non-HTTPS URL refused: %s", req.URL.Redacted())
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("rate card URL returned HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if len(raw) >= 1<<20 {
		return fmt.Errorf("rate card URL exceeds 1 MiB")
	}
	c, err := parseCard(raw)
	if err != nil {
		return fmt.Errorf("rate card JSON: %w", err)
	}
	if err := validateCard(c, time.Now().UTC()); err != nil {
		return err
	}
	current.Store(&state{card: c, source: "url:" + rawURL, loadedAt: time.Now().UTC(), hash: digest(raw)})
	return nil
}

func isLoopbackHost(h string) bool {
	h = strings.ToLower(strings.TrimSpace(h))
	return h == "localhost" || h == "127.0.0.1" || h == "::1" || h == "[::1]"
}

// ConfigureFromEnv loads the operator's runtime card. With neither env set,
// the embedded bootstrap remains active. Both set is ambiguous and refuses.
func ConfigureFromEnv(ctx context.Context) error {
	path := strings.TrimSpace(os.Getenv("KUBONA_RATE_CARD_PATH"))
	rawURL := strings.TrimSpace(os.Getenv("KUBONA_RATE_CARD_URL"))
	if path != "" && rawURL != "" {
		return fmt.Errorf("KUBONA_RATE_CARD_PATH and KUBONA_RATE_CARD_URL are mutually exclusive")
	}
	if path != "" && !filepath.IsAbs(path) {
		return fmt.Errorf("KUBONA_RATE_CARD_PATH must be absolute (CWD-relative rate cards refuse)")
	}
	if path == "" && rawURL == "" {
		slog.Warn("[rates] using embedded bootstrap card; production cloud pricing should set KUBONA_RATE_CARD_PATH or KUBONA_RATE_CARD_URL")
		return nil
	}
	if path != "" {
		if err := LoadFile(path); err != nil {
			return fmt.Errorf("KUBONA_RATE_CARD_PATH %s: %w", path, err)
		}
	} else {
		if err := LoadURL(ctx, rawURL); err != nil {
			return fmt.Errorf("KUBONA_RATE_CARD_URL: %w", err)
		}
	}
	st := current.Load()
	slog.Info("[rates] loaded runtime card", "source", st.source, "version", st.card.Version,
		"models", len(st.card.Models), "effective", st.card.EffectiveDate)
	return nil
}

// Watch reloads a configured file/URL card on a fixed interval. A failed
// refresh keeps the last good card (which becomes stale on its own schedule)
// and logs loudly; it never installs an invalid card.
func Watch(ctx context.Context, interval time.Duration) {
	path := strings.TrimSpace(os.Getenv("KUBONA_RATE_CARD_PATH"))
	rawURL := strings.TrimSpace(os.Getenv("KUBONA_RATE_CARD_URL"))
	if interval <= 0 || (path == "" && rawURL == "") {
		return
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			old := current.Load()
			var err error
			if path != "" {
				raw, rerr := os.ReadFile(path)
				if rerr == nil && digest(raw) == old.hash {
					continue
				}
				err = LoadFile(path)
			} else {
				err = LoadURL(ctx, rawURL)
			}
			if err != nil {
				slog.Error("[rates] refresh failed; keeping last good card", "error", err)
				continue
			}
			st := current.Load()
			slog.Info("[rates] refreshed card", "source", st.source, "version", st.card.Version,
				"models", len(st.card.Models), "effective", st.card.EffectiveDate)
		}
	}
}

// Validate checks the current card's shape and effective window.
func Validate(now time.Time) error {
	st := current.Load()
	if st == nil || st.card == nil {
		return fmt.Errorf("rate card is not loaded")
	}
	return validateCard(st.card, now)
}

func validateCard(c *Card, now time.Time) error {
	if c == nil {
		return fmt.Errorf("rate card is not loaded")
	}
	if c.Version <= 0 {
		return fmt.Errorf("rate card version must be positive")
	}
	if c.Source == "" {
		return fmt.Errorf("rate card source is required")
	}
	if c.EffectiveDate == "" || c.StaleAfterDays <= 0 {
		return fmt.Errorf("rate card effective_date and positive stale_after_days are required")
	}
	effective, err := time.Parse("2006-01-02", c.EffectiveDate)
	if err != nil {
		return fmt.Errorf("rate card effective_date: %w", err)
	}
	if now.UTC().After(effective.AddDate(0, 0, c.StaleAfterDays)) {
		return fmt.Errorf("%w: effective %s, stale after %d days; update the runtime card", ErrStale, c.EffectiveDate, c.StaleAfterDays)
	}
	for model, r := range c.Models {
		if r.InputPer1KUSD < 0 || r.OutputPer1KUSD < 0 {
			return fmt.Errorf("rate card model %q has a negative rate", model)
		}
	}
	return nil
}

// IsStale reports whether the current card's effective window has elapsed.
func IsStale(now time.Time) bool {
	st := current.Load()
	if st == nil || st.card == nil {
		return true
	}
	effective, err := time.Parse("2006-01-02", st.card.EffectiveDate)
	if err != nil {
		return true
	}
	return now.UTC().After(effective.AddDate(0, 0, st.card.StaleAfterDays))
}

// Source reports where the active card came from ("embedded", "file:...",
// "url:...").
func Source() string {
	if st := current.Load(); st != nil {
		return st.source
	}
	return ""
}

// IsExternal reports whether a runtime file/URL card is active. Production
// cloud pricing requires an external vendor-derived card; the embedded
// bootstrap is for local/dev and tests only.
func IsExternal() bool {
	src := Source()
	return src != "" && src != "embedded"
}

// LoadedAt reports when the active card was installed.
func LoadedAt() time.Time {
	if st := current.Load(); st != nil {
		return st.loadedAt
	}
	return time.Time{}
}

// LensCostUSD returns the per-dispatch lens cost, or false when the card is
// missing the entry or marks it retired.
func LensCostUSD(name string) (float64, bool) {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("KUBONA_ENV")), "production") && !IsExternal() {
		return 0, false
	}
	st := current.Load()
	if st == nil || st.card == nil {
		return 0, false
	}
	r, ok := st.card.Lens[name]
	if !ok || strings.EqualFold(r.Status, "retired") {
		return 0, false
	}
	return r.CostUSD, true
}

func MustLensCostUSD(name string) float64 {
	cost, ok := LensCostUSD(name)
	if !ok {
		panic("rates: missing lens rate " + name)
	}
	return cost
}

// LookupModelRate returns the input/output rates for a provider/model.
// Provider-qualified keys win; bare keys and aliases are compatibility
// fallbacks. A retired row is never priced.
func LookupModelRate(provider, model string) (ModelRate, bool) {
	_, r, ok := ModelRateKey(provider, model)
	return r, ok
}

// ModelRateKey returns the provider-qualified key that resolves a model, so a
// capture/export path can write the same stable key it audited.
func ModelRateKey(provider, model string) (string, ModelRate, bool) {
	st := current.Load()
	if st == nil || st.card == nil {
		return "", ModelRate{}, false
	}
	p := strings.TrimSpace(provider)
	m := strings.TrimSpace(model)
	candidates := make([]string, 0, 3)
	if p != "" && m != "" {
		candidates = append(candidates, p+"/"+m)
		candidates = append(candidates, p+"/"+strings.TrimPrefix(m, p+"/"))
	}
	if m != "" {
		candidates = append(candidates, m)
	}
	for _, key := range candidates {
		if r, ok := st.card.Models[key]; ok {
			if strings.EqualFold(r.Status, "retired") {
				return "", ModelRate{}, false
			}
			return key, r, true
		}
	}
	if m == "" {
		return "", ModelRate{}, false
	}
	for key, r := range st.card.Models {
		if strings.EqualFold(r.Status, "retired") {
			continue
		}
		for _, a := range r.Aliases {
			aa := strings.TrimSpace(a)
			if aa == m || (p != "" && aa == p+"/"+m) {
				return key, r, true
			}
		}
	}
	return "", ModelRate{}, false
}

// CardSnapshot returns a copy of the loaded card for diagnostics/tests.
func CardSnapshot() Card {
	st := current.Load()
	if st == nil || st.card == nil {
		return Card{}
	}
	return *st.card
}
