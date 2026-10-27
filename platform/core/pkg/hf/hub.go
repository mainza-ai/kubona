package hf

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const HFAPIBase = "https://huggingface.co/api"

// HFResolveBase is the file-download origin. It is a variable (not a const)
// so tests can point it at an httptest server; production always uses
// https://huggingface.co.
var HFResolveBase = "https://huggingface.co"

// HFModelQuantization represents an individual model file (e.g. GGUF format)
type HFModelQuantization struct {
	Filename    string `json:"filename"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`
	QuantType   string `json:"quant_type"`
	DownloadURL string `json:"download_url"`
}

// HFModelSummary represents high-level metadata for a Hugging Face model
type HFModelSummary struct {
	ID            string                `json:"id"`
	Author        string                `json:"author"`
	Downloads     int64                 `json:"downloads"`
	Likes         int64                 `json:"likes"`
	PipelineTag   string                `json:"pipeline_tag"`
	Tags          []string              `json:"tags"`
	Quantizations []HFModelQuantization `json:"quantizations"`
	Description   string                `json:"description,omitempty"`
	// Provenance: live Hub metadata vs the offline sovereign catalog.
	// Fallback counts are illustrative and must never read as measured.
	Source    string `json:"source,omitempty"` // "live" | "fallback"
	Estimated bool   `json:"estimated,omitempty"`
}

// HFDownloadTask tracks an asynchronous model weight download
type HFDownloadTask struct {
	TaskID          string  `json:"task_id"`
	RepoID          string  `json:"repo_id"`
	Filename        string  `json:"filename"`
	Status          string  `json:"status"` // pending, downloading, completed, failed, cancelled
	BytesDownloaded int64   `json:"bytes_downloaded"`
	TotalBytes      int64   `json:"total_bytes"`
	ProgressPercent float64 `json:"progress_percent"`
	SpeedMBS        float64 `json:"speed_mb_s"`
	LocalPath       string  `json:"local_path"`
	Error           string  `json:"error,omitempty"`
	SHA256          string  `json:"sha256,omitempty"`
	StartedAt       int64   `json:"started_at"`
	CompletedAt     int64   `json:"completed_at,omitempty"`

	cancelFunc context.CancelFunc `json:"-"`
}

// HubClient provides Hugging Face model search and metadata resolution
type HubClient struct {
	httpClient *http.Client
	storageDir string
	mu         sync.RWMutex
	tasks      map[string]*HFDownloadTask
}

// DefaultModelsDir resolves the model storage directory: ~/.kubona/models,
// or /tmp/kubona/models when the home directory is unresolvable. Single
// source (S2's disk guard watches this same path — never a copied literal).
func DefaultModelsDir() string {
	home, err := os.UserHomeDir()
	if err == nil {
		return filepath.Join(home, ".kubona", "models")
	}
	return "/tmp/kubona/models"
}

// NewHubClient initializes a new Hugging Face client with local model storage
func NewHubClient(storageDir string) *HubClient {
	if storageDir == "" {
		storageDir = DefaultModelsDir()
	}
	_ = os.MkdirAll(storageDir, 0755)

	return &HubClient{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		storageDir: storageDir,
		tasks:      make(map[string]*HFDownloadTask),
	}
}

func inferQuantType(filename string) string {
	upper := strings.ToUpper(filename)
	if strings.Contains(upper, "MLX") {
		if strings.Contains(upper, "4BIT") {
			return "MLX-4BIT"
		}
		if strings.Contains(upper, "8BIT") {
			return "MLX-8BIT"
		}
		return "MLX"
	}
	commonQuants := []string{
		"Q4_K_M", "Q4_K_S", "Q5_K_M", "Q5_K_S", "Q8_0", "Q6_K", "Q2_K", "Q3_K_M",
		"Q3_K_S", "Q3_K_L", "Q4_0", "Q4_1", "Q5_0", "Q5_1", "FP16", "BF16", "F32",
	}
	for _, q := range commonQuants {
		if strings.Contains(upper, q) {
			return q
		}
	}
	return "GGUF"
}

