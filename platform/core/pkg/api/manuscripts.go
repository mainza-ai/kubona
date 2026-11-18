package api

// Publication pipeline HTTP surface (R1/R2/R5).
//
// Division of labour: this file owns identity, tenancy, persistence, evidence
// resolution against the ledger, and signoff. The Python bridge
// (ComposeManuscript) owns prose composition, peer review, and the LaTeX
// build; it never decides whether evidence is real.
//
// Invariants enforced here:
//   * a manuscript belongs to one session/tenant; cross-tenant reads 404;
//   * a draft is built from the stored campaign projection, never from a
//     client-supplied template of numbers;
//   * a build/re-review recomposes from the stored snapshot and refuses when
//     the resulting source hash differs from the signed content hash;
//   * signoff requires the operator bearer and signs the exact content hash;
//   * deposit staging requires an ACCEPT review and a valid, unexpired signoff.

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/agents"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	pb "github.com/mainza-ai/kubona/core/proto"
)

const (
	maxManuscriptBodyBytes = 16 * 1024 * 1024
	maxManuscriptFigures   = 8
	maxManuscriptExpiry    = 365 * 24 * time.Hour
)

var errManuscriptSourceChanged = errors.New("manuscript source changed since the draft was stored")

type manuscriptBridgeSection struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
	Latex string `json:"latex"`
}

type manuscriptBridgeResponse struct {
	OK           bool                       `json:"ok"`
	ErrorCode    string                     `json:"error_code"`
	Error        string                     `json:"error"`
	ManuscriptID string                     `json:"manuscript_id"`
	Title        string                     `json:"title"`
	Abstract     string                     `json:"abstract"`
	Sections     []manuscriptBridgeSection  `json:"sections"`
	Bibtex       string                     `json:"bibtex"`
	FigureSHA    map[string]string          `json:"figure_sha256"`
	Citations    []manuscriptCiteIn         `json:"citations"`
	Review       json.RawMessage            `json:"review"`
	Build        *manuscriptBridgeBuildMeta `json:"build"`
}

type manuscriptBridgeBuildMeta struct {
	BuildHash string `json:"build_hash"`
	PDFSHA256 string `json:"pdf_sha256"`
	Log       string `json:"log"`
}

// manuscriptRequest is the bridge JSON contract (not the HTTP body).
type manuscriptRequest struct {
	ManuscriptID    string           `json:"manuscript_id"`
	Title           string           `json:"title,omitempty"`
	Authors         []string         `json:"authors"`
	Abstract        string           `json:"abstract,omitempty"`
	TargetSystem    string           `json:"target_system"`
	Campaign        json.RawMessage  `json:"campaign"`
	Evidence        map[string]any   `json:"evidence"`
	Citations       []map[string]any `json:"citations,omitempty"`
	Figures         []map[string]any `json:"figures,omitempty"`
	Build           bool             `json:"build"`
	SourceDateEpoch int64            `json:"source_date_epoch"`
}

type manuscriptCreateBody struct {
	RequestID        string               `json:"request_id"`
	Title            string               `json:"title"`
	Authors          []string             `json:"authors"`
	Abstract         string               `json:"abstract"`
	EvidenceRefs     []string             `json:"evidence_refs"`
	ProofHash        string               `json:"proof_hash"`
	CitationDistance *float64             `json:"citation_distance"`
	Citations        []manuscriptCiteIn   `json:"citations"`
	Figures          []manuscriptFigureIn `json:"figures"`
	Build            bool                 `json:"build"`
}

type manuscriptCiteIn struct {
	Key      string `json:"key"`
	Title    string `json:"title"`
	Authors  string `json:"authors"`
	Year     string `json:"year"`
	DOI      string `json:"doi"`
	ClaimID  string `json:"claim_id"`
	SourceID string `json:"source_id"`
}

type manuscriptFigureIn struct {
	Name       string `json:"name"`
	ContentB64 string `json:"content_b64"`
}

func (s *Server) handleManuscriptRoute(w http.ResponseWriter, r *http.Request, sessionID, rest string) {
	sess, ok := s.authorizeKnownSession(w, r, sessionID)
	if !ok {
		return
	}
	if sess == nil {
		WriteAPIError(w, "not_found", "unknown session", nil)
		return
	}
	rest = strings.Trim(rest, "/")
	switch {
	case rest == "":
		switch r.Method {
		case http.MethodGet:
			s.handleManuscriptList(w, r, sessionID)
		case http.MethodPost:
			s.handleManuscriptCreate(w, r, sessionID, sess.TenantID)
		default:
			WriteAPIError(w, "method_not_allowed", "manuscripts support GET and POST", nil)
		}
		return
	}
	if ap, ok := manuscriptArtifactPath(rest); ok {
		s.handleManuscriptArtifact(w, r, sessionID, ap.manuscriptID, ap.artifact)
		return
	}
	if strings.HasSuffix(rest, "/build") {
		s.handleManuscriptBuild(w, r, sessionID, strings.TrimSuffix(rest, "/build"))
		return
	}
	if strings.HasSuffix(rest, "/reviews") {
		s.handleManuscriptReviews(w, r, sessionID, strings.TrimSuffix(rest, "/reviews"))
		return
	}
	if strings.HasSuffix(rest, "/signoff") {
		s.handleManuscriptSignoff(w, r, sessionID, strings.TrimSuffix(rest, "/signoff"))
		return
	}
	if strings.HasSuffix(rest, "/deposit") {
		s.handleManuscriptDeposit(w, r, sessionID, strings.TrimSuffix(rest, "/deposit"))
		return
	}
	if strings.HasSuffix(rest, "/citations") {
		s.handleManuscriptCitations(w, r, sessionID, strings.TrimSuffix(rest, "/citations"))
		return
	}
	if strings.HasSuffix(rest, "/verify") {
		s.handleManuscriptVerification(w, r, sessionID, strings.TrimSuffix(rest, "/verify"))
		return
	}
	if idx := strings.Index(rest, "/annotations"); idx > 0 {
		mid := rest[:idx]
		tail := strings.TrimPrefix(rest[idx:], "/annotations")
		if strings.Contains(mid, "/") || (tail != "" && !strings.HasPrefix(tail, "/")) {
			WriteAPIError(w, "not_found", "unknown manuscript route", nil)
			return
		}
		s.handleManuscriptAnnotations(w, r, sessionID, mid, strings.TrimPrefix(tail, "/"))
		return
	}
	if strings.Contains(rest, "/") {
		WriteAPIError(w, "not_found", "unknown manuscript route", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.writeManuscriptDetail(w, sessionID, rest, http.StatusOK)
	case http.MethodPut:
		s.handleManuscriptUpdate(w, r, sessionID, rest)
	case http.MethodDelete:
		s.handleManuscriptDelete(w, r, sessionID, rest)
	default:
		WriteAPIError(w, "method_not_allowed", "manuscript detail supports GET, PUT (draft), DELETE", nil)
	}
}

type artifactPath struct {
	manuscriptID string
	artifact     string
}

// manuscriptArtifactPath recognises "<mid>/artifacts/<name>" where <name> may
// be a nested project path such as "sections/results.tex" or
// "figures/plot.png". Every segment is validated: no traversal, no absolute
// path, no empty segment. A flat-only parser silently 404'd the section
// sources the build advertises.
func manuscriptArtifactPath(rest string) (artifactPath, bool) {
	parts := strings.SplitN(rest, "/artifacts/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return artifactPath{}, false
	}
	mid, name := parts[0], parts[1]
	if strings.Contains(mid, "/") || strings.Contains(mid, ".") {
		return artifactPath{}, false
	}
	if strings.HasPrefix(name, "/") || strings.Contains(name, "\\") {
		return artifactPath{}, false
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return artifactPath{}, false
		}
	}
	return artifactPath{manuscriptID: mid, artifact: name}, true
}

