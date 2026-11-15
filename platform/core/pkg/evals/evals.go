// Package evals scores recorded LLM decision outputs against sha-pinned
// fixtures. It is deterministic: no model is called here, and no score is
// invented. A report is only produced when every scored fixture has a
// probability vector that sums to one.
package evals

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	TrustMeasured = "measured"
	TrustInferred = "inferred"
	TrustUnscored = "unscored"

	StatusEvaluated   = "evaluated"
	StatusUnevaluated = "unevaluated"
	StatusStale       = "stale"
)

// AllowedLabels is the closed decision vocabulary.
var AllowedLabels = []string{"supported", "refuted", "unverified"}

var (
	roleRe = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)
	idRe   = regexp.MustCompile(`^[a-z0-9-]{1,80}$`)
)

// Fixture is one recorded decision case. SHA256 pins id+prompt+label+trust,
// so editing a prompt without updating the pin is a load failure.
type Fixture struct {
	ID     string `json:"id"`
	Prompt string `json:"prompt"`
	Label  string `json:"label"`
	Trust  string `json:"trust"`
	SHA256 string `json:"sha256"`
}

// Set is one role's fixture document.
type Set struct {
	Version  int       `json:"version"`
	Role     string    `json:"role"`
	Labels   []string  `json:"labels"`
	Fixtures []Fixture `json:"fixtures"`
	// SHA256 covers the canonical document, including each fixture pin.
	SHA256 string `json:"-"`
}

// Prediction is one model output distribution.
type Prediction struct {
	ID            string             `json:"id"`
	Probabilities map[string]float64 `json:"probabilities"`
}

// PredictionFile is the recorded input to Evaluate.
type PredictionFile struct {
	Role           string       `json:"role"`
	Provider       string       `json:"provider"`
	Model          string       `json:"model"`
	FixturesSHA256 string       `json:"fixtures_sha256"`
	Predictions    []Prediction `json:"predictions"`
}

// LabelScore is one label's exact-match count.
type LabelScore struct {
	Correct int `json:"correct"`
	Total   int `json:"total"`
}

// Report is a stored evaluation.
type Report struct {
	Role           string                `json:"role"`
	Provider       string                `json:"provider"`
	Model          string                `json:"model"`
	FixturesSHA256 string                `json:"fixtures_sha256"`
	EvaluatedAt    string                `json:"evaluated_at"`
	Total          int                   `json:"total"`
	Unscored       int                   `json:"unscored"`
	Brier          float64               `json:"brier"`
	ECE            float64               `json:"ece"`
	PerLabel       map[string]LabelScore `json:"per_label"`
	Labels         []string              `json:"labels"`
}

// Status is the ModelPicker-facing summary.
type Status struct {
	Role           string   `json:"role"`
	Provider       string   `json:"provider"`
	Model          string   `json:"model"`
	Status         string   `json:"status"`
	Evaluated      int      `json:"evaluated"`
	Total          int      `json:"total"`
	Unscored       int      `json:"unscored"`
	EvaluatedAt    *string  `json:"evaluated_at"`
	FixturesSHA256 string   `json:"fixtures_sha256"`
	Brier          *float64 `json:"brier"`
	ECE            *float64 `json:"ece"`
	Labels         []string `json:"labels"`
}

//go:embed fixtures/*.json
var fixtureFS embed.FS

// Load reads one role's embedded fixture set and verifies every pin.
func Load(role string) (*Set, error) {
	if !roleRe.MatchString(role) {
		return nil, fmt.Errorf("invalid role %q", role)
	}
	path := "fixtures/llm-eval-" + role + ".json"
	raw, err := fixtureFS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("no fixtures for role %q", role)
	}
	var set Set
	if err := strictUnmarshal(raw, &set); err != nil {
		return nil, fmt.Errorf("fixtures %s: %w", path, err)
	}
	if err := set.validate(); err != nil {
		return nil, err
	}
	set.SHA256 = set.computeHash()
	return &set, nil
}

