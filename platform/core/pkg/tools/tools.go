package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

// Handler executes one tool call. Args are the raw JSON arguments object;
// the returned value must be JSON-marshalable and is fed back to the model
// as the observation. Handlers must be fail-closed: unknown inputs refuse,
// never guess.
type Handler func(ctx context.Context, args json.RawMessage) (any, error)

// Definition is one registered tool: its LLM-facing contract plus the
// server-side implementation. Capability names the allow-list scope
// (e.g. "anchor:read", "jobs:read") checked at dispatch.
type Definition struct {
	Name        string
	Description string
	// Parameters is a JSON-Schema-like map (type/properties/required).
	Parameters map[string]any
	Capability string
	ReadOnly   bool
	Handler    Handler
}

// Snapshot describes one registered tool for catalog generation
// (R5/C5): name, capability, read-only flag, and granted roles.
type Snapshot struct {
	Name       string   `json:"name"`
	Capability string   `json:"capability"`
	ReadOnly   bool     `json:"read_only"`
	Roles      []string `json:"roles"`
}

// Catalog returns a sorted snapshot of the registry (tools by name,
// roles sorted). The R5 tool catalog + capabilities digest build on
// this — one accessor, never registry internals elsewhere.
func (r *Registry) Catalog() []Snapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	byName := map[string]*Snapshot{}
	for name, def := range r.defs {
		byName[name] = &Snapshot{Name: name, Capability: def.Capability, ReadOnly: def.ReadOnly}
	}
	for role, names := range r.roles {
		for _, n := range names {
			if s, ok := byName[n]; ok {
				s.Roles = append(s.Roles, role)
			}
		}
	}
	out := make([]Snapshot, 0, len(byName))
	for _, s := range byName {
		sort.Strings(s.Roles)
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// CatalogJSON renders snapshots in catalog wire shape (sorted tools,
// sorted roles — Catalog guarantees both) plus the sha256 digest that
// pins the surface. Single source for the checked-in file, the embedded
// const, and the capabilities endpoint: all three hash THESE bytes, so
// they cannot disagree on the algorithm, only on staleness (which 500s).
func CatalogJSON(snaps []Snapshot) ([]byte, string) {
	body, err := json.Marshal(snaps)
	if err != nil {
		return nil, ""
	}
	sum := sha256.Sum256(body)
	return body, hex.EncodeToString(sum[:])
}

type Call struct {
	ID        string
	Name      string
	Arguments string
}

// Registry is the single tool registry for the platform. It replaces the
// disconnected allow-lists (RoleManifest.AllowedTools, MCP catalog): one
// name → one contract → one implementation, enforced at dispatch.
type Registry struct {
	mu   sync.RWMutex
	defs map[string]Definition
	// roles maps a caller role to its allowed tool names. Deny by default:
	// a role with no entry may call nothing.
	roles map[string][]string
}

func NewRegistry() *Registry {
	return &Registry{defs: map[string]Definition{}, roles: map[string][]string{}}
}

func (r *Registry) Register(d Definition) error {
	if d.Name == "" || d.Handler == nil {
		return fmt.Errorf("tool registration requires name and handler")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.defs[d.Name]; exists {
		return fmt.Errorf("tool %q already registered", d.Name)
	}
	r.defs[d.Name] = d
	return nil
}

// Allow grants a role permission to call the named tools. Unknown tool
// names refuse at grant time so typos fail loudly, not silently.
func (r *Registry) Allow(role string, names ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, n := range names {
		if _, ok := r.defs[n]; !ok {
			return fmt.Errorf("cannot allow unknown tool %q for role %q", n, role)
		}
	}
	r.roles[role] = append(r.roles[role], names...)
	return nil
}

func (r *Registry) allowed(role, name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, n := range r.roles[role] {
		if n == name {
			return true
		}
	}
	return false
}

// DefinitionsForLLM renders the OpenAI-compatible tools array for chat
// requests. Only tools the role may call are included.
func (r *Registry) DefinitionsForLLM(role string) []map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []map[string]any
	for _, name := range r.roles[role] {
		d, ok := r.defs[name]
		if !ok {
			continue
		}
		params := d.Parameters
		if params == nil {
			params = map[string]any{"type": "object"}
		}
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        d.Name,
				"description": d.Description,
				"parameters":  params,
			},
		})
	}
	sort.Slice(out, func(i, j int) bool {
		fi, _ := out[i]["function"].(map[string]any)
		fj, _ := out[j]["function"].(map[string]any)
		ni, _ := fi["name"].(string)
		nj, _ := fj["name"].(string)
		return ni < nj
	})
	return out
}