// manuscriptSubpath splits "<session>/manuscripts[/rest]" without matching a
// session whose id merely contains "manuscripts".
func manuscriptSubpath(id string) (sessionID, rest string, ok bool) {
	idx := strings.Index(id, "/manuscripts")
	if idx <= 0 {
		return "", "", false
	}
	sessionID = id[:idx]
	rest = strings.TrimPrefix(id[idx:], "/manuscripts")
	if rest != "" && !strings.HasPrefix(rest, "/") {
		return "", "", false
	}
	return sessionID, strings.TrimPrefix(rest, "/"), true
}

func (s *Server) handleManuscriptList(w http.ResponseWriter, r *http.Request, sessionID string) {
	limit, offset := 50, 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 200 {
		limit = 200
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			offset = n
		}
	}
	items, hasMore, err := s.ledger.ListManuscriptsPage(sessionID, limit, offset)
	if err != nil {
		WriteAPIError(w, "internal", "manuscript list failed", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"manuscripts": items, "limit": limit, "offset": offset, "has_more": hasMore,
	})
}

// manuscriptWorkAllowed paces the CPU-bound manuscript operations (compose,
// build, re-review). Reads and the operator's signoff/deposit are not paced.
func (s *Server) manuscriptWorkAllowed(w http.ResponseWriter, r *http.Request) bool {
	if s.manuscriptLimiter == nil || s.manuscriptLimiter.allow(clientKey(r)) {
		return true
	}
	WriteAPIError(w, "rate_limited", "manuscript compose/build is CPU-bound: back off before retrying", nil)
	return false
}

func (s *Server) handleManuscriptCreate(w http.ResponseWriter, r *http.Request, sessionID, tenantID string) {
	if !s.manuscriptWorkAllowed(w, r) {
		return
	}
	if blocked, reason := s.diskBlocked(); blocked {
		WriteAPIError(w, "storage_blocked", "storage is blocked: "+reason, nil)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxManuscriptBodyBytes)
	var body manuscriptCreateBody
	if err := decodeStrict(r, &body); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid manuscript request: %v", err), nil)
		return
	}
	if body.RequestID != "" {
		if err := ValidateRequestID(body.RequestID); err != nil {
			WriteAPIError(w, "bad_request", err.Error(), nil)
			return
		}
		if existing, gerr := s.ledger.GetManuscriptByRequestID(sessionID, body.RequestID); gerr == nil {
			s.writeManuscriptDetail(w, sessionID, existing.ID, http.StatusOK)
			return
		} else if !errors.Is(gerr, ledger.ErrManuscriptNotFound) {
			WriteAPIError(w, "internal", "idempotency lookup failed", nil)
			return
		}
	}
	rawState, err := s.ledger.GetCampaignState(sessionID)
	if err != nil || len(rawState) == 0 {
		WriteAPIError(w, "campaign_missing", "no campaign projection exists for this session", nil)
		return
	}
	var campaign agents.CampaignState
	if err := json.Unmarshal(rawState, &campaign); err != nil {
		WriteAPIError(w, "campaign_invalid", "campaign projection is not decodable", nil)
		return
	}

	resolved, unresolved := s.resolveManuscriptReceipts(&campaign, body.EvidenceRefs)
	proofHash, proofOK := s.resolveManuscriptProof(sessionID, strings.TrimSpace(body.ProofHash))
	evidence := manuscriptEvidencePayload(resolved, unresolved, proofHash, proofOK)
	if body.CitationDistance != nil {
		if *body.CitationDistance < 0 || *body.CitationDistance > 100 {
			WriteAPIError(w, "bad_request", "citation_distance must be between 0 and 100", nil)
			return
		}
		evidence["citation_distance"] = *body.CitationDistance
	}
	verifiedClaims := map[string]bool{}
	for _, c := range body.Citations {
		if c.ClaimID == "" {
			continue
		}
		claim, _, _, cerr := s.ledger.ClaimProvenance(sessionID, c.ClaimID)
		if cerr != nil || claim == nil {
			WriteAPIError(w, "bad_request",
				fmt.Sprintf("citation claim %q was not found in this session", c.ClaimID), nil)
			return
		}
		verifiedClaims[c.ClaimID] = true
	}
	citations := citePayload(body.Citations)
	figures, ok := figurePayload(w, body.Figures)
	if !ok {
		return
	}
	authors := cleanAuthors(body.Authors)
	epoch := time.Now().UTC().Unix()
	id := newManuscriptID()

	req := manuscriptRequest{
		ManuscriptID:    id,
		Title:           strings.TrimSpace(body.Title),
		Authors:         authors,
		Abstract:        strings.TrimSpace(body.Abstract),
		TargetSystem:    campaign.TargetSystem,
		Campaign:        json.RawMessage(rawState),
		Evidence:        evidence,
		Citations:       citations,
		Figures:         figures,
		Build:           false,
		SourceDateEpoch: epoch,
	}
	resp, _, err := s.callManuscriptBridge(r.Context(), req)
	if err != nil {
		writeBridgeUnavailable(w, "manuscript_compose", err.Error())
		return
	}
	if !resp.OK {
		WriteAPIError(w, manuscriptErrorCode(resp.ErrorCode), resp.Error, nil)
		return
	}

	if err := verifyBridgeFigureHashes(figures, resp.FigureSHA); err != nil {
		WriteAPIError(w, "bridge_unavailable", err.Error(), nil)
		return
	}
	// The bridge sanitizes and deduplicates BibTeX keys; store the effective
	// keys the PDF actually cites, not the caller's raw string.
	if err := verifyBridgeFigureHashes(req.Figures, resp.FigureSHA); err != nil {
		WriteAPIError(w, "bridge_unavailable", err.Error(), nil)
		return
	}
	citationInput := body.Citations
	if len(resp.Citations) > 0 {
		citationInput = resp.Citations
	}
	storedCitations := manuscriptCitations(citationInput)
	for i := range storedCitations {
		storedCitations[i].Verified = storedCitations[i].ClaimID != "" && verifiedClaims[storedCitations[i].ClaimID]
	}
	hash := manuscriptContentHash(resp.Title, resp.Abstract, resp.Sections, storedCitations, figures, resp.FigureSHA)
	authorsJSON, _ := json.Marshal(authors)
	figuresJSON, _ := json.Marshal(figures)
	shaJSON, _ := json.Marshal(resp.FigureSHA)
	evidenceJSON, _ := json.Marshal(evidence)
	ms := &ledger.Manuscript{
		ID:              id,
		SessionID:       sessionID,
		TenantID:        tenantID,
		Title:           resp.Title,
		Abstract:        resp.Abstract,
		State:           ledger.ManuscriptStateDraft,
		RequestID:       body.RequestID,
		ContentHash:     hash,
		ReviewJSON:      nonEmptyJSON(resp.Review),
		CampaignJSON:    string(rawState),
		EvidenceJSON:    string(evidenceJSON),
		AuthorsJSON:     string(authorsJSON),
		FiguresJSON:     string(figuresJSON),
		FigureSHAJSON:   string(shaJSON),
		SourceDateEpoch: epoch,
		Sections:        manuscriptSections(resp.Sections),
		Citations:       storedCitations,
	}
	if err := s.ledger.CreateManuscript(ms); err != nil {
		// A concurrent create may have won the unique request-id index.
		if body.RequestID != "" {
			if existing, gerr := s.ledger.GetManuscriptByRequestID(sessionID, body.RequestID); gerr == nil {
				s.writeManuscriptDetail(w, sessionID, existing.ID, http.StatusOK)
				return
			}
		}
		WriteAPIError(w, "internal", "manuscript persistence failed: "+err.Error(), nil)
		return
	}
	if err := s.recordManuscriptEvent(sessionID, "manuscript_drafted", map[string]any{
		"manuscript_id": id, "content_hash": hash,
		"review": reviewRecommendation(ms.ReviewJSON),
	}); err != nil {
		WriteAPIError(w, "internal", "manuscript event failed: "+err.Error(), nil)
		return
	}
	s.recordManuscriptReview(sessionID, ms)

	if body.Build {
		if err := s.buildStoredManuscript(r.Context(), ms); err != nil {
			s.writeManuscriptError(w, err)
			return
		}
	}
	s.writeManuscriptDetail(w, sessionID, id, http.StatusCreated)
}