// Fallback catalog when offline / air-gapped. Downloads/Likes below are
// STATIC ESTIMATES captured at audit time (W2.20), not live Hub counts —
// served only when the live API is unreachable, and labeled as estimates
// wherever they surface (never presented as measured). Live results carry
// real counts from the API.
var fallbackCatalog = []HFModelSummary{
	{
		ID:          "Qwen/Qwen2.5-Coder-32B-Instruct-GGUF",
		Author:      "Qwen",
		Downloads:   1482000,
		Likes:       3412,
		PipelineTag: "text-generation",
		Tags:        []string{"code", "gguf", "qwen", "reasoning"},
		Description: "State-of-the-art open-weights coder LLM optimized for scientific computing and AST synthesis.",
		Quantizations: []HFModelQuantization{
			{Filename: "qwen2.5-coder-32b-instruct-q4_k_m.gguf", SizeBytes: 19800000000, QuantType: "Q4_K_M", DownloadURL: "https://huggingface.co/Qwen/Qwen2.5-Coder-32B-Instruct-GGUF/resolve/main/qwen2.5-coder-32b-instruct-q4_k_m.gguf"},
			{Filename: "qwen2.5-coder-32b-instruct-q8_0.gguf", SizeBytes: 34500000000, QuantType: "Q8_0", DownloadURL: "https://huggingface.co/Qwen/Qwen2.5-Coder-32B-Instruct-GGUF/resolve/main/qwen2.5-coder-32b-instruct-q8_0.gguf"},
		},
	},
	{
		ID:          "Qwen/Qwen2.5-Coder-7B-Instruct-GGUF",
		Author:      "Qwen",
		Downloads:   984000,
		Likes:       1920,
		PipelineTag: "text-generation",
		Tags:        []string{"code", "gguf", "lightweight", "fast"},
		Description: "Fast, low-latency 7B code model ideal for continuous loop mutation generation.",
		Quantizations: []HFModelQuantization{
			{Filename: "qwen2.5-coder-7b-instruct-q4_k_m.gguf", SizeBytes: 4680000000, QuantType: "Q4_K_M", DownloadURL: "https://huggingface.co/Qwen/Qwen2.5-Coder-7B-Instruct-GGUF/resolve/main/qwen2.5-coder-7b-instruct-q4_k_m.gguf"},
		},
	},
	{
		ID:          "meta-llama/Llama-3.3-70B-Instruct-GGUF",
		Author:      "meta-llama",
		Downloads:   2150000,
		Likes:       4890,
		PipelineTag: "text-generation",
		Tags:        []string{"general", "reasoning", "llama-3.3", "gguf"},
		Description: "High-capacity frontier model for polymath theory synthesis and peer review generation.",
		Quantizations: []HFModelQuantization{
			{Filename: "llama-3.3-70b-instruct-q4_k_m.gguf", SizeBytes: 42500000000, QuantType: "Q4_K_M", DownloadURL: "https://huggingface.co/meta-llama/Llama-3.3-70B-Instruct-GGUF/resolve/main/llama-3.3-70b-instruct-q4_k_m.gguf"},
		},
	},
	{
		ID:          "deepseek-ai/DeepSeek-R1-Distill-Qwen-32B-GGUF",
		Author:      "deepseek-ai",
		Downloads:   3200000,
		Likes:       8120,
		PipelineTag: "text-generation",
		Tags:        []string{"reasoning", "math", "deepseek-r1", "gguf"},
		Description: "Chain-of-thought mathematical proof and quantum ansatz search specialist model.",
		Quantizations: []HFModelQuantization{
			{Filename: "deepseek-r1-distill-qwen-32b-q4_k_m.gguf", SizeBytes: 19900000000, QuantType: "Q4_K_M", DownloadURL: "https://huggingface.co/deepseek-ai/DeepSeek-R1-Distill-Qwen-32B-GGUF/resolve/main/deepseek-r1-distill-qwen-32b-q4_k_m.gguf"},
		},
	},
}