func (s *Set) validate() error {
	if s.Version != 1 {
		return fmt.Errorf("unsupported fixture version %d", s.Version)
	}
	if !roleRe.MatchString(s.Role) {
		return fmt.Errorf("invalid fixture role %q", s.Role)
	}
	if len(s.Labels) != len(AllowedLabels) {
		return fmt.Errorf("labels must be exactly %v", AllowedLabels)
	}
	for i, label := range AllowedLabels {
		if s.Labels[i] != label {
			return fmt.Errorf("labels must be exactly %v (got %v)", AllowedLabels, s.Labels)
		}
	}
	if len(s.Fixtures) == 0 {
		return errors.New("fixture set is empty")
	}
	seen := map[string]bool{}
	scored := 0
	for _, fixture := range s.Fixtures {
		if !idRe.MatchString(fixture.ID) {
			return fmt.Errorf("invalid fixture id %q", fixture.ID)
		}
		if seen[fixture.ID] {
			return fmt.Errorf("duplicate fixture id %q", fixture.ID)
		}
		seen[fixture.ID] = true
		if strings.TrimSpace(fixture.Prompt) == "" {
			return fmt.Errorf("fixture %q has an empty prompt", fixture.ID)
		}
		if !contains(AllowedLabels, fixture.Label) {
			return fmt.Errorf("fixture %q label %q is outside the closed vocabulary", fixture.ID, fixture.Label)
		}
		switch fixture.Trust {
		case TrustMeasured, TrustInferred:
			scored++
		case TrustUnscored:
		default:
			return fmt.Errorf("fixture %q trust %q is not measured|inferred|unscored", fixture.ID, fixture.Trust)
		}
		if want := fixturePin(fixture); fixture.SHA256 != want {
			return fmt.Errorf("fixture %q sha256 drifted: got %s want %s (update the pin deliberately)", fixture.ID, fixture.SHA256, want)
		}
	}
	if scored == 0 {
		return errors.New("fixture set has no scored cases")
	}
	return nil
}