func (s *Server) writeManuscriptDetail(w http.ResponseWriter, sessionID, id string, status int) {
	ms, err := s.ledger.GetManuscript(sessionID, id)
	if errors.Is(err, ledger.ErrManuscriptNotFound) {
		WriteAPIError(w, "not_found", "unknown manuscript", nil)
		return
	}
	if err != nil {
		WriteAPIError(w, "internal", "manuscript read failed", nil)
		return
	}
	reviews, err := s.ledger.ListManuscriptReviews(id)
	if err != nil {
		WriteAPIError(w, "internal", "manuscript reviews read failed", nil)
		return
	}
	annotations, err := s.ledger.ListManuscriptAnnotations(sessionID, id)
	if err != nil {
		WriteAPIError(w, "internal", "manuscript annotations read failed", nil)
		return
	}
	open := 0
	for _, a := range annotations {
		if a.Status == ledger.AnnotationOpen {
			open++
		}
	}
	writeJSON(w, status, map[string]any{
		"manuscript":       manuscriptJSON(ms),
		"reviews":          reviews,
		"annotations":      annotations,
		"open_annotations": open,
	})
}

func manuscriptJSON(ms *ledger.Manuscript) map[string]any {
	sections := make([]map[string]any, 0, len(ms.Sections))
	for _, sec := range ms.Sections {
		sections = append(sections, map[string]any{
			"slug": sec.Slug, "title": sec.Title, "latex": sec.LaTeX, "position": sec.Position,
		})
	}
	citations := make([]map[string]any, 0, len(ms.Citations))
	for _, c := range ms.Citations {
		citations = append(citations, map[string]any{
			"key": c.Key, "title": c.Title, "authors": c.Authors, "year": c.Year, "doi": c.DOI,
			"claim_id": c.ClaimID, "source_id": c.SourceID, "verified": c.Verified,
		})
	}
	out := map[string]any{
		"id":                ms.ID,
		"session_id":        ms.SessionID,
		"title":             ms.Title,
		"abstract":          ms.Abstract,
		"state":             ms.State,
		"content_hash":      ms.ContentHash,
		"build_hash":        ms.BuildHash,
		"review":            json.RawMessage(nonEmptyJSON([]byte(ms.ReviewJSON))),
		"source_date_epoch": ms.SourceDateEpoch,
		"created_at":        ms.CreatedAt,
		"updated_at":        ms.UpdatedAt,
		"sections":          sections,
		"citations":         citations,
		"artifacts":         ms.Artifacts,
	}
	if ms.Signoff != nil {
		out["signoff"] = signoffJSON(ms.Signoff)
	} else {
		out["signoff"] = nil
	}
	if strings.TrimSpace(ms.VerificationJSON) != "" {
		out["verification"] = json.RawMessage(ms.VerificationJSON)
	} else {
		out["verification"] = nil
	}
	return out
}

func (s *Server) handleManuscriptArtifact(w http.ResponseWriter, r *http.Request, sessionID, id, name string) {
	if r.Method != http.MethodGet {
		WriteAPIError(w, "method_not_allowed", "artifact reads are GET-only", nil)
		return
	}
	raw, sum, err := s.ledger.GetManuscriptArtifact(sessionID, id, name)
	if errors.Is(err, ledger.ErrManuscriptNotFound) {
		WriteAPIError(w, "not_found", "unknown manuscript artifact", nil)
		return
	}
	if err != nil {
		WriteAPIError(w, "internal", "artifact read failed: "+err.Error(), nil)
		return
	}
	switch {
	case strings.HasSuffix(name, ".pdf"):
		w.Header().Set("Content-Type", "application/pdf")
	case strings.HasSuffix(name, ".tex"), strings.HasSuffix(name, ".bib"):
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	case strings.HasSuffix(name, ".json"):
		w.Header().Set("Content-Type", "application/json")
	case strings.HasSuffix(name, ".png"):
		w.Header().Set("Content-Type", "image/png")
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	w.Header().Set("X-Kubona-SHA256", sum)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

func (s *Server) handleManuscriptBuild(w http.ResponseWriter, r *http.Request, sessionID, id string) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "build is POST-only", nil)
		return
	}
	if !s.manuscriptWorkAllowed(w, r) {
		return
	}
	if blocked, reason := s.diskBlocked(); blocked {
		WriteAPIError(w, "storage_blocked", "storage is blocked: "+reason, nil)
		return
	}
	ms, err := s.ledger.GetManuscript(sessionID, id)
	if errors.Is(err, ledger.ErrManuscriptNotFound) {
		WriteAPIError(w, "not_found", "unknown manuscript", nil)
		return
	}
	if err != nil {
		WriteAPIError(w, "internal", "manuscript read failed", nil)
		return
	}
	if len(ms.Sections) == 0 {
		WriteAPIError(w, "bad_request", "manuscript has no sections", nil)
		return
	}
	if err := s.buildStoredManuscript(r.Context(), ms); err != nil {
		s.writeManuscriptError(w, err)
		return
	}
	s.writeManuscriptDetail(w, sessionID, id, http.StatusOK)
}

// buildStoredManuscript recomposes from the stored snapshot, refuses a
// changed source, and installs the artifact set.
func (s *Server) buildStoredManuscript(ctx context.Context, ms *ledger.Manuscript) (err error) {
	started := time.Now()
	defer func() { s.observeManuscriptBuild(err, time.Since(started)) }()
	req, err := s.storedManuscriptRequest(ms, true)
	if err != nil {
		return err
	}
	resp, artifacts, err := s.callManuscriptBridge(ctx, req)
	if err != nil {
		return err
	}
	if !resp.OK {
		return &manuscriptRefusal{code: manuscriptErrorCode(resp.ErrorCode), message: resp.Error}
	}
	hash := manuscriptContentHash(resp.Title, resp.Abstract, resp.Sections, ms.Citations, req.Figures, resp.FigureSHA)
	if hash != ms.ContentHash {
		return errManuscriptSourceChanged
	}
	if resp.Build == nil {
		return &manuscriptRefusal{code: "build_failed", message: "bridge returned no build metadata"}
	}
	if err := s.checkArtifactCeiling(ms, artifacts); err != nil {
		return err
	}
	if err := s.ledger.ReplaceManuscriptArtifacts(ms.SessionID, ms.ID, resp.Build.BuildHash, artifacts); err != nil {
		return err
	}
	if err := s.recordManuscriptEvent(ms.SessionID, "manuscript_built", map[string]any{
		"manuscript_id": ms.ID, "build_hash": resp.Build.BuildHash,
	}); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleManuscriptReviews(w http.ResponseWriter, r *http.Request, sessionID, id string) {
	if r.Method == http.MethodGet {
		reviews, err := s.ledger.ListManuscriptReviews(id)
		if err != nil {
			WriteAPIError(w, "internal", "review read failed", nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"reviews": reviews})
		return
	}
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "reviews support GET and POST", nil)
		return
	}
	if !s.manuscriptWorkAllowed(w, r) {
		return
	}
	ms, err := s.ledger.GetManuscript(sessionID, id)
	if errors.Is(err, ledger.ErrManuscriptNotFound) {
		WriteAPIError(w, "not_found", "unknown manuscript", nil)
		return
	}
	if err != nil {
		WriteAPIError(w, "internal", "manuscript read failed", nil)
		return
	}
	req, err := s.storedManuscriptRequest(ms, false)
	if err != nil {
		WriteAPIError(w, "internal", "manuscript snapshot is unreadable", nil)
		return
	}
	resp, _, err := s.callManuscriptBridge(r.Context(), req)
	if err != nil {
		writeBridgeUnavailable(w, "manuscript_review", err.Error())
		return
	}
	if !resp.OK {
		WriteAPIError(w, manuscriptErrorCode(resp.ErrorCode), resp.Error, nil)
		return
	}
	if manuscriptContentHash(resp.Title, resp.Abstract, resp.Sections, ms.Citations, req.Figures, resp.FigureSHA) != ms.ContentHash {
		WriteAPIError(w, "source_changed", "the manuscript source changed; create a new draft", nil)
		return
	}
	if err := s.ledger.SetManuscriptReviewState(sessionID, id, nonEmptyJSON(resp.Review)); err != nil {
		WriteAPIError(w, "internal", "review state failed", nil)
		return
	}
	ms.ReviewJSON = nonEmptyJSON(resp.Review)
	s.recordManuscriptReview(sessionID, ms)
	if err := s.recordManuscriptEvent(sessionID, "manuscript_reviewed", map[string]any{
		"manuscript_id": id, "recommendation": reviewRecommendation(ms.ReviewJSON),
		"content_hash": ms.ContentHash,
	}); err != nil {
		WriteAPIError(w, "internal", "manuscript event failed: "+err.Error(), nil)
		return
	}
	s.writeManuscriptDetail(w, sessionID, id, http.StatusOK)
}

