package core_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// TestArchitecturalFitness locks the Capability-Core contract (Q-28): core
// packages must never import adapter packages. Adapters (api, agents, mcp,
// hf) may call inward; enforcement and compute never call outward. Uses
// go/parser over source so the test needs no toolchain at runtime.
func TestArchitecturalFitness(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	pkgDir := filepath.Dir(thisFile)

	corePackages := []string{
		"ledger", "jobs", "triage", "mandates", "epistemics", "fidelity",
		"ifc", "qec", "session", "tenants", "watchdog", "cost", "memory",
		"qir", "sandbox", "vault", "metasearch", "autoresearch", "llm",
		"supervisor",
	}
	adapterPrefixes := []string{
		"github.com/mainza-ai/kubona/core/pkg/api",
		"github.com/mainza-ai/kubona/core/pkg/agents",
		"github.com/mainza-ai/kubona/core/pkg/mcp",
		"github.com/mainza-ai/kubona/core/pkg/hf",
	}

	fset := token.NewFileSet()
	for _, pkg := range corePackages {
		dir := filepath.Join(pkgDir, pkg)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("core package dir missing: %s", dir)
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse %s: %v", name, err)
			}
			for _, imp := range f.Imports {
				path, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					t.Fatalf("unquote import in %s: %v", name, err)
				}
				for _, prefix := range adapterPrefixes {
					if path == prefix || strings.HasPrefix(path, prefix+"/") {
						t.Errorf("architectural violation: core package %q file %s imports adapter %q (adapters call inward only)", pkg, name, path)
					}
				}
			}
		}
	}
}