// SearchModels queries the live Hugging Face Hub API, falling back to local sovereign models if offline
func (c *HubClient) SearchModels(ctx context.Context, query, library string, limit int) ([]HFModelSummary, error) {
	if limit <= 0 {
		limit = 15
	}
	if library == "" {
		library = "gguf"
	}

	endpoint := fmt.Sprintf("%s/models?search=%s&filter=%s&sort=downloads&direction=-1&limit=%d&full=false",
		HFAPIBase, url.QueryEscape(query), url.QueryEscape(library), limit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return c.filterFallback(query, library, limit), nil
	}
	req.Header.Set("User-Agent", "Kubona-Platform/0.2.0")

	resp, err := c.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		// Resilient sovereign fallback
		return c.filterFallback(query, library, limit), nil
	}
	defer resp.Body.Close()

	var rawItems []struct {
		ID          string   `json:"id"`
		Author      string   `json:"author"`
		Downloads   int64    `json:"downloads"`
		Likes       int64    `json:"likes"`
		PipelineTag string   `json:"pipeline_tag"`
		Tags        []string `json:"tags"`
		Description string   `json:"description"`
		Siblings    []struct {
			RFilename string `json:"rfilename"`
			Size      int64  `json:"size"`
		} `json:"siblings"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rawItems); err != nil {
		return c.filterFallback(query, library, limit), nil
	}

	var results []HFModelSummary
	for _, item := range rawItems {
		author := item.Author
		if author == "" && strings.Contains(item.ID, "/") {
			author = strings.Split(item.ID, "/")[0]
		}

		var quants []HFModelQuantization
		for _, s := range item.Siblings {
			if strings.HasSuffix(strings.ToLower(s.RFilename), ".gguf") {
				quants = append(quants, HFModelQuantization{
					Filename:    s.RFilename,
					SizeBytes:   s.Size,
					QuantType:   inferQuantType(s.RFilename),
					DownloadURL: fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", item.ID, s.RFilename),
				})
			}
		}

		// Provide at least default GGUF entry if siblings array was omitted in full=false
		if len(quants) == 0 && strings.Contains(strings.ToLower(item.ID), "gguf") {
			quants = append(quants, HFModelQuantization{
				Filename:    "model.gguf",
				QuantType:   "Q4_K_M",
				DownloadURL: fmt.Sprintf("https://huggingface.co/%s/resolve/main/model.gguf", item.ID),
			})
		}

		results = append(results, HFModelSummary{
			ID:            item.ID,
			Author:        author,
			Downloads:     item.Downloads,
			Likes:         item.Likes,
			PipelineTag:   item.PipelineTag,
			Tags:          item.Tags,
			Quantizations: quants,
			Description:   item.Description,
			Source:        "live",
		})
	}

	if len(results) == 0 {
		return c.filterFallback(query, library, limit), nil
	}

	return results, nil
}

func (c *HubClient) filterFallback(query, library string, limit int) []HFModelSummary {
	matched := make([]HFModelSummary, 0)
	q := strings.ToLower(query)
	for _, m := range fallbackCatalog {
		if q == "" || strings.Contains(strings.ToLower(m.ID), q) ||
			strings.Contains(strings.ToLower(m.Author), q) ||
			strings.Contains(strings.ToLower(m.Description), q) {
			m.Source = "fallback"
			m.Estimated = true
			matched = append(matched, m)
		}
		if len(matched) >= limit {
			break
		}
	}
	return matched
}

// GetModelDetails returns detailed metadata and available quantizations for a model
func (c *HubClient) GetModelDetails(ctx context.Context, repoID string) (*HFModelSummary, error) {
	endpoint := fmt.Sprintf("%s/models/%s", HFAPIBase, repoID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Kubona-Platform/0.2.0")

	resp, err := c.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		// Fail closed: no invented metadata. Unknown models must surface as
		// errors (HTTP 404 upstream), never as plausible-looking records with
		// fabricated download counts and guessed filenames.
		if err != nil {
			if fb := fallbackModel(repoID); fb != nil {
				return fb, nil
			}
			return nil, fmt.Errorf("huggingface metadata request failed for %q: %w", repoID, err)
		}
		return nil, fmt.Errorf("huggingface has no model %q (status %d)", repoID, resp.StatusCode)
	}
	defer resp.Body.Close()

	var item struct {
		ID          string   `json:"id"`
		Author      string   `json:"author"`
		Downloads   int64    `json:"downloads"`
		Likes       int64    `json:"likes"`
		PipelineTag string   `json:"pipeline_tag"`
		Tags        []string `json:"tags"`
		Description string   `json:"description"`
		Siblings    []struct {
			RFilename string `json:"rfilename"`
			Size      int64  `json:"size"`
		} `json:"siblings"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return nil, err
	}

	var quants []HFModelQuantization
	for _, s := range item.Siblings {
		if strings.HasSuffix(strings.ToLower(s.RFilename), ".gguf") || strings.Contains(strings.ToLower(s.RFilename), "mlx") {
			quants = append(quants, HFModelQuantization{
				Filename:    s.RFilename,
				SizeBytes:   s.Size,
				QuantType:   inferQuantType(s.RFilename),
				DownloadURL: fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", item.ID, s.RFilename),
			})
		}
	}

	return &HFModelSummary{
		ID:            item.ID,
		Author:        item.Author,
		Downloads:     item.Downloads,
		Likes:         item.Likes,
		PipelineTag:   item.PipelineTag,
		Tags:          item.Tags,
		Quantizations: quants,
		Description:   item.Description,
		Source:        "live",
	}, nil
}