func (s *Server) handleManuscriptSignoff(w http.ResponseWriter, r *http.Request, sessionID, id string) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "signoff is POST-only", nil)
		return
	}
	tenant, _, code := s.checkAuth(r)
	if code != http.StatusOK || tenant != "" {
		WriteAPIError(w, "forbidden", "manuscript signoff requires the operator bearer", nil)
		return
	}
	if !s.reviewerKeyConfigured && !ephemeralAllowed() {
		WriteAPIError(w, "reviewer_key_unconfigured", "no commissioned reviewer key is configured", nil)
		return
	}
	var body struct {
		ContentHash           string `json:"content_hash"`
		Expiry                string `json:"expiry"`
		AcknowledgeUnverified bool   `json:"acknowledge_unverified"`
	}
	if err := decodeStrict(r, &body); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid signoff request: %v", err), nil)
		return
	}
	ms, err := s.ledger.GetManuscript(sessionID, id)
	if errors.Is(err, ledger.ErrManuscriptNotFound) {
		WriteAPIError(w, "not_found", "unknown manuscript", nil)
		return
	}
	if err != nil {
		WriteAPIError(w, "internal", "manuscript read failed", nil)
		return
	}
	if body.ContentHash == "" || body.ContentHash != ms.ContentHash {
		WriteAPIError(w, "content_hash_mismatch", "signoff must name the manuscript's current content hash", nil)
		return
	}
	open, err := s.ledger.CountOpenAnnotations(sessionID, id)
	if err != nil {
		WriteAPIError(w, "internal", "annotation count failed", nil)
		return
	}
	if open > 0 {
		WriteAPIError(w, "annotations_open",
			fmt.Sprintf("%d open annotation(s) must be resolved before signoff", open), nil)
		return
	}
	// AG-5: verify against recorded evidence at the moment of signing. The
	// report is bound to this content hash and persisted before the gate.
	report, err := s.verifyManuscript(r.Context(), sessionID, ms)
	if err != nil {
		WriteAPIError(w, "internal", "verification failed: "+err.Error(), nil)
		return
	}
	if err := s.ledger.SetManuscriptVerification(sessionID, id, report.JSON()); err != nil {
		WriteAPIError(w, "internal", "verification persistence failed: "+err.Error(), nil)
		return
	}
	if report.Summary.Status == "blocked" {
		WriteAPIError(w, "evidence_verification_failed",
			fmt.Sprintf("%d finding(s) are unsupported by recorded evidence", report.Summary.Blocking),
			map[string]any{"summary": report.Summary, "findings": report.BlockingDetails()})
		return
	}
	if report.Summary.Status == "unverified" && !body.AcknowledgeUnverified {
		WriteAPIError(w, "verification_unavailable",
			"verification could not confirm every claim; pass acknowledge_unverified to sign anyway",
			map[string]any{"summary": report.Summary})
		return
	}
	expiry, err := normalizeExpiry(body.Expiry)
	if err != nil {
		WriteAPIError(w, "bad_request", err.Error(), nil)
		return
	}
	payload := []byte("manuscript-signoff:" + id + ":" + ms.ContentHash + ":" + expiry)
	sig := ed25519.Sign(s.signerPrivKey, payload)
	pub := hex.EncodeToString(s.signerPubKey)
	if err := s.ledger.SetManuscriptSignoff(sessionID, id, ledger.ManuscriptSignoff{
		SignerPubKey: pub,
		Signature:    hex.EncodeToString(sig),
		ContentHash:  ms.ContentHash,
		Expiry:       expiry,
	}); err != nil {
		WriteAPIError(w, "internal", "signoff persistence failed: "+err.Error(), nil)
		return
	}
	if _, err := s.ledger.AppendSignedEvent(sessionID, "manuscript_signed", map[string]any{
		"manuscript_id": id, "content_hash": ms.ContentHash, "signer_pubkey_hex": pub,
		"verification":            report.Summary.Status,
		"acknowledged_unverified": body.AcknowledgeUnverified && report.Summary.Status == "unverified",
	}, s.signerPrivKey); err != nil {
		WriteAPIError(w, "internal", "signoff event failed: "+err.Error(), nil)
		return
	}
	s.writeManuscriptDetail(w, sessionID, id, http.StatusOK)
}

