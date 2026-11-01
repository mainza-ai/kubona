package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCanvasWorkbenchLayoutValidation(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()

	mk := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader([]byte(`{"title":"workbench validation"}`)))
	mw := httptest.NewRecorder()
	srv.Handler().ServeHTTP(mw, mk)
	if mw.Code != http.StatusCreated && mw.Code != http.StatusOK {
		t.Fatalf("create session: %d %s", mw.Code, mw.Body.String())
	}
	var session struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(mw.Body).Decode(&session); err != nil || session.ID == "" {
		t.Fatalf("decode session: %v", err)
	}

	put := func(body string) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodPut, "/api/canvas/layout?session_id="+session.ID, strings.NewReader(body))
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		return w.Code
	}

	if code := put(`{"workbench":{"version":1,"camera":{"x":0,"y":0,"zoom":1},"objects":{"campaign":{"x":10,"y":20},"chart":{"x":840,"y":0}}}}`); code != http.StatusOK {
		t.Fatalf("valid workbench layout: %d", code)
	}
	if code := put(`{"workbench":{"version":1,"camera":{"x":0,"y":0,"zoom":9}}}`); code != http.StatusBadRequest {
		t.Fatalf("out-of-range zoom must 400, got %d", code)
	}
	if code := put(`{"workbench":{"version":1,"objects":{"bad":{"x":"nope","y":0}}}}`); code != http.StatusBadRequest {
		t.Fatalf("non-numeric position must 400, got %d", code)
	}
	if code := put(`{"workbench":{"version":1,"objects":{"far":{"x":1000001,"y":0}}}}`); code != http.StatusBadRequest {
		t.Fatalf("out-of-bounds position must 400, got %d", code)
	}
	if code := put(`{"workbench":{"version":1,"objects":{"tiny":{"x":0,"y":0,"width":10,"height":200}}}}`); code != http.StatusBadRequest {
		t.Fatalf("out-of-bounds width must 400, got %d", code)
	}
	if code := put(`{"workbench":{"version":2}}`); code != http.StatusBadRequest {
		t.Fatalf("unknown workbench version must 400, got %d", code)
	}

	var many strings.Builder
	many.WriteString(`{"workbench":{"version":1,"objects":{`)
	for i := 0; i < 513; i++ {
		if i > 0 {
			many.WriteByte(',')
		}
		fmt.Fprintf(&many, `"o%d":{"x":0,"y":0}`, i)
	}
	many.WriteString(`}}}`)
	if code := put(many.String()); code != http.StatusBadRequest {
		t.Fatalf("513 objects must 400, got %d", code)
	}

	// Legacy payloads stay accepted.
	if code := put(`{"canvas_mode":"observatory","canvas_camera":{"position":[1,2,3],"target":[0,0,0]}}`); code != http.StatusOK {
		t.Fatalf("legacy layout must stay 200, got %d", code)
	}
}