func fallbackModel(repoID string) *HFModelSummary {
	for _, m := range fallbackCatalog {
		if strings.EqualFold(m.ID, repoID) {
			m.Source = "fallback"
			m.Estimated = true
			return &m
		}
	}
	return nil
}

// StartDownload initiates an asynchronous in-platform model weight download.
// Filenames are strictly validated (no path traversal); the worker streams
// the real bytes from the Hub resolve endpoint. Validation failures are
// returned synchronously so callers fail closed before any task exists.
func (c *HubClient) StartDownload(repoID, filename string) (*HFDownloadTask, error) {
	if err := validateDownloadTarget(repoID, filename); err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	taskID := "dl-" + uuid.New().String()[:8]
	destDir := filepath.Join(c.storageDir, repoID)
	_ = os.MkdirAll(destDir, 0755)
	localPath := filepath.Join(destDir, filename)

	ctx, cancel := context.WithCancel(context.Background())
	task := &HFDownloadTask{
		TaskID:          taskID,
		RepoID:          repoID,
		Filename:        filename,
		Status:          "downloading",
		BytesDownloaded: 0,
		TotalBytes:      -1, // unknown until response headers arrive
		ProgressPercent: 0.0,
		SpeedMBS:        0.0,
		LocalPath:       localPath,
		StartedAt:       time.Now().Unix(),
		cancelFunc:      cancel,
	}

	c.tasks[taskID] = task

	// Snapshot before the worker can mutate: callers must never share
	// memory with the stored task (race detector proved they do).
	taskCopy := *task

	// Run background download worker (real bytes, see runDownload).
	go c.runDownload(ctx, task)

	return &taskCopy, nil
}

// validateDownloadTarget refuses path-traversal and empty inputs before any
// filesystem or network touch.
func validateDownloadTarget(repoID, filename string) error {
	if repoID == "" || filename == "" {
		return fmt.Errorf("repo_id and filename are required")
	}
	for _, part := range []string{repoID, filename} {
		if part == "." || part == ".." || strings.Contains(part, "..") {
			return fmt.Errorf("invalid download target %q", part)
		}
	}
	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		return fmt.Errorf("filename must not contain path separators: %q", filename)
	}
	if strings.Contains(repoID, "..") {
		return fmt.Errorf("invalid repo_id: %q", repoID)
	}
	return nil
}

// setTerminal moves a task to a final state exactly once: a racing worker
// must never overwrite a state the user (or another path) already settled.
func (c *HubClient) setTerminal(task *HFDownloadTask, status, errMsg string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch task.Status {
	case "completed", "failed", "cancelled":
		return
	}
	task.Status = status
	task.Error = errMsg
	if status == "completed" {
		task.ProgressPercent = 100.0
		task.CompletedAt = time.Now().Unix()
	}
}