func (s *Server) handleManuscriptDeposit(w http.ResponseWriter, r *http.Request, sessionID, id string) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "deposit staging is POST-only", nil)
		return
	}
	tenant, _, code := s.checkAuth(r)
	if code != http.StatusOK || tenant != "" {
		WriteAPIError(w, "forbidden", "deposit staging requires the operator bearer", nil)
		return
	}
	ms, err := s.ledger.GetManuscript(sessionID, id)
	if errors.Is(err, ledger.ErrManuscriptNotFound) {
		WriteAPIError(w, "not_found", "unknown manuscript", nil)
		return
	}
	if err != nil {
		WriteAPIError(w, "internal", "manuscript read failed", nil)
		return
	}
	if ms.Signoff == nil {
		WriteAPIError(w, "signoff_required", "deposit staging requires a signoff", nil)
		return
	}
	if expiry, err := time.Parse(time.RFC3339, ms.Signoff.Expiry); err != nil || time.Now().UTC().After(expiry) {
		WriteAPIError(w, "signoff_expired", "signoff is expired or unreadable", nil)
		return
	}
	if ms.Signoff.ContentHash != ms.ContentHash {
		WriteAPIError(w, "source_changed", "signoff no longer matches the manuscript content", nil)
		return
	}
	if rec := reviewRecommendation(ms.ReviewJSON); rec != "ACCEPT" {
		WriteAPIError(w, "review_not_accepted", "deposit staging requires an ACCEPT review; current review is "+rec, nil)
		return
	}
	if len(ms.Artifacts) == 0 {
		WriteAPIError(w, "build_required", "deposit staging requires a built PDF", nil)
		return
	}
	files := make([]map[string]string, 0, len(ms.Artifacts))
	artifacts := map[string][]byte{}
	for _, a := range ms.Artifacts {
		raw, _, gerr := s.ledger.GetManuscriptArtifact(sessionID, id, a.Name)
		if gerr != nil {
			WriteAPIError(w, "internal", "artifact read failed: "+gerr.Error(), nil)
			return
		}
		files = append(files, map[string]string{"name": a.Name, "sha256": a.SHA256})
		artifacts[a.Name] = raw
	}
	manifest := map[string]any{
		"state":                 "staged-needs-human-upload",
		"title":                 ms.Title,
		"description":           ms.Abstract,
		"manuscript_id":         ms.ID,
		"content_hash":          ms.ContentHash,
		"build_hash":            ms.BuildHash,
		"review":                json.RawMessage(nonEmptyJSON([]byte(ms.ReviewJSON))),
		"signoff":               signoffJSON(ms.Signoff),
		"files":                 files,
		"requires_human_upload": true,
		"auto_publish":          false,
		"doi":                   nil,
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		WriteAPIError(w, "internal", "manifest encode failed", nil)
		return
	}
	artifacts["zenodo_deposit.json"] = raw
	if err := s.checkArtifactCeiling(ms, artifacts); err != nil {
		s.writeManuscriptError(w, err)
		return
	}
	if err := s.ledger.ReplaceManuscriptArtifacts(sessionID, id, ms.BuildHash, artifacts); err != nil {
		WriteAPIError(w, "internal", "deposit artifact persistence failed: "+err.Error(), nil)
		return
	}
	if err := s.ledger.SetManuscriptState(sessionID, id, ledger.ManuscriptStateDepositReady); err != nil {
		WriteAPIError(w, "internal", "deposit state failed: "+err.Error(), nil)
		return
	}
	if err := s.recordManuscriptEvent(sessionID, "manuscript_deposit_staged", map[string]any{
		"manuscript_id": id, "content_hash": ms.ContentHash, "build_hash": ms.BuildHash,
	}); err != nil {
		WriteAPIError(w, "internal", "manuscript event failed: "+err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "manifest": manifest})
}

func signoffJSON(s *ledger.ManuscriptSignoff) map[string]any {
	return map[string]any{
		"signer_pubkey_hex": s.SignerPubKey,
		"signature_hex":     s.Signature,
		"content_hash":      s.ContentHash,
		"expiry":            s.Expiry,
		"signed_at":         s.SignedAt,
	}
}

// storedManuscriptRequest reconstructs the bridge request from the persisted
// snapshot. This is what makes a rebuild verify the same source the signoff
// binds, rather than trusting the current campaign (which may have moved).
func (s *Server) storedManuscriptRequest(ms *ledger.Manuscript, build bool) (manuscriptRequest, error) {
	if strings.TrimSpace(ms.CampaignJSON) == "" || ms.CampaignJSON == "{}" {
		return manuscriptRequest{}, fmt.Errorf("stored campaign snapshot is empty")
	}
	var authors []string
	if ms.AuthorsJSON != "" {
		if err := json.Unmarshal([]byte(ms.AuthorsJSON), &authors); err != nil {
			return manuscriptRequest{}, err
		}
	}
	var figures []map[string]any
	if ms.FiguresJSON != "" {
		if err := json.Unmarshal([]byte(ms.FiguresJSON), &figures); err != nil {
			return manuscriptRequest{}, err
		}
	}
	evidence := map[string]any{}
	if ms.EvidenceJSON != "" {
		if err := json.Unmarshal([]byte(ms.EvidenceJSON), &evidence); err != nil {
			return manuscriptRequest{}, err
		}
	}
	var target string
	var campaign agents.CampaignState
	if err := json.Unmarshal([]byte(ms.CampaignJSON), &campaign); err == nil {
		target = campaign.TargetSystem
	}
	citations := make([]map[string]any, 0, len(ms.Citations))
	for _, c := range ms.Citations {
		citations = append(citations, map[string]any{
			"key": c.Key, "title": c.Title, "authors": c.Authors, "year": c.Year, "doi": c.DOI,
			"claim_id": c.ClaimID, "source_id": c.SourceID, "verified": c.Verified,
		})
	}
	return manuscriptRequest{
		ManuscriptID:    ms.ID,
		Title:           ms.Title,
		Authors:         authors,
		Abstract:        ms.Abstract,
		TargetSystem:    target,
		Campaign:        json.RawMessage(ms.CampaignJSON),
		Evidence:        evidence,
		Citations:       citations,
		Figures:         figures,
		Build:           build,
		SourceDateEpoch: ms.SourceDateEpoch,
	}, nil
}

func (s *Server) callManuscriptBridge(ctx context.Context, req manuscriptRequest) (*manuscriptBridgeResponse, map[string][]byte, error) {
	client := s.bridgeClient()
	if client == nil {
		return nil, nil, fmt.Errorf("bridge unavailable: no gRPC client")
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return nil, nil, err
	}
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	pbResp, err := client.ComposeManuscript(callCtx, &pb.ManuscriptRequest{
		SessionId:   "",
		RequestJson: string(raw),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("ComposeManuscript RPC failed: %w", err)
	}
	var resp manuscriptBridgeResponse
	if err := json.Unmarshal([]byte(pbResp.ResponseJson), &resp); err != nil {
		return nil, nil, fmt.Errorf("manuscript response is not decodable: %w", err)
	}
	if !pbResp.Ok && resp.Error == "" {
		resp.Error = pbResp.ErrorMessage
		resp.ErrorCode = pbResp.ErrorCode
	}
	artifacts := map[string][]byte{}
	for _, a := range pbResp.Artifacts {
		if a.Name == "" || len(a.Content) == 0 {
			continue
		}
		sum := sha256.Sum256(a.Content)
		if a.Sha256 != "" && a.Sha256 != hex.EncodeToString(sum[:]) {
			return nil, nil, fmt.Errorf("artifact %q failed the bridge hash", a.Name)
		}
		artifacts[a.Name] = a.Content
	}
	return &resp, artifacts, nil
}

type manuscriptRefusal struct {
	code    string
	message string
}

func (e *manuscriptRefusal) Error() string { return e.message }

func (s *Server) writeManuscriptError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errManuscriptSourceChanged):
		WriteAPIError(w, "source_changed", "the manuscript source changed since the draft; create a new draft", nil)
	case errors.As(err, new(*manuscriptRefusal)):
		var ref *manuscriptRefusal
		errors.As(err, &ref)
		WriteAPIError(w, ref.code, ref.message, nil)
	default:
		writeBridgeUnavailable(w, "manuscript", err.Error())
	}
}

func manuscriptErrorCode(code string) string {
	switch code {
	case "invalid_request":
		return "bad_request"
	case "build_unavailable", "build_failed":
		return code
	case "":
		return "bad_request"
	default:
		return code
	}
}

func manuscriptSections(in []manuscriptBridgeSection) []ledger.ManuscriptSection {
	out := make([]ledger.ManuscriptSection, 0, len(in))
	for i, sec := range in {
		out = append(out, ledger.ManuscriptSection{
			Slug: sec.Slug, Title: sec.Title, LaTeX: sec.Latex, Position: i,
		})
	}
	return out
}

func manuscriptCitations(in []manuscriptCiteIn) []ledger.ManuscriptCitation {
	out := make([]ledger.ManuscriptCitation, 0, len(in))
	for _, c := range in {
		out = append(out, ledger.ManuscriptCitation{
			Key: c.Key, Title: c.Title, Authors: c.Authors, Year: c.Year, DOI: c.DOI,
			ClaimID: c.ClaimID, SourceID: c.SourceID,
		})
	}
	return out
}