func (s *Set) computeHash() string {
	body, _ := json.Marshal(struct {
		Version  int       `json:"version"`
		Role     string    `json:"role"`
		Labels   []string  `json:"labels"`
		Fixtures []Fixture `json:"fixtures"`
	}{s.Version, s.Role, s.Labels, s.Fixtures})
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func (s *Set) scoredFixtures() []Fixture {
	var out []Fixture
	for _, fixture := range s.Fixtures {
		if fixture.Trust != TrustUnscored {
			out = append(out, fixture)
		}
	}
	return out
}

// ScoredCount is the number of fixtures that contribute to Brier/ECE.
func (s *Set) ScoredCount() int { return len(s.scoredFixtures()) }

// UnscoredCount is the number of fixtures deliberately excluded from scores.
func (s *Set) UnscoredCount() int { return len(s.Fixtures) - s.ScoredCount() }

// Evaluate scores complete predictions. `now` is explicit so tests are
// deterministic.
func Evaluate(set *Set, input PredictionFile, now time.Time) (Report, error) {
	report := Report{
		Role:           set.Role,
		Provider:       strings.TrimSpace(input.Provider),
		Model:          strings.TrimSpace(input.Model),
		FixturesSHA256: set.SHA256,
		EvaluatedAt:    now.UTC().Format(time.RFC3339),
		Total:          len(set.scoredFixtures()),
		Unscored:       len(set.Fixtures) - len(set.scoredFixtures()),
		PerLabel:       map[string]LabelScore{},
		Labels:         append([]string(nil), set.Labels...),
	}
	if report.Provider == "" || report.Model == "" {
		return Report{}, errors.New("prediction file must name provider and model")
	}
	if input.Role != set.Role {
		return Report{}, fmt.Errorf("prediction role %q does not match fixture role %q", input.Role, set.Role)
	}
	if input.FixturesSHA256 == "" {
		return Report{}, errors.New("prediction file must carry fixtures_sha256")
	}
	if input.FixturesSHA256 != set.SHA256 {
		return Report{}, fmt.Errorf("fixtures_sha256 mismatch: predictions pin %s, current set is %s", input.FixturesSHA256, set.SHA256)
	}

	byID := map[string]Prediction{}
	for _, prediction := range input.Predictions {
		if _, dup := byID[prediction.ID]; dup {
			return Report{}, fmt.Errorf("duplicate prediction for fixture %q", prediction.ID)
		}
		byID[prediction.ID] = prediction
	}

	scored := set.scoredFixtures()
	scoredIDs := map[string]bool{}
	var missing []string
	for _, fixture := range scored {
		scoredIDs[fixture.ID] = true
		if _, ok := byID[fixture.ID]; !ok {
			missing = append(missing, fixture.ID)
		}
	}
	if len(missing) > 0 {
		return Report{}, fmt.Errorf("incomplete evaluation: missing predictions for %s", strings.Join(missing, ", "))
	}
	for id := range byID {
		if !scoredIDs[id] {
			return Report{}, fmt.Errorf("prediction for %q is not a scored fixture (unscored cases must be omitted)", id)
		}
	}

	brierSum := 0.0
	binCount := make([]int, 10)
	binConf := make([]float64, 10)
	binCorrect := make([]float64, 10)
	for _, fixture := range scored {
		probs, err := validateProbabilities(byID[fixture.ID].Probabilities, set.Labels)
		if err != nil {
			return Report{}, fmt.Errorf("fixture %q: %w", fixture.ID, err)
		}
		// Multiclass Brier: mean sum((p_l - y_l)^2).
		for _, label := range set.Labels {
			y := 0.0
			if label == fixture.Label {
				y = 1
			}
			d := probs[label] - y
			brierSum += d * d
		}
		top, confidence := topLabel(set.Labels, probs)
		correct := top == fixture.Label
		bin := int(confidence * 10)
		if bin > 9 {
			bin = 9
		}
		binCount[bin]++
		binConf[bin] += confidence
		if correct {
			binCorrect[bin]++
		}
		score := report.PerLabel[fixture.Label]
		score.Total++
		if correct {
			score.Correct++
		}
		report.PerLabel[fixture.Label] = score
	}
	report.Brier = brierSum / float64(len(scored))
	ece := 0.0
	for bin := 0; bin < 10; bin++ {
		if binCount[bin] == 0 {
			continue
		}
		accuracy := binCorrect[bin] / float64(binCount[bin])
		confidence := binConf[bin] / float64(binCount[bin])
		ece += (float64(binCount[bin]) / float64(len(scored))) * math.Abs(accuracy-confidence)
	}
	report.ECE = ece
	return report, nil
}

func validateProbabilities(probs map[string]float64, labels []string) (map[string]float64, error) {
	if len(probs) != len(labels) {
		return nil, fmt.Errorf("probabilities must name exactly %v", labels)
	}
	total := 0.0
	for _, label := range labels {
		p, ok := probs[label]
		if !ok {
			return nil, fmt.Errorf("probabilities missing %q", label)
		}
		if math.IsNaN(p) || math.IsInf(p, 0) || p < 0 || p > 1 {
			return nil, fmt.Errorf("probability for %q must be finite in [0,1]", label)
		}
		total += p
	}
	if math.Abs(total-1) > 1e-6 {
		return nil, fmt.Errorf("probabilities sum to %.9f, want 1", total)
	}
	return probs, nil
}

func topLabel(labels []string, probs map[string]float64) (string, float64) {
	best := labels[0]
	for _, label := range labels {
		if probs[label] > probs[best] {
			best = label
		}
	}
	return best, probs[best]
}

// Store persists reports outside the ledger. No secrets are stored: provider,
// model, counts, scores and the fixture pin only.
type Store struct {
	Dir string
}

// DefaultDir returns ~/.kubona/evals, overridable with KUBONA_EVALS_DIR.
func DefaultDir() string {
	if dir := strings.TrimSpace(os.Getenv("KUBONA_EVALS_DIR")); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".kubona", "evals")
}