func (c *HubClient) runDownload(ctx context.Context, task *HFDownloadTask) {
	downloadURL := fmt.Sprintf("%s/%s/resolve/main/%s", HFResolveBase, task.RepoID, task.Filename)

	// A dedicated client without the metadata client's 10s overall timeout:
	// weight files are gigabytes and stream for minutes.
	dlClient := &http.Client{Timeout: 30 * time.Minute}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		c.setTerminal(task, "failed", err.Error())
		return
	}
	req.Header.Set("User-Agent", "Kubona-Platform/0.2.0")

	resp, err := dlClient.Do(req)
	if err != nil {
		c.setTerminal(task, "failed", fmt.Sprintf("download request failed: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.setTerminal(task, "failed", fmt.Sprintf("hub returned status %d for %s/%s", resp.StatusCode, task.RepoID, task.Filename))
		return
	}

	// Stream to a temp path and rename only on success, so a failed or
	// cancelled download never masquerades as a complete weight file.
	partPath := task.LocalPath + ".part"
	f, err := os.Create(partPath)
	if err != nil {
		c.setTerminal(task, "failed", err.Error())
		return
	}

	hasher := sha256.New()
	multi := io.MultiWriter(f, hasher)
	start := time.Now()
	buf := make([]byte, 128*1024)

	if resp.ContentLength > 0 {
		c.mu.Lock()
		task.TotalBytes = resp.ContentLength
		c.mu.Unlock()
	}

	streamErr := func() error {
		defer f.Close()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			n, rerr := resp.Body.Read(buf)
			if n > 0 {
				if _, werr := multi.Write(buf[:n]); werr != nil {
					return werr
				}
				elapsed := time.Since(start).Seconds()
				c.mu.Lock()
				task.BytesDownloaded += int64(n)
				if task.TotalBytes > 0 {
					task.ProgressPercent = (float64(task.BytesDownloaded) / float64(task.TotalBytes)) * 100.0
				}
				if elapsed > 0 {
					task.SpeedMBS = (float64(task.BytesDownloaded) / (1024 * 1024)) / elapsed
				}
				c.mu.Unlock()
			}
			if rerr != nil {
				if rerr == io.EOF {
					return nil
				}
				return rerr
			}
		}
	}()

	if streamErr != nil {
		_ = os.Remove(partPath)
		if ctx.Err() != nil {
			c.setTerminal(task, "cancelled", "")
		} else {
			c.setTerminal(task, "failed", fmt.Sprintf("download stream failed: %v", streamErr))
		}
		return
	}

	sha := hex.EncodeToString(hasher.Sum(nil))
	if err := os.Rename(partPath, task.LocalPath); err != nil {
		_ = os.Remove(partPath)
		c.setTerminal(task, "failed", err.Error())
		return
	}

	c.mu.Lock()
	task.SHA256 = sha
	c.mu.Unlock()
	c.setTerminal(task, "completed", "")
}

// ListDownloads returns snapshots of all active and historical download
// tasks — copies, never the live structs the workers mutate.
func (c *HubClient) ListDownloads() []*HFDownloadTask {
	c.mu.RLock()
	defer c.mu.RUnlock()

	list := make([]*HFDownloadTask, 0, len(c.tasks))
	for _, t := range c.tasks {
		cp := *t
		list = append(list, &cp)
	}
	return list
}

// GetDownload returns a snapshot copy of one task for status polling.
// Polling the StartDownload return value directly races the worker.
func (c *HubClient) GetDownload(taskID string) (*HFDownloadTask, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	t, exists := c.tasks[taskID]
	if !exists {
		return nil, fmt.Errorf("task %s not found", taskID)
	}
	cp := *t
	return &cp, nil
}

// CancelDownload cancels a currently executing download task. W2.20
// TOCTOU guard: the terminal transition goes through setTerminal, so a
// worker that just completed/failed wins over a racing cancel (a cancel
// that overwrote "completed" would lie about the weight file). Terminal
// tasks refuse instead of re-settling.
func (c *HubClient) CancelDownload(taskID string) error {
	c.mu.Lock()
	task, exists := c.tasks[taskID]
	if !exists {
		c.mu.Unlock()
		return fmt.Errorf("task %s not found", taskID)
	}
	switch task.Status {
	case "completed", "failed", "cancelled":
		status := task.Status
		c.mu.Unlock()
		return fmt.Errorf("task %s already %s: refusing to re-settle terminal state", taskID, status)
	}
	cancel := task.cancelFunc
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	c.setTerminal(task, "cancelled", "")
	return nil
}
