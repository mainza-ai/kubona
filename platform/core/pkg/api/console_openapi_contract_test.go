package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
)

// Cross-language contract gate: real Go handlers are checked against the
// generated console OpenAPI response schema. Route drift is tested separately.
func TestConsoleOpenAPIResponseContracts(t *testing.T) {
	t.Setenv("KUBONA_ALLOW_EPHEMERAL_SIGNER", "1")
	dir := t.TempDir()
	l, err := ledger.OpenLedger(dir + "/contract.db")
	if err != nil {
		t.Fatalf("ledger: %v", err)
	}
	defer l.Close()
	sm := session.NewManager(l)
	sup := supervisor.NewSupervisor(l)
	srv := NewServer(l, sm, jobs.NewRunner(l, nil, 1), sup, watchdog.NewTaskWatchdog(l, sup))
	sess, err := sm.CreateSession("contract fixture", 10, "")
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	handler := srv.Handler()

	doc := loadConsoleOpenAPI(t)
	cases := []struct {
		method   string
		path     string
		template string
		status   int
	}{
		{http.MethodGet, "/api/sessions", "/api/sessions", http.StatusOK},
		{http.MethodGet, "/api/sessions/" + sess.ID + "/messages", "/api/sessions/{id}/messages", http.StatusOK},
		{http.MethodGet, "/api/jobs?session_id=" + sess.ID, "/api/jobs", http.StatusOK},
		{http.MethodGet, "/api/events?session_id=" + sess.ID, "/api/events", http.StatusOK},
		{http.MethodGet, "/api/fleet", "/api/fleet", http.StatusOK},
		{http.MethodGet, "/api/radar", "/api/radar", http.StatusOK},
		{http.MethodGet, "/api/workbench/layout?session_id=" + sess.ID, "/api/workbench/layout", http.StatusOK},
		{http.MethodGet, "/api/tasks?session_id=" + sess.ID, "/api/tasks", http.StatusOK},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Errorf("%s %s status %d, want %d: %s", tc.method, tc.path, w.Code, tc.status, strings.TrimSpace(w.Body.String()))
			continue
		}
		contract := responseContractFor(t, doc, tc.method, tc.template)
		var body any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Errorf("%s %s body is not JSON: %v", tc.method, tc.path, err)
			continue
		}
		if err := validateOpenAPISchema(doc, contract, body, "$"); err != nil {
			t.Errorf("%s %s violates %s: %v\nbody=%s", tc.method, tc.path, contract, err, w.Body.String())
		}
	}
}

func loadConsoleOpenAPI(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile("../../../contracts/console-openapi.json")
	if err != nil {
		t.Fatalf("read OpenAPI: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode OpenAPI: %v", err)
	}
	return doc
}

func responseContractFor(t *testing.T, doc map[string]any, method, path string) string {
	t.Helper()
	paths, _ := doc["paths"].(map[string]any)
	item, _ := paths[path].(map[string]any)
	op, _ := item[strings.ToLower(method)].(map[string]any)
	responses, _ := op["responses"].(map[string]any)
	resp, _ := responses["200"].(map[string]any)
	content, _ := resp["content"].(map[string]any)
	media, _ := content["application/json"].(map[string]any)
	schema, _ := media["schema"].(map[string]any)
	ref, _ := schema["$ref"].(string)
	if !strings.HasPrefix(ref, "#/components/schemas/") {
		t.Fatalf("no response schema for %s %s", method, path)
	}
	return strings.TrimPrefix(ref, "#/components/schemas/")
}

func validateOpenAPISchema(doc map[string]any, name string, value any, at string) error {
	components, _ := doc["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	root, ok := schemas[name].(map[string]any)
	if !ok {
		return fmt.Errorf("schema %q missing", name)
	}
	return validateSchemaValue(doc, root, value, at)
}

func validateSchemaValue(doc map[string]any, schema map[string]any, value any, at string) error {
	if ref, ok := schema["$ref"].(string); ok {
		name := strings.TrimPrefix(ref, "#/components/schemas/")
		components, _ := doc["components"].(map[string]any)
		schemas, _ := components["schemas"].(map[string]any)
		next, _ := schemas[name].(map[string]any)
		return validateSchemaValue(doc, next, value, at)
	}
	if value == nil {
		if nullable, _ := schema["nullable"].(bool); nullable {
			return nil
		}
	}
	if anyOf, ok := schema["anyOf"].([]any); ok && len(anyOf) > 0 {
		var errs []string
		for _, raw := range anyOf {
			part, _ := raw.(map[string]any)
			if err := validateSchemaValue(doc, part, value, at); err == nil {
				return nil
			} else {
				errs = append(errs, err.Error())
			}
		}
		return fmt.Errorf("%s matches no union member: %s", at, strings.Join(errs, "; "))
	}
	typ, _ := schema["type"].(string)
	switch typ {
	case "", "any":
		return nil
	case "object":
		obj, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s expected object, got %T", at, value)
		}
		if required, ok := schema["required"].([]any); ok {
			for _, raw := range required {
				key, _ := raw.(string)
				if _, exists := obj[key]; !exists {
					return fmt.Errorf("%s missing required %q", at, key)
				}
			}
		}
		if props, ok := schema["properties"].(map[string]any); ok {
			for key, raw := range props {
				if child, exists := obj[key]; exists {
					part, _ := raw.(map[string]any)
					if err := validateSchemaValue(doc, part, child, at+"."+key); err != nil {
						return err
					}
				}
			}
		}
		return nil
	case "array":
		list, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s expected array, got %T", at, value)
		}
		if items, ok := schema["items"].(map[string]any); ok {
			for i, child := range list {
				if err := validateSchemaValue(doc, items, child, fmt.Sprintf("%s[%d]", at, i)); err != nil {
					return err
				}
			}
		}
		return nil
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%s expected string, got %T", at, value)
		}
	case "number", "integer":
		if _, ok := value.(float64); !ok {
			return fmt.Errorf("%s expected number, got %T", at, value)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s expected boolean, got %T", at, value)
		}
	}
	return nil
}