// manuscriptContentHash is the Go-side canonical source hash. It deliberately
// does not depend on Python formatting, but it IS order-sensitive: section
// order, citation order (the introduction cites in order), and figure order
// all change the rendered document, so they must change the hash. Sorting
// would let a reordered paper share a signoff hash.
func manuscriptContentHash(title, abstract string, sections []manuscriptBridgeSection,
	citations []ledger.ManuscriptCitation, figures []map[string]any, figureSHA map[string]string) string {
	var b strings.Builder
	b.WriteString("kubona-manuscript-v2\n")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(abstract)
	b.WriteString("\n")
	for i, sec := range sections {
		fmt.Fprintf(&b, "section:%d\n%s\n%s\n%s\n", i, sec.Slug, sec.Title, sec.Latex)
	}
	for i, c := range citations {
		fmt.Fprintf(&b, "cite:%d\n%s\n%s\n%s\n%s\n%s\n",
			i, c.Key, c.Title, c.Authors, c.Year, c.DOI)
	}
	for i, f := range figures {
		name, _ := f["name"].(string)
		fmt.Fprintf(&b, "figure:%d\n%s\n%s\n", i, name, figureSHA[name])
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func newManuscriptID() string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	return "ms_" + hex.EncodeToString(sum[:8])
}

func nonEmptyJSON(raw []byte) string {
	if len(strings.TrimSpace(string(raw))) == 0 || string(raw) == "null" {
		return "{}"
	}
	return string(raw)
}

func cleanAuthors(in []string) []string {
	out := make([]string, 0, len(in))
	for _, a := range in {
		a = strings.TrimSpace(a)
		if a != "" {
			out = append(out, a)
		}
	}
	if len(out) == 0 {
		out = []string{"Kubona Operator"}
	}
	return out
}

func citePayload(in []manuscriptCiteIn) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, c := range in {
		out = append(out, map[string]any{
			"key": c.Key, "title": c.Title, "authors": c.Authors, "year": c.Year, "doi": c.DOI,
		})
	}
	return out
}

// figurePayload validates names and decodes base64 before the bridge sees
// them; a malformed figure is a 400 here, not a build failure later.
func figurePayload(w http.ResponseWriter, in []manuscriptFigureIn) ([]map[string]any, bool) {
	if len(in) > maxManuscriptFigures {
		WriteAPIError(w, "bad_request", fmt.Sprintf("at most %d figures", maxManuscriptFigures), nil)
		return nil, false
	}
	out := make([]map[string]any, 0, len(in))
	for _, f := range in {
		if strings.TrimSpace(f.Name) == "" || strings.Contains(f.Name, "..") || strings.Contains(f.Name, "/") {
			WriteAPIError(w, "bad_request", "figure names must be plain file names", nil)
			return nil, false
		}
		if _, err := base64.StdEncoding.DecodeString(f.ContentB64); err != nil {
			WriteAPIError(w, "bad_request", fmt.Sprintf("figure %q is not valid base64", f.Name), nil)
			return nil, false
		}
		out = append(out, map[string]any{"name": f.Name, "content_b64": f.ContentB64})
	}
	return out, true
}

// resolveManuscriptReceipts maps caller references onto the campaign's signed
// receipts. Empty refs means "the campaign's own receipts"; the campaign is
// the evidence set, not the caller's assertion. A reference that does not map
// to a signed receipt with a result hash is unresolved and earns no credit.
func (s *Server) resolveManuscriptReceipts(campaign *agents.CampaignState, refs []string) (resolved, unresolved []string) {
	type receiptRef struct {
		jobID  string
		signed bool
	}
	index := map[string]receiptRef{}
	for _, r := range campaign.ResultReceipts {
		if r.Receipt == nil {
			continue
		}
		signed := r.Receipt.Signed && r.Receipt.Signature != "" && r.Receipt.ResultSHA256 != ""
		ref := receiptRef{jobID: r.JobID, signed: signed}
		if r.JobID != "" {
			index["job:"+r.JobID] = ref
			index[r.JobID] = ref
		}
		if r.Receipt.ResultSHA256 != "" {
			index["sha256:"+r.Receipt.ResultSHA256] = ref
			index[r.Receipt.ResultSHA256] = ref
		}
	}
	if len(refs) == 0 {
		seen := map[string]bool{}
		for _, ref := range index {
			if !ref.signed || seen[ref.jobID] {
				continue
			}
			seen[ref.jobID] = true
			resolved = append(resolved, "job:"+ref.jobID)
		}
		sort.Strings(resolved)
		return resolved, nil
	}
	seen := map[string]bool{}
	for _, raw := range refs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		ref, ok := index[raw]
		if !ok || !ref.signed {
			unresolved = append(unresolved, raw)
			continue
		}
		token := "job:" + ref.jobID
		if !seen[token] {
			seen[token] = true
			resolved = append(resolved, token)
		}
	}
	sort.Strings(resolved)
	sort.Strings(unresolved)
	return resolved, unresolved
}

// resolveManuscriptProof searches the session's recent events for a
// comparator record that names this hash and attests a kernel proof with zero
// sorryAx. No production producer exists yet, so a supplied hash stays
// unverified — the honest outcome, not a bug.
func (s *Server) resolveManuscriptProof(sessionID, hash string) (string, bool) {
	if hash == "" {
		return "", false
	}
	events, err := s.ledger.GetRecentEvents(sessionID, 500)
	if err != nil {
		return hash, false
	}
	for _, e := range events {
		switch e.EventType {
		case "formal_verification", "comparator_verification", "lean_verification":
		default:
			continue
		}
		var payload map[string]any
		if json.Unmarshal(e.Payload, &payload) != nil {
			continue
		}
		candidate := strings.TrimSpace(fmt.Sprint(payload["proof_hash"]))
		if candidate == "" {
			candidate = strings.TrimSpace(fmt.Sprint(payload["lean4_hash"]))
		}
		if candidate != hash {
			continue
		}
		if v, _ := payload["verdict"].(string); v != "live_kernel_proven" {
			continue
		}
		if n, ok := payload["sorry_ax_count"].(float64); !ok || n != 0 {
			continue
		}
		if clean, ok := payload["axioms_clean"].(bool); ok && !clean {
			continue
		}
		return hash, true
	}
	return hash, false
}

func manuscriptEvidencePayload(resolved, unresolved []string, proofHash string, proofOK bool) map[string]any {
	out := map[string]any{
		"resolved_receipts":   resolved,
		"unresolved_receipts": unresolved,
	}
	if proofHash != "" {
		out["lean_proof_hash"] = proofHash
	}
	if proofOK {
		out["lean_proof_verified"] = true
		out["comparator_verdict"] = "live_kernel_proven"
		out["sorry_ax_count"] = 0
	}
	return out
}

func (s *Server) recordManuscriptReview(sessionID string, ms *ledger.Manuscript) {
	rec := reviewRecommendation(ms.ReviewJSON)
	mean, hygiene := reviewScores(ms.ReviewJSON)
	_ = s.ledger.AddManuscriptReview(sessionID, ms.ID, ledger.ManuscriptReviewRecord{
		ID:             newManuscriptID() + "-review",
		Reviewer:       "automated_peer_review",
		Recommendation: rec,
		Acceptable:     rec == "ACCEPT",
		MeanScore:      mean,
		Hygiene:        hygiene,
		EvidenceJSON:   nonEmptyJSON([]byte(ms.ReviewJSON)),
		ContentHash:    ms.ContentHash,
	})
}

func reviewRecommendation(reviewJSON string) string {
	var r struct {
		Recommendation string `json:"recommendation"`
	}
	if json.Unmarshal([]byte(nonEmptyJSON([]byte(reviewJSON))), &r) != nil || r.Recommendation == "" {
		return "REVISE"
	}
	return r.Recommendation
}

func reviewScores(reviewJSON string) (mean, hygiene float64) {
	var r struct {
		MeanScore float64 `json:"mean_score"`
		Hygiene   float64 `json:"hygiene"`
	}
	_ = json.Unmarshal([]byte(nonEmptyJSON([]byte(reviewJSON))), &r)
	return r.MeanScore, r.Hygiene
}

