package scripts_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckBoundaries(t *testing.T) {
	t.Run("clean tree passes", func(t *testing.T) {
		out, err := runCheck(t)
		require.NoError(t, err, out)
		assert.Contains(t, out, "Boundaries OK")
	})

	// Checks 1, 5 and 6 need a real usecase/<slice>/ directory to drop a probe
	// file into. Every flatten task (see REFACTOR-PLAN.md) deletes one
	// module's usecase/ tree entirely -- wishlist's went first, in Task 6 --
	// so the probe target cannot be a fixed feature name; it has to be
	// whichever sliced feature is still standing when this test runs.
	feature, slice, sibling := pickSliceWithSibling(t)
	sliceDir := filepath.Join("internal", "modules", feature, "usecase", slice)

	wireTagProbe := fmt.Sprintf("package %s\n\ntype probe struct {\n\tName string `json:\"name\"`\n}\n", slice)
	siblingProbe := fmt.Sprintf(
		"package %s\n\nimport _ \"github.com/residwi/go-api-project-template/internal/modules/%s/usecase/%s\"\n",
		slice, feature, sibling,
	)
	transportProbe := fmt.Sprintf(
		"package %s\n\nimport _ \"github.com/residwi/go-api-project-template/internal/transport/http/middleware\"\n",
		slice,
	)

	t.Run("check 1 catches a json tag outside a slice http adapter", func(t *testing.T) {
		out := runCheckWithProbe(t, filepath.Join(sliceDir, "probe_wiretag.go"), wireTagProbe)
		assert.Contains(t, out, "json tag outside an http adapter")
	})

	t.Run("check 5 catches a sibling slice import", func(t *testing.T) {
		out := runCheckWithProbe(t, filepath.Join(sliceDir, "probe_sibling.go"), siblingProbe)
		assert.Contains(t, out, "imports sibling slice")
	})

	t.Run("check 6 catches a module importing internal/transport", func(t *testing.T) {
		out := runCheckWithProbe(t, filepath.Join(sliceDir, "probe_transport.go"), transportProbe)
		assert.Contains(t, out, "imports internal/transport")
	})
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("..")
	require.NoError(t, err)
	return root
}

func runCheck(t *testing.T) (string, error) {
	t.Helper()
	root := repoRoot(t)
	cmd := exec.Command(filepath.Join(root, "scripts", "check-boundaries.sh"))
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func runCheckWithProbe(t *testing.T, relPath, content string) string {
	t.Helper()
	full := filepath.Join(repoRoot(t), relPath)
	require.NoError(t, os.WriteFile(full, []byte(content), 0o600))
	t.Cleanup(func() { _ = os.Remove(full) })

	out, err := runCheck(t)
	require.Error(t, err, "check-boundaries.sh must fail while the probe file exists:\n%s", out)
	return out
}

// pickSliceWithSibling finds a feature that still has at least two
// usecase/<slice>/ directories, so the sibling-import probe names a real
// sibling. It fails loudly rather than skipping if none is left: today that
// only happens once every module in REFACTOR-PLAN.md has flattened, at which
// point check_sibling_slice_imports itself has nothing left to check and
// this whole test (Task 23, per the plan) needs rewriting anyway -- a silent
// skip would instead report a green suite for a check that quietly stopped
// running.
func pickSliceWithSibling(t *testing.T) (feature, slice, sibling string) {
	t.Helper()

	modulesRoot := filepath.Join(repoRoot(t), "internal", "modules")
	features, err := os.ReadDir(modulesRoot)
	require.NoError(t, err)

	for _, f := range features {
		if !f.IsDir() {
			continue
		}
		slices, err := os.ReadDir(filepath.Join(modulesRoot, f.Name(), "usecase"))
		if err != nil {
			continue // no usecase/ dir -- flattened, or a feature with none
		}
		var names []string
		for _, s := range slices {
			if s.IsDir() {
				names = append(names, s.Name())
			}
		}
		if len(names) >= 2 {
			return f.Name(), names[0], names[1]
		}
	}

	t.Fatal("no feature under internal/modules has two usecase/ slices left -- " +
		"every module has flattened; check_sibling_slice_imports and this probe are due for retirement")
	return "", "", ""
}
