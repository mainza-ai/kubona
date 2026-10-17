package tools

import (
	"context"
	"encoding/json"
	"testing"
)

// W2.20: unknown properties refuse unless the schema explicitly opts in
// with additionalProperties:true. Silently ignoring a misspelled argument
// would execute the wrong call with a zero value.
func TestW220UnknownPropertiesRefuse(t *testing.T) {
	r := testRegistry()
	_, err := r.Dispatch(context.Background(), "copilot", Call{Name: "get_anchor", Arguments: `{"molecule":"H2","molecul":"H2"}`})
	if err == nil {
		t.Fatal("misspelled extra property must refuse")
	}
	// Explicit opt-in still allows extras.
	r2 := NewRegistry()
	_ = r2.Register(Definition{
		Name:        "free",
		Description: "free-form",
		Parameters: map[string]any{
			"type":                 "object",
			"properties":           map[string]any{"known": map[string]any{"type": "string"}},
			"additionalProperties": true,
		},
		Capability: "test:read",
		ReadOnly:   true,
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			return map[string]any{"ok": true}, nil
		},
	})
	_ = r2.Allow("copilot", "free")
	if _, err := r2.Dispatch(context.Background(), "copilot", Call{Name: "free", Arguments: `{"known":"a","extra":"b"}`}); err != nil {
		t.Errorf("additionalProperties:true must allow extras, got %v", err)
	}
}
