package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const discoverProbeTimeout = 900 * time.Millisecond

type localPortHint struct {
	Port     int
	Provider ProviderType
	Name     string
}

var defaultLocalDiscoveryPorts = []localPortHint{
	{Port: 8787, Provider: ProviderOMLX, Name: "oMLX Local Engine"},
	{Port: 11434, Provider: ProviderOllama, Name: "Ollama Local Engine"},
	{Port: 1234, Provider: ProviderLMStudio, Name: "LM Studio Local Server"},
	{Port: 47831, Provider: ProviderStrata, Name: "Strata Local Engine"},
	{Port: 8080, Provider: ProviderCustom, Name: "Local OpenAI-Compatible (8080)"},
	{Port: 8000, Provider: ProviderCustom, Name: "vLLM / OpenAI-Compatible (8000)"},
	{Port: 8899, Provider: ProviderOMLX, Name: "oMLX Local Engine (8899)"},
	{Port: 8081, Provider: ProviderCustom, Name: "llama.cpp Server"},
}

// DiscoveredLocalServer describes one live loopback inference engine found
// during local discovery. Empty Servers means no reachable local engine.
type DiscoveredLocalServer struct {
	Provider       string   `json:"provider"`
	Name           string   `json:"name"`
	Wire           WireType `json:"wire"`
	BaseURL        string   `json:"base_url"`
	Port           int      `json:"port"`
	Models         []string `json:"models"`
	ContextWindow  int      `json:"context_window,omitempty"`
	ContextWarning string   `json:"context_warning,omitempty"`
	Source         string   `json:"source"`
}

// DiscoverLocalResult is the typed payload returned by GET /api/llm/discover.
type DiscoverLocalResult struct {
	Servers      []DiscoveredLocalServer `json:"servers"`
	PortsScanned []int                   `json:"ports_scanned"`
	CheckedAt    string                  `json:"checked_at"`
}

// DiscoverLocalProviders probes common localhost inference ports (plus the
// `lms` CLI when installed) in parallel under a 900ms budget per target.
func DiscoverLocalProviders(ctx context.Context) DiscoverLocalResult {
	ports := make([]int, 0, len(defaultLocalDiscoveryPorts))
	for _, h := range defaultLocalDiscoveryPorts {
		ports = append(ports, h.Port)
	}
	return discoverLocalWithTargets(ctx, "127.0.0.1", defaultLocalDiscoveryPorts, true, ports)
}

