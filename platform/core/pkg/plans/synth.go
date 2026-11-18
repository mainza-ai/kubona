package plans

// Automatic experiment-spec synthesis (AG-3b).
//
// A hypothesis says what to test; this package turns the executable ones into
// the concrete job spec the bridge already understands. Two inputs, never
// invented:
//
//   1. campaign facts — molecule, geometry, prior layers/shots/seed, and the
//      last spin-chain size, all read from the measured campaign state;
//   2. the hypothesis's declared patch — the intervention that would test it
//      (one more layer, four times the shots, a larger optimizer budget).
//
// Every value records where it came from in the returned basis, which the plan
// persists. A kind this package cannot ground is refused instead of receiving
// a plausible-looking spec.

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Facts are the campaign-derived inputs. Zero values mean "not recorded"; the
// synthesizer then uses the bridge's documented default and says so.
type Facts struct {
	TargetSystem  string
	Goal          string
	Molecule      string
	BondDistance  float64
	Ansatz        string
	Shots         int
	Seed          int
	MaxIterations int
	NSites        int
	Model         string
	Coupling      float64
	Field         float64
	Periodic      *bool
}

// Request is the hypothesis side: the executable method plus its declared
// intervention.
type Request struct {
	Method string
	Hint   string
	Patch  map[string]any
}

// Patch keys are relative to the campaign baseline so a hypothesis does not
// need to know the absolute value it is changing.
const (
	PatchLayersDelta          = "layers_delta"
	PatchShotsMultiplier      = "shots_multiplier"
	PatchIterationsMultiplier = "iterations_multiplier"
	PatchRepeats              = "repeats"
)

var ansatzLayers = regexp.MustCompile(`(?i)layers\s*=\s*([0-9]+)`)