func (s Store) path(role, provider, model string) string {
	key := role + "\x00" + provider + "\x00" + model
	sum := sha256.Sum256([]byte(key))
	name := sanitize(role) + "__" + sanitize(provider) + "__" + sanitize(model) + "__" + hex.EncodeToString(sum[:4]) + ".json"
	return filepath.Join(s.Dir, name)
}

func sanitize(value string) string {
	cleaned := regexp.MustCompile(`[^A-Za-z0-9._-]+`).ReplaceAllString(strings.TrimSpace(value), "_")
	cleaned = strings.Trim(cleaned, "._-")
	if cleaned == "" {
		return "unknown"
	}
	if len(cleaned) > 48 {
		cleaned = cleaned[:48]
	}
	return cleaned
}

// Write stores a report atomically.
func (s Store) Write(report Report) error {
	if s.Dir == "" {
		return errors.New("eval store has no directory")
	}
	if math.IsNaN(report.Brier) || math.IsInf(report.Brier, 0) || math.IsNaN(report.ECE) || math.IsInf(report.ECE, 0) {
		return errors.New("refusing to store a non-finite score")
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	target := s.path(report.Role, report.Provider, report.Model)
	tmp, err := os.CreateTemp(s.Dir, ".eval-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(append(body, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, target)
}

// Read returns the stored report, or an fs.ErrNotExist error.
func (s Store) Read(role, provider, model string) (*Report, error) {
	if s.Dir == "" {
		return nil, os.ErrNotExist
	}
	raw, err := os.ReadFile(s.path(role, provider, model))
	if err != nil {
		return nil, err
	}
	var report Report
	if err := strictUnmarshal(raw, &report); err != nil {
		return nil, fmt.Errorf("stored eval is unreadable: %w", err)
	}
	if report.Role != role || report.Provider != provider || report.Model != model {
		return nil, errors.New("stored eval identity does not match its path")
	}
	return &report, nil
}

// Status summarizes the stored eval against the current fixture set.
func (s Store) Status(set *Set, role, provider, model string) Status {
	status := Status{
		Role:     role,
		Provider: provider,
		Model:    model,
		Status:   StatusUnevaluated,
		Labels:   []string{},
	}
	if set != nil {
		status.Total = len(set.scoredFixtures())
		status.Unscored = len(set.Fixtures) - status.Total
		status.FixturesSHA256 = set.SHA256
		status.Labels = append([]string(nil), set.Labels...)
	}
	report, err := s.Read(role, provider, model)
	if err != nil {
		return status
	}
	status.Evaluated = report.Total
	status.Status = StatusEvaluated
	at := report.EvaluatedAt
	status.EvaluatedAt = &at
	brier := report.Brier
	ece := report.ECE
	status.Brier = &brier
	status.ECE = &ece
	if set != nil && report.FixturesSHA256 != set.SHA256 {
		status.Status = StatusStale
	}
	return status
}

// LoadPredictionFile reads a recorded prediction document strictly.
func LoadPredictionFile(path string) (PredictionFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return PredictionFile{}, err
	}
	var input PredictionFile
	if err := strictUnmarshal(raw, &input); err != nil {
		return PredictionFile{}, err
	}
	return input, nil
}

// SortedLabels returns a deterministic label list for diagnostics.
func (s *Set) SortedLabels() []string {
	out := append([]string(nil), s.Labels...)
	sort.Strings(out)
	return out
}

func fixturePin(fixture Fixture) string {
	material := fixture.ID + "\n" + fixture.Prompt + "\n" + fixture.Label + "\n" + fixture.Trust
	sum := sha256.Sum256([]byte(material))
	return hex.EncodeToString(sum[:])
}

func strictUnmarshal(raw []byte, target any) error {
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("trailing JSON after the document")
	}
	return nil
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
