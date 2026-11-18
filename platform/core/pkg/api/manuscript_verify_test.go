package api_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	pb "github.com/mainza-ai/kubona/core/proto"
)

func TestManuscriptVerificationFlagsFabricatedNumber(t *testing.T) {
	bridge := &fakeManuscriptBridge{sectionLatex: "The measured energy was -1.1373 Ha."}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedCampaign(t, srv, sid)
	if _, err := srv.Ledger().MintClaimRecord(sid, "claim_eval", "H2 measured",
		0.9, 0.05, 0.05, "verified", `{"energy": -1.13727}`); err != nil {
		t.Fatalf("seed claim: %v", err)
	}

	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts", "", map[string]any{})
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", w.Code, w.Body.String())
	}
	var created struct {
		Manuscript struct {
			ID          string `json:"id"`
			ContentHash string `json:"content_hash"`
		} `json:"manuscript"`
	}
	json.Unmarshal(w.Body.Bytes(), &created)
	mid := created.Manuscript.ID

	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/manuscripts/"+mid+"/verify", "", nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":"not_verified"`) {
		t.Fatalf("unverified read = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts/"+mid+"/verify", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("verify = %d: %s", w.Code, w.Body.String())
	}
	var verified struct {
		Status string `json:"status"`
		Report struct {
			Numbers []struct {
				Status      string  `json:"status"`
				MatchedPath string  `json:"matched_path"`
				Value       float64 `json:"value"`
			} `json:"numbers"`
			Summary struct {
				NumbersChecked int `json:"numbers_checked"`
				Blocking       int `json:"blocking"`
			} `json:"summary"`
		} `json:"report"`
	}
	json.Unmarshal(w.Body.Bytes(), &verified)
	if verified.Status != "verified" || len(verified.Report.Numbers) != 1 ||
		verified.Report.Numbers[0].Status != "matched" || verified.Report.Numbers[0].MatchedPath != "energy" {
		t.Fatalf("supported number = %+v", verified)
	}

	// A fabricated number must block signoff.
	bridge.sectionLatex = "The measured energy was -1.9000 Ha."
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts", "", map[string]any{})
	if w.Code != http.StatusCreated {
		t.Fatalf("create fabricated = %d: %s", w.Code, w.Body.String())
	}
	json.Unmarshal(w.Body.Bytes(), &created)
	bad := created.Manuscript.ID
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts/"+bad+"/verify", "", nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":"blocked"`) {
		t.Fatalf("fabricated verify = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts/"+bad+"/signoff", "op-token",
		map[string]any{"content_hash": created.Manuscript.ContentHash})
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "evidence_verification_failed") {
		t.Fatalf("fabricated signoff = %d: %s", w.Code, w.Body.String())
	}
}

func TestManuscriptCitationVerificationGatesSignoff(t *testing.T) {
	bridge := &fakeManuscriptBridge{
		literature: []*pb.LiteratureRecord{{Id: "10.1/abc", Title: "Prior H2 work"}},
	}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedCampaign(t, srv, sid)

	citation := []map[string]any{{"key": "prior", "title": "Prior H2 work", "doi": "https://doi.org/10.1/ABC"}}
	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts", "", map[string]any{"citations": citation})
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", w.Code, w.Body.String())
	}
	var created struct {
		Manuscript struct {
			ID          string `json:"id"`
			ContentHash string `json:"content_hash"`
		} `json:"manuscript"`
	}
	json.Unmarshal(w.Body.Bytes(), &created)
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts/"+created.Manuscript.ID+"/verify", "", nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":"verified"`) {
		t.Fatalf("doi verify = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts/"+created.Manuscript.ID+"/signoff", "op-token",
		map[string]any{"content_hash": created.Manuscript.ContentHash})
	if w.Code != http.StatusOK {
		t.Fatalf("verified citation signoff = %d: %s", w.Code, w.Body.String())
	}

	// A DOI that resolves to another work blocks signoff.
	bridge.literature = []*pb.LiteratureRecord{{Id: "10.1/other", Title: "Prior H2 work"}}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts", "", map[string]any{"citations": citation})
	if w.Code != http.StatusCreated {
		t.Fatalf("create mismatch = %d: %s", w.Code, w.Body.String())
	}
	json.Unmarshal(w.Body.Bytes(), &created)
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts/"+created.Manuscript.ID+"/verify", "", nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"doi_mismatch"`) {
		t.Fatalf("mismatch verify = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts/"+created.Manuscript.ID+"/signoff", "op-token",
		map[string]any{"content_hash": created.Manuscript.ContentHash})
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "evidence_verification_failed") {
		t.Fatalf("mismatch signoff = %d: %s", w.Code, w.Body.String())
	}

	// An unavailable resolver refuses until explicitly acknowledged.
	bridge.literatureErr = errors.New("openalex offline")
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts", "", map[string]any{"citations": citation})
	if w.Code != http.StatusCreated {
		t.Fatalf("create unavailable = %d: %s", w.Code, w.Body.String())
	}
	json.Unmarshal(w.Body.Bytes(), &created)
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts/"+created.Manuscript.ID+"/verify", "", nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":"unverified"`) {
		t.Fatalf("unavailable verify = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts/"+created.Manuscript.ID+"/signoff", "op-token",
		map[string]any{"content_hash": created.Manuscript.ContentHash})
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "verification_unavailable") {
		t.Fatalf("unavailable signoff = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts/"+created.Manuscript.ID+"/signoff", "op-token",
		map[string]any{"content_hash": created.Manuscript.ContentHash, "acknowledge_unverified": true})
	if w.Code != http.StatusOK {
		t.Fatalf("acknowledged signoff = %d: %s", w.Code, w.Body.String())
	}
	var signed struct {
		Manuscript struct {
			Verification *struct {
				Summary struct {
					Status string `json:"status"`
				} `json:"summary"`
			} `json:"verification"`
		} `json:"manuscript"`
	}
	json.Unmarshal(w.Body.Bytes(), &signed)
	if signed.Manuscript.Verification == nil || signed.Manuscript.Verification.Summary.Status != "unverified" {
		t.Fatalf("verification report not surfaced: %s", w.Body.String())
	}
}

func TestManuscriptVerificationRejectsUnknownPossession(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedCampaign(t, srv, sid)
	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts", "", map[string]any{})
	var created struct {
		Manuscript struct {
			ID string `json:"id"`
		} `json:"manuscript"`
	}
	json.Unmarshal(w.Body.Bytes(), &created)
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts/"+created.Manuscript.ID+"/verify", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("verify = %d: %s", w.Code, w.Body.String())
	}
	var out struct {
		Report struct {
			ContentHash string `json:"content_hash"`
		} `json:"report"`
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	if out.Report.ContentHash == "" {
		t.Fatalf("report not bound to the content hash: %s", w.Body.String())
	}
	// The stored report is bound to the ledger's current content hash.
	ms, err := srv.Ledger().GetManuscript(sid, created.Manuscript.ID)
	if err != nil || !strings.Contains(ms.VerificationJSON, out.Report.ContentHash) {
		t.Fatalf("stored report = %v (%v)", ms, err)
	}
}