func normalizeExpiry(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Now().UTC().Add(30 * 24 * time.Hour).Format(time.RFC3339), nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return "", fmt.Errorf("expiry must be RFC3339")
	}
	now := time.Now().UTC()
	if !t.After(now) {
		return "", fmt.Errorf("expiry must be in the future")
	}
	if t.After(now.Add(maxManuscriptExpiry)) {
		return "", fmt.Errorf("expiry must be within %s", maxManuscriptExpiry)
	}
	return t.UTC().Format(time.RFC3339), nil
}

// handleManuscriptAnnotations serves review notes. `tail` is "" for the
// collection and the annotation id for a status change.
func (s *Server) handleManuscriptAnnotations(w http.ResponseWriter, r *http.Request, sessionID, manuscriptID, tail string) {
	if tail == "" {
		switch r.Method {
		case http.MethodGet:
			annotations, err := s.ledger.ListManuscriptAnnotations(sessionID, manuscriptID)
			if errors.Is(err, ledger.ErrManuscriptNotFound) {
				WriteAPIError(w, "not_found", "unknown manuscript", nil)
				return
			}
			if err != nil {
				WriteAPIError(w, "internal", "annotation read failed", nil)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"annotations": annotations})
		case http.MethodPost:
			var body struct {
				Anchor string `json:"anchor"`
				Body   string `json:"body"`
			}
			if err := decodeStrict(r, &body); err != nil {
				WriteAPIError(w, "bad_request", fmt.Sprintf("invalid annotation: %v", err), nil)
				return
			}
			if strings.TrimSpace(body.Body) == "" {
				WriteAPIError(w, "bad_request", "annotation body is required", nil)
				return
			}
			annotation := ledger.ManuscriptAnnotation{
				ID:     "ann_" + newManuscriptID()[3:],
				Author: s.callerActor(r),
				Anchor: strings.TrimSpace(body.Anchor),
				Body:   strings.TrimSpace(body.Body),
				Status: ledger.AnnotationOpen,
			}
			if err := s.ledger.AddManuscriptAnnotation(sessionID, manuscriptID, annotation); err != nil {
				if errors.Is(err, ledger.ErrManuscriptNotFound) {
					WriteAPIError(w, "not_found", "unknown manuscript", nil)
					return
				}
				WriteAPIError(w, "internal", "annotation persistence failed: "+err.Error(), nil)
				return
			}
			if err := s.recordManuscriptEvent(sessionID, "manuscript_annotated", map[string]any{
				"manuscript_id": manuscriptID, "annotation_id": annotation.ID,
				"anchor": annotation.Anchor,
			}); err != nil {
				WriteAPIError(w, "internal", "annotation event failed: "+err.Error(), nil)
				return
			}
			writeJSON(w, http.StatusCreated, map[string]any{"annotation": annotation})
		default:
			WriteAPIError(w, "method_not_allowed", "annotations support GET and POST", nil)
		}
		return
	}
	if strings.Contains(tail, "/") {
		WriteAPIError(w, "not_found", "unknown annotation route", nil)
		return
	}
	if r.Method != http.MethodPatch && r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "annotation status is PATCH-only", nil)
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := decodeStrict(r, &body); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid status: %v", err), nil)
		return
	}
	if err := s.ledger.SetManuscriptAnnotationStatus(sessionID, manuscriptID, tail, body.Status); err != nil {
		if errors.Is(err, ledger.ErrManuscriptNotFound) {
			WriteAPIError(w, "not_found", "unknown annotation", nil)
			return
		}
		WriteAPIError(w, "bad_request", err.Error(), nil)
		return
	}
	if err := s.recordManuscriptEvent(sessionID, "manuscript_annotation_resolved", map[string]any{
		"manuscript_id": manuscriptID, "annotation_id": tail, "status": body.Status,
	}); err != nil {
		WriteAPIError(w, "internal", "annotation event failed: "+err.Error(), nil)
		return
	}
	s.writeManuscriptDetail(w, sessionID, manuscriptID, http.StatusOK)
}

// recordManuscriptEvent appends an informational publication event with the
// durable spool fallback. A failure means neither the ledger nor the spool
// accepted it, so the caller refuses instead of leaving silent state.
func (s *Server) recordManuscriptEvent(sessionID, eventType string, payload map[string]any) error {
	// A spooled event is durable and replays on the next open; only a failure
	// of BOTH the ledger and the spool refuses the request.
	_, err := s.ledger.AppendEventWithSpool(sessionID, eventType, payload)
	return err
}

// observeManuscriptBuild records one build outcome. Metrics are observational:
// they never change the build result.
func (s *Server) observeManuscriptBuild(err error, elapsed time.Duration) {
	result := "success"
	if err != nil {
		result = "failure"
	}
	s.manuscriptMu.Lock()
	defer s.manuscriptMu.Unlock()
	if s.manuscriptBuilds == nil {
		s.manuscriptBuilds = map[string]int64{}
	}
	s.manuscriptBuilds[result]++
	s.manuscriptBuildSeconds += elapsed.Seconds()
	s.manuscriptBuildCount++
}

// handleManuscriptUpdate edits a draft's title/abstract before the first
// review. The content hash is recomputed over the stored sections/citations
// and the review is cleared, so an edited draft cannot carry a stale review.
func (s *Server) handleManuscriptUpdate(w http.ResponseWriter, r *http.Request, sessionID, id string) {
	var body struct {
		Title    string `json:"title"`
		Abstract string `json:"abstract"`
	}
	if err := decodeStrict(r, &body); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid update: %v", err), nil)
		return
	}
	ms, err := s.ledger.GetManuscript(sessionID, id)
	if errors.Is(err, ledger.ErrManuscriptNotFound) {
		WriteAPIError(w, "not_found", "unknown manuscript", nil)
		return
	}
	if err != nil {
		WriteAPIError(w, "internal", "manuscript read failed", nil)
		return
	}
	if ms.State != ledger.ManuscriptStateDraft {
		WriteAPIError(w, "manuscript_immutable",
			"only a draft can be edited; create a new draft for changed content", nil)
		return
	}
	if title := strings.TrimSpace(body.Title); title != "" {
		ms.Title = title
	}
	if abstract := strings.TrimSpace(body.Abstract); abstract != "" {
		ms.Abstract = abstract
	}
	sections := make([]manuscriptBridgeSection, 0, len(ms.Sections))
	for _, sec := range ms.Sections {
		sections = append(sections, manuscriptBridgeSection{Slug: sec.Slug, Title: sec.Title, Latex: sec.LaTeX})
	}
	var figures []map[string]any
	if ms.FiguresJSON != "" {
		if err := json.Unmarshal([]byte(ms.FiguresJSON), &figures); err != nil {
			WriteAPIError(w, "internal", "stored figures are unreadable", nil)
			return
		}
	}
	sha := map[string]string{}
	if ms.FigureSHAJSON != "" {
		if err := json.Unmarshal([]byte(ms.FigureSHAJSON), &sha); err != nil {
			WriteAPIError(w, "internal", "stored figure hashes are unreadable", nil)
			return
		}
	}
	ms.ContentHash = manuscriptContentHash(ms.Title, ms.Abstract, sections, ms.Citations, figures, sha)
	ms.ReviewJSON = "{}"
	if err := s.ledger.UpdateManuscriptDraft(ms); err != nil {
		WriteAPIError(w, "internal", "manuscript update failed: "+err.Error(), nil)
		return
	}
	if err := s.recordManuscriptEvent(sessionID, "manuscript_updated", map[string]any{
		"manuscript_id": id, "content_hash": ms.ContentHash,
	}); err != nil {
		WriteAPIError(w, "internal", "manuscript event failed: "+err.Error(), nil)
		return
	}
	s.writeManuscriptDetail(w, sessionID, id, http.StatusOK)
}