// Dispatch validates (role → allow-list → argument schema) then executes.
// Every refusal names its reason; nothing executes on a failed check.
func (r *Registry) Dispatch(ctx context.Context, role string, call Call) (any, error) {
	r.mu.RLock()
	def, ok := r.defs[call.Name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown tool %q: refused", call.Name)
	}
	if !r.allowed(role, call.Name) {
		return nil, fmt.Errorf("role %q may not call tool %q: refused", role, call.Name)
	}
	args := json.RawMessage(call.Arguments)
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	if err := checkSchema(def.Parameters, args); err != nil {
		return nil, fmt.Errorf("tool %q argument validation failed: %w", call.Name, err)
	}
	out, err := def.Handler(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("tool %q failed: %w", call.Name, err)
	}
	return out, nil
}

// checkSchema validates args against a minimal JSON-Schema subset
// (type: object, properties with scalar types, required[]). Enough for
// fail-closed dispatch with zero new dependencies. W2.20 strictness: the
// top level must be a JSON object, and unknown properties refuse —
// silently ignoring a misspelled argument would execute the wrong call
// with a zero value (e.g. wrong molecule). Schemas that genuinely accept
// free-form keys must declare "additionalProperties": true explicitly;
// anything else (absent or false) refuses unknown fields.
func checkSchema(schema map[string]any, args json.RawMessage) error {
	if len(schema) == 0 {
		return nil
	}
	var decoded any
	if err := json.Unmarshal(args, &decoded); err != nil {
		return fmt.Errorf("arguments are not valid JSON: %w", err)
	}
	obj, ok := decoded.(map[string]any)
	if !ok {
		return fmt.Errorf("arguments must be a JSON object")
	}
	if t, _ := schema["type"].(string); t != "" && t != "object" {
		return fmt.Errorf("unsupported top-level schema type %q", t)
	}
	props, _ := schema["properties"].(map[string]any)
	allowExtra, _ := schema["additionalProperties"].(bool)
	// Generic {"type":"object"} with no declared properties constrains
	// nothing (legacy test tools use exactly this) — unknown-refusal
	// applies only when the schema declares properties.
	strictProps := len(props) > 0 && !allowExtra
	if req, ok := schema["required"].([]any); ok {
		for _, item := range req {
			name, _ := item.(string)
			if name == "" {
				continue
			}
			v, present := obj[name]
			if !present || v == nil {
				return fmt.Errorf("missing required argument %q", name)
			}
		}
	}
	for name, value := range obj {
		spec, ok := props[name].(map[string]any)
		if !ok {
			if strictProps {
				return fmt.Errorf("unknown argument %q: refused (not in tool schema)", name)
			}
			continue
		}
		want, _ := spec["type"].(string)
		switch want {
		case "", "any":
		case "string":
			if _, ok := value.(string); !ok {
				return fmt.Errorf("argument %q must be a string", name)
			}
		case "number":
			if _, ok := value.(float64); !ok {
				return fmt.Errorf("argument %q must be a number", name)
			}
		case "integer":
			if f, ok := value.(float64); !ok || f != float64(int(f)) {
				return fmt.Errorf("argument %q must be an integer", name)
			}
		case "boolean":
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("argument %q must be a boolean", name)
			}
		default:
			return fmt.Errorf("unsupported schema type %q for %q", want, name)
		}
	}
	return nil
}