func discoverLocalWithTargets(ctx context.Context, host string, hints []localPortHint, includeLMS bool, scanned []int) DiscoverLocalResult {
	cli := &http.Client{
		Timeout: discoverProbeTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	type probeTarget struct {
		baseURL string
		hint    localPortHint
		source  string
	}

	seenBase := make(map[string]bool)
	var targets []probeTarget
	for _, h := range hints {
		base := fmt.Sprintf("http://%s:%d", host, h.Port)
		if !seenBase[base] {
			seenBase[base] = true
			targets = append(targets, probeTarget{baseURL: base, hint: h, source: "port_scan"})
		}
	}
	if includeLMS {
		if lmsBase, port := probeLMStudioCLI(ctx); lmsBase != "" && !seenBase[lmsBase] {
			seenBase[lmsBase] = true
			targets = append(targets, probeTarget{
				baseURL: lmsBase,
				hint:    localPortHint{Port: port, Provider: ProviderLMStudio, Name: "LM Studio CLI Server"},
				source:  "lms_cli",
			})
		}
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	found := make([]DiscoveredLocalServer, 0)
	for _, tgt := range targets {
		wg.Add(1)
		go func(t probeTarget) {
			defer wg.Done()
			srv, ok := probeOneLocalTarget(ctx, cli, t.baseURL, t.hint, t.source)
			if !ok {
				return
			}
			mu.Lock()
			found = append(found, srv)
			mu.Unlock()
		}(tgt)
	}
	wg.Wait()

	sort.Slice(found, func(i, j int) bool {
		if found[i].Port != found[j].Port {
			return found[i].Port < found[j].Port
		}
		return found[i].BaseURL < found[j].BaseURL
	})
	if scanned == nil {
		scanned = []int{}
	}
	return DiscoverLocalResult{
		Servers:      found,
		PortsScanned: scanned,
		CheckedAt:    time.Now().UTC().Format(time.RFC3339),
	}
}

func probeOneLocalTarget(ctx context.Context, cli *http.Client, baseURL string, hint localPortHint, source string) (DiscoveredLocalServer, bool) {
	probeCtx, cancel := context.WithTimeout(ctx, discoverProbeTimeout)
	defer cancel()

	// Try Ollama native /api/tags first when port is 11434 or default probe.
	if srv, ok := probeOllamaEndpoint(probeCtx, cli, baseURL, hint.Port, source); ok {
		return srv, true
	}
	// Fallback to OpenAI-compatible /v1/models.
	if srv, ok := probeOpenAICompatEndpoint(probeCtx, cli, baseURL, hint, source); ok {
		return srv, true
	}
	return DiscoveredLocalServer{}, false
}

func probeOllamaEndpoint(ctx context.Context, cli *http.Client, baseURL string, port int, source string) (DiscoveredLocalServer, bool) {
	tagsURL := strings.TrimRight(baseURL, "/") + "/api/tags"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tagsURL, nil)
	if err != nil {
		return DiscoveredLocalServer{}, false
	}
	resp, err := cli.Do(req)
	if err != nil {
		return DiscoveredLocalServer{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return DiscoveredLocalServer{}, false
	}
	body, err := readBoundedBody(resp.Body)
	if err != nil {
		return DiscoveredLocalServer{}, false
	}
	var parsed struct {
		Models []struct {
			Name  string `json:"name"`
			Model string `json:"model"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return DiscoveredLocalServer{}, false
	}
	// Require JSON "models" key presence cleanly by verifying unmarshal + not random HTML.
	if !bytes.Contains(body, []byte(`"models"`)) {
		return DiscoveredLocalServer{}, false
	}
	models := make([]string, 0, len(parsed.Models))
	for _, m := range parsed.Models {
		id := strings.TrimSpace(m.Name)
		if id == "" {
			id = strings.TrimSpace(m.Model)
		}
		if id != "" {
			models = append(models, id)
		}
	}
	ctxWin, ctxWarn := inspectOllamaNumCtx(ctx, cli, baseURL, models)
	return DiscoveredLocalServer{
		Provider:       string(ProviderOllama),
		Name:           "Ollama Local Engine",
		Wire:           WireOllamaNative,
		BaseURL:        strings.TrimRight(baseURL, "/"),
		Port:           port,
		Models:         models,
		ContextWindow:  ctxWin,
		ContextWarning: ctxWarn,
		Source:         source,
	}, true
}

var numCtxLineRe = regexp.MustCompile(`(?m)\bnum_ctx\s+(\d+)`)

func inspectOllamaNumCtx(ctx context.Context, cli *http.Client, baseURL string, models []string) (int, string) {
	if len(models) == 0 {
		return 0, "No Ollama models pulled yet; raise num_ctx to >= 8192 in Modelfile or OLLAMA_CONTEXT_LENGTH for reliable tool/tribunal prompts."
	}
	payload, _ := json.Marshal(map[string]any{"name": models[0]})
	showURL := strings.TrimRight(baseURL, "/") + "/api/show"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, showURL, bytes.NewReader(payload))
	if err != nil {
		return 0, ""
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := cli.Do(req)
	if err != nil {
		return 0, "Ollama default num_ctx is typically 2048-4096 when unset; configure num_ctx >= 8192 for reliable agent/tribunal prompts."
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, "Ollama default num_ctx is typically 2048-4096 when unset; configure num_ctx >= 8192 for reliable agent/tribunal prompts."
	}
	raw, err := readBoundedBody(resp.Body)
	if err != nil {
		return 0, ""
	}
	var show struct {
		Parameters string `json:"parameters"`
	}
	if err := json.Unmarshal(raw, &show); err != nil {
		return 0, ""
	}
	if m := numCtxLineRe.FindStringSubmatch(show.Parameters); len(m) == 2 {
		if n, perr := strconv.Atoi(m[1]); perr == nil && n > 0 {
			if n < 8192 {
				return n, fmt.Sprintf("Ollama model %s has num_ctx=%d (< 8192); agent/tribunal prompts may truncate.", models[0], n)
			}
			return n, ""
		}
	}
	return 0, fmt.Sprintf("Ollama model %s does not explicitly set num_ctx (defaults to 2048-4096); recommend num_ctx >= 8192.", models[0])
}

func probeOpenAICompatEndpoint(ctx context.Context, cli *http.Client, baseURL string, hint localPortHint, source string) (DiscoveredLocalServer, bool) {
	modelsURL := strings.TrimRight(baseURL, "/") + "/v1/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return DiscoveredLocalServer{}, false
	}
	resp, err := cli.Do(req)
	if err != nil {
		return DiscoveredLocalServer{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return DiscoveredLocalServer{}, false
	}
	body, err := readBoundedBody(resp.Body)
	if err != nil {
		return DiscoveredLocalServer{}, false
	}
	if !bytes.Contains(body, []byte(`"data"`)) {
		return DiscoveredLocalServer{}, false
	}
	var parsed struct {
		Data []struct {
			ID            string `json:"id"`
			OwnedBy       string `json:"owned_by"`
			MaxLen        int    `json:"max_model_len"`
			ContextLength int    `json:"context_length"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return DiscoveredLocalServer{}, false
	}
	models := make([]string, 0, len(parsed.Data))
	prov := hint.Provider
	name := hint.Name
	maxCtx := 0
	for _, m := range parsed.Data {
		id := strings.TrimSpace(m.ID)
		if id != "" {
			models = append(models, id)
		}
		ctxLen := m.MaxLen
		if ctxLen == 0 {
			ctxLen = m.ContextLength
		}
		if ctxLen > maxCtx {
			maxCtx = ctxLen
		}
		owned := strings.ToLower(m.OwnedBy)
		switch {
		case strings.Contains(owned, "lmstudio") || strings.Contains(owned, "lm-studio"):
			prov = ProviderLMStudio
			name = "LM Studio Local Server"
		case strings.Contains(owned, "omlx"):
			prov = ProviderOMLX
			name = "oMLX Local Engine"
		case strings.Contains(owned, "vllm"):
			if prov == "" || prov == ProviderCustom {
				name = "vLLM Local Server"
			}
		case strings.Contains(owned, "llamacpp") || strings.Contains(owned, "llama.cpp"):
			if prov == "" || prov == ProviderCustom {
				name = "llama.cpp Server"
			}
		}
	}
	if prov == "" {
		prov = ProviderCustom
	}
	if name == "" {
		name = string(prov)
	}
	return DiscoveredLocalServer{
		Provider:      string(prov),
		Name:          name,
		Wire:          WireChatCompletions,
		BaseURL:       strings.TrimRight(baseURL, "/") + "/v1",
		Port:          hint.Port,
		Models:        models,
		ContextWindow: maxCtx,
		Source:        source,
	}, true
}

func probeLMStudioCLI(ctx context.Context) (string, int) {
	bin, err := exec.LookPath("lms")
	if err != nil || bin == "" {
		return "", 0
	}
	cmdCtx, cancel := context.WithTimeout(ctx, discoverProbeTimeout)
	defer cancel()
	out, err := exec.CommandContext(cmdCtx, bin, "server", "status").CombinedOutput()
	if err != nil {
		return "", 0
	}
	for _, field := range strings.Fields(string(out)) {
		clean := strings.Trim(field, ".,;:\"'`()[]{}")
		if !strings.HasPrefix(strings.ToLower(clean), "http://") {
			continue
		}
		u, uerr := url.Parse(clean)
		if uerr != nil {
			continue
		}
		if !IsLocalEndpoint(u.String()) {
			continue
		}
		port, _ := strconv.Atoi(u.Port())
		return fmt.Sprintf("http://%s", u.Host), port
	}
	return "", 0
}
