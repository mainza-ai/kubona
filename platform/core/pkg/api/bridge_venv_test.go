package api

// CI 2026-10-22: findPythonVenv returned a nonexistent dir whenever no
// dev venv existed, so every exec failed chdir and the rails 503d with
// no named cause. The invariant: the returned dir always exists.
import (
	"os"
	"testing"
)

func TestFindPythonVenvDirAlwaysExists(t *testing.T) {
	for i := 0; i < 3; i++ {
		_, dir := findPythonVenv()
		st, err := os.Stat(dir)
		if err != nil || !st.IsDir() {
			t.Fatalf("findPythonVenv must return an existing dir, got %q: %v", dir, err)
		}
	}
}