// ParseAnsatzLayers extracts the layer count the campaign's measured run used
// (the bridge records "hardware-efficient layers=N"). It reports false rather
// than guessing when the ansatz carries no count.
func ParseAnsatzLayers(ansatz string) (int, bool) {
	m := ansatzLayers.FindStringSubmatch(ansatz)
	if len(m) != 2 {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n < 1 || n > 64 {
		return 0, false
	}
	return n, true
}

// Synthesize builds the spec and its provenance. The method must be one this
// synthesizer can ground; anything else is a typed refusal.
func Synthesize(req Request, f Facts) (map[string]any, []string, error) {
	method := strings.ToLower(strings.TrimSpace(req.Method))
	basis := []string{}
	spec := map[string]any{"kind": method}

	layers := 2
	if n, ok := ParseAnsatzLayers(f.Ansatz); ok {
		layers = n
		basis = append(basis, fmt.Sprintf("layers=%d from the campaign's measured ansatz", n))
	} else {
		basis = append(basis, "layers=2 from the bridge default (no measured ansatz layer count)")
	}
	shots := f.Shots
	if shots <= 0 {
		shots = 4096
		basis = append(basis, "shots=4096 from the bridge default (no measured shot count)")
	} else {
		basis = append(basis, fmt.Sprintf("shots=%d from the campaign measurement", shots))
	}
	seed := f.Seed
	if seed <= 0 {
		seed = 7
		basis = append(basis, "seed=7 from the bridge default (no measured seed)")
	} else {
		basis = append(basis, fmt.Sprintf("seed=%d from the campaign measurement", seed))
	}
	if err := applyPatch(req.Patch, &layers, &shots); err != nil {
		return nil, nil, err
	}
	if req.Patch != nil {
		if _, ok := req.Patch[PatchLayersDelta]; ok {
			basis = append(basis, fmt.Sprintf("layers changed to %d by the hypothesis patch", layers))
		}
		if _, ok := req.Patch[PatchShotsMultiplier]; ok {
			basis = append(basis, fmt.Sprintf("shots changed to %d by the hypothesis patch", shots))
		}
	}

	switch method {
	case "vqe":
		molecule, bond, note, err := moleculeFacts(f)
		if err != nil {
			return nil, nil, err
		}
		basis = append(basis, note)
		iterations := defaultIterations(f.MaxIterations, 120)
		if err := applyIterations(req.Patch, &iterations); err != nil {
			return nil, nil, err
		}
		spec["molecule"], spec["bond_distance"] = molecule, bond
		spec["layers"], spec["shots"], spec["seed"] = layers, shots, seed
		spec["max_iterations"] = iterations
		basis = append(basis, fmt.Sprintf("max_iterations=%d (%s)", iterations, "bridge default or hypothesis patch"))
	case "vqe_repeats":
		molecule, bond, note, err := moleculeFacts(f)
		if err != nil {
			return nil, nil, err
		}
		basis = append(basis, note)
		iterations := defaultIterations(f.MaxIterations, 80)
		if err := applyIterations(req.Patch, &iterations); err != nil {
			return nil, nil, err
		}
		repeats, note2, err := repeatsFromPatch(req.Patch)
		if err != nil {
			return nil, nil, err
		}
		basis = append(basis, note2)
		spec["molecule"], spec["bond_distance"] = molecule, bond
		spec["layers"], spec["shots"], spec["seed"] = layers, shots, seed
		spec["max_iterations"], spec["repeats"] = iterations, repeats
	case "geometry_optimization":
		molecule, bond, note, err := moleculeFacts(f)
		if err != nil {
			return nil, nil, err
		}
		basis = append(basis, note)
		iterations := defaultIterations(f.MaxIterations, 80)
		if err := applyIterations(req.Patch, &iterations); err != nil {
			return nil, nil, err
		}
		spec["molecule"], spec["initial_bond_distance"] = molecule, bond
		spec["layers"], spec["shots"], spec["seed"] = layers, shots, seed
		spec["max_iterations"], spec["max_geometry_steps"] = iterations, 50
		basis = append(basis, "max_geometry_steps=50 from the bridge default")
	case "spin_chain":
		n := f.NSites
		if n < 2 || n > 20 {
			n = 4
			basis = append(basis, "n_sites=4 from the bridge default (no measured chain size)")
		} else {
			basis = append(basis, fmt.Sprintf("n_sites=%d from the campaign's last spin-chain run", n))
		}
		model := strings.TrimSpace(f.Model)
		if model == "" {
			model = "tfim"
		}
		coupling, field := f.Coupling, f.Field
		if coupling == 0 {
			coupling = 1.0
		}
		if field == 0 {
			field = 1.0
		}
		periodic := true
		if f.Periodic != nil {
			periodic = *f.Periodic
		}
		iterations := defaultIterations(f.MaxIterations, 160)
		if err := applyIterations(req.Patch, &iterations); err != nil {
			return nil, nil, err
		}
		spec["model"], spec["n_sites"] = model, n
		spec["coupling"], spec["field"], spec["periodic"] = coupling, field, periodic
		spec["layers"], spec["shots"], spec["seed"] = layers, shots, seed
		spec["max_iterations"], spec["target_error"] = iterations, 0.005
		basis = append(basis, fmt.Sprintf("model=%s coupling=%g field=%g periodic=%t from campaign/default", model, coupling, field, periodic))
	case "qaoa":
		iterations := defaultIterations(f.MaxIterations, 80)
		if err := applyIterations(req.Patch, &iterations); err != nil {
			return nil, nil, err
		}
		spec["layers"], spec["shots"], spec["seed"] = layers, shots, seed
		spec["max_iterations"], spec["graph_seed"] = iterations, 3
		basis = append(basis, "graph_seed=3 from the bridge default")
	default:
		return nil, nil, fmt.Errorf("no grounded spec synthesis for method %q: provide an explicit spec", method)
	}
	return spec, basis, nil
}

func moleculeFacts(f Facts) (string, float64, string, error) {
	molecule := strings.TrimSpace(f.Molecule)
	if molecule == "" {
		return "", 0, "", fmt.Errorf("the campaign has no target molecule: provide an explicit spec")
	}
	bond := f.BondDistance
	if bond <= 0 {
		return "", 0, "", fmt.Errorf("the campaign has no target bond distance: provide an explicit spec")
	}
	if bond < 0.3 || bond > 10.0 {
		return "", 0, "", fmt.Errorf("campaign bond distance %g is outside [0.3, 10.0]", bond)
	}
	note := fmt.Sprintf("molecule=%s bond_distance=%g from the campaign target", molecule, bond)
	return molecule, bond, note, nil
}

func defaultIterations(recorded, def int) int {
	if recorded > 0 && recorded <= 1000 {
		return recorded
	}
	return def
}

func applyIterations(patch map[string]any, iterations *int) error {
	if patch == nil {
		return nil
	}
	raw, ok := patch[PatchIterationsMultiplier]
	if !ok {
		return nil
	}
	m, err := number(raw)
	if err != nil {
		return fmt.Errorf("patch %s: %w", PatchIterationsMultiplier, err)
	}
	if m < 1 || m > 8 {
		return fmt.Errorf("patch %s must be in [1,8]", PatchIterationsMultiplier)
	}
	*iterations = int(math.Round(float64(*iterations) * m))
	if *iterations > 1000 {
		*iterations = 1000
	}
	return nil
}

func repeatsFromPatch(patch map[string]any) (int, string, error) {
	repeats := 3
	if patch == nil {
		return repeats, "repeats=3 from the bridge default", nil
	}
	raw, ok := patch[PatchRepeats]
	if !ok {
		return repeats, "repeats=3 from the bridge default", nil
	}
	n, err := number(raw)
	if err != nil {
		return 0, "", fmt.Errorf("patch %s: %w", PatchRepeats, err)
	}
	if n < 2 || n > 10 {
		return 0, "", fmt.Errorf("patch %s must be in [2,10]", PatchRepeats)
	}
	return int(n), fmt.Sprintf("repeats=%d from the hypothesis patch", int(n)), nil
}

func applyPatch(patch map[string]any, layers, shots *int) error {
	if patch == nil {
		return nil
	}
	if raw, ok := patch[PatchLayersDelta]; ok {
		d, err := number(raw)
		if err != nil {
			return fmt.Errorf("patch %s: %w", PatchLayersDelta, err)
		}
		if d < -4 || d > 4 {
			return fmt.Errorf("patch %s must be in [-4,4]", PatchLayersDelta)
		}
		*layers += int(d)
		if *layers < 1 || *layers > 16 {
			return fmt.Errorf("patched layers %d is outside [1,16]", *layers)
		}
	}
	if raw, ok := patch[PatchShotsMultiplier]; ok {
		m, err := number(raw)
		if err != nil {
			return fmt.Errorf("patch %s: %w", PatchShotsMultiplier, err)
		}
		if m < 1 || m > 16 {
			return fmt.Errorf("patch %s must be in [1,16]", PatchShotsMultiplier)
		}
		*shots = int(math.Round(float64(*shots) * m))
		if *shots < 128 || *shots > 1_000_000 {
			return fmt.Errorf("patched shots %d is outside [128,1000000]", *shots)
		}
	}
	return nil
}

func number(v any) (float64, error) {
	switch n := v.(type) {
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case float64:
		return n, nil
	case json.Number:
		return n.Float64()
	}
	return 0, fmt.Errorf("value %v is not a number", v)
}