// handleManuscriptDelete removes a manuscript and its artifacts. A draft can
// be deleted by a session caller; a signed/built/deposit_ready record is
// evidence and requires the operator bearer. The deletion itself is sealed
// with the hashes before the rows are removed.
func (s *Server) handleManuscriptDelete(w http.ResponseWriter, r *http.Request, sessionID, id string) {
	ms, err := s.ledger.GetManuscript(sessionID, id)
	if errors.Is(err, ledger.ErrManuscriptNotFound) {
		WriteAPIError(w, "not_found", "unknown manuscript", nil)
		return
	}
	if err != nil {
		WriteAPIError(w, "internal", "manuscript read failed", nil)
		return
	}
	evidence := ms.Signoff != nil || ms.State == ledger.ManuscriptStateSigned ||
		ms.State == ledger.ManuscriptStateBuilt || ms.State == ledger.ManuscriptStateDepositReady
	if evidence {
		tenant, _, code := s.checkAuth(r)
		if code != http.StatusOK || tenant != "" {
			WriteAPIError(w, "forbidden", "deleting a signed or built manuscript requires the operator bearer", nil)
			return
		}
	}
	if err := s.recordManuscriptEvent(sessionID, "manuscript_deleted", map[string]any{
		"manuscript_id": id, "content_hash": ms.ContentHash, "build_hash": ms.BuildHash,
	}); err != nil {
		WriteAPIError(w, "internal", "manuscript event failed: "+err.Error(), nil)
		return
	}
	if err := s.ledger.DeleteManuscript(sessionID, id); err != nil {
		WriteAPIError(w, "internal", "manuscript delete failed: "+err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": id})
}

const defaultManuscriptArtifactCeiling = 256 << 20 // 256 MiB per session

func manuscriptArtifactCeiling() int64 {
	if raw := strings.TrimSpace(os.Getenv("KUBONA_MANUSCRIPT_ARTIFACT_CEILING_BYTES")); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return defaultManuscriptArtifactCeiling
}

// verifyBridgeFigureHashes recomputes submitted figure hashes so the content
// hash never depends on an unverified compute-plane assertion.
func verifyBridgeFigureHashes(figures []map[string]any, hashes map[string]string) error {
	for name, expected := range hashes {
		var raw []byte
		for _, f := range figures {
			if f["name"] == name {
				if b64, _ := f["content_b64"].(string); b64 != "" {
					raw, _ = base64.StdEncoding.DecodeString(b64)
				}
				break
			}
		}
		sum := sha256.Sum256(raw)
		if len(raw) == 0 || expected != hex.EncodeToString(sum[:]) {
			return fmt.Errorf("bridge figure hash does not match the submitted bytes")
		}
	}
	return nil
}

func artifactSetBytes(artifacts map[string][]byte) int64 {
	var total int64
	for _, raw := range artifacts {
		total += int64(len(raw))
	}
	return total
}

// checkArtifactCeiling bounds one session's stored artifact bytes. Replacing
// this manuscript's set counts its own previous bytes once.
func (s *Server) checkArtifactCeiling(ms *ledger.Manuscript, artifacts map[string][]byte) error {
	current, err := s.ledger.ManuscriptArtifactBytesForSession(ms.SessionID)
	if err != nil {
		return err
	}
	own, err := s.ledger.ManuscriptArtifactBytesForManuscript(ms.ID)
	if err != nil {
		return err
	}
	next := current - own + artifactSetBytes(artifacts)
	ceiling := manuscriptArtifactCeiling()
	if next > ceiling {
		return &manuscriptRefusal{
			code: "artifact_ceiling_exceeded",
			message: fmt.Sprintf("artifact set would use %d bytes, above the %d-byte session ceiling; delete old drafts or raise KUBONA_MANUSCRIPT_ARTIFACT_CEILING_BYTES",
				next, ceiling),
		}
	}
	return nil
}

// handleManuscriptCitations replaces a draft's reference list and recomposes
// the document (the introduction cites in order). Only drafts are editable;
// a claim-bound citation must resolve in this session.
func (s *Server) handleManuscriptCitations(w http.ResponseWriter, r *http.Request, sessionID, id string) {
	if r.Method != http.MethodPut {
		WriteAPIError(w, "method_not_allowed", "citations are PUT-only (replace the list)", nil)
		return
	}
	if !s.manuscriptWorkAllowed(w, r) {
		return
	}
	var body struct {
		Citations []manuscriptCiteIn `json:"citations"`
	}
	if err := decodeStrict(r, &body); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid citations: %v", err), nil)
		return
	}
	ms, err := s.ledger.GetManuscript(sessionID, id)
	if errors.Is(err, ledger.ErrManuscriptNotFound) {
		WriteAPIError(w, "not_found", "unknown manuscript", nil)
		return
	}
	if err != nil {
		WriteAPIError(w, "internal", "manuscript read failed", nil)
		return
	}
	if ms.State != ledger.ManuscriptStateDraft {
		WriteAPIError(w, "manuscript_immutable",
			"only a draft can change references; create a new draft for changed content", nil)
		return
	}
	verifiedClaims := map[string]bool{}
	for _, c := range body.Citations {
		if c.ClaimID == "" {
			continue
		}
		claim, _, _, cerr := s.ledger.ClaimProvenance(sessionID, c.ClaimID)
		if cerr != nil || claim == nil {
			WriteAPIError(w, "bad_request",
				fmt.Sprintf("citation claim %q was not found in this session", c.ClaimID), nil)
			return
		}
		verifiedClaims[c.ClaimID] = true
	}
	req, err := s.storedManuscriptRequest(ms, false)
	if err != nil {
		WriteAPIError(w, "internal", "manuscript snapshot is unreadable", nil)
		return
	}
	req.Citations = citePayload(body.Citations)
	resp, _, err := s.callManuscriptBridge(r.Context(), req)
	if err != nil {
		writeBridgeUnavailable(w, "manuscript_citations", err.Error())
		return
	}
	if !resp.OK {
		WriteAPIError(w, manuscriptErrorCode(resp.ErrorCode), resp.Error, nil)
		return
	}
	if err := verifyBridgeFigureHashes(req.Figures, resp.FigureSHA); err != nil {
		WriteAPIError(w, "bridge_unavailable", err.Error(), nil)
		return
	}
	citationInput := body.Citations
	if len(resp.Citations) > 0 {
		citationInput = resp.Citations
	}
	storedCitations := manuscriptCitations(citationInput)
	for i := range storedCitations {
		storedCitations[i].Verified = storedCitations[i].ClaimID != "" && verifiedClaims[storedCitations[i].ClaimID]
	}
	ms.Title = resp.Title
	ms.Abstract = resp.Abstract
	ms.Sections = manuscriptSections(resp.Sections)
	ms.Citations = storedCitations
	ms.ContentHash = manuscriptContentHash(resp.Title, resp.Abstract, resp.Sections, storedCitations, req.Figures, resp.FigureSHA)
	ms.ReviewJSON = nonEmptyJSON(resp.Review)
	if err := s.ledger.UpdateManuscriptDraft(ms); err != nil {
		WriteAPIError(w, "internal", "citation update failed: "+err.Error(), nil)
		return
	}
	if err := s.recordManuscriptEvent(sessionID, "manuscript_updated", map[string]any{
		"manuscript_id": id, "content_hash": ms.ContentHash, "citations": len(storedCitations),
	}); err != nil {
		WriteAPIError(w, "internal", "manuscript event failed: "+err.Error(), nil)
		return
	}
	s.writeManuscriptDetail(w, sessionID, id, http.StatusOK)
}
