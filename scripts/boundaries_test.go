package scripts_test

import (
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

	// Every probe lands at a module root. That used to be impossible for
	// checks 1 and 6, whose probes needed a real usecase/<slice>/ directory
	// and so had to hunt for whichever module was still sliced; no module is,
	// since Task 19 flattened the last one. A module root is scanned by every
	// check below and, unlike a slice directory, is not something a later task
	// deletes.
	wishlistDir := filepath.Join("internal", "modules", "wishlist")

	wireTagProbe := "package wishlist\n\ntype probe struct {\n\tName string `json:\"name\"`\n}\n"
	transportProbe := "package wishlist\n\n" +
		"import _ \"github.com/residwi/go-api-project-template/internal/transport/http/middleware\"\n"

	t.Run("check 1 catches a json tag outside an http adapter", func(t *testing.T) {
		out := runCheckWithProbe(t, filepath.Join(wishlistDir, "probe_wiretag.go"), wireTagProbe)
		assert.Contains(t, out, "json tag outside an http adapter")
	})

	t.Run("check 6 catches a module importing internal/transport", func(t *testing.T) {
		out := runCheckWithProbe(t, filepath.Join(wishlistDir, "probe_transport.go"), transportProbe)
		assert.Contains(t, out, "imports internal/transport")
	})

	// Check 4's post-refactor rule: a module's root package is importable
	// like a contract/ package always was; domain/ and every adapter stay
	// private.
	rootImportProbe := "package wishlist\n\nimport _ \"github.com/residwi/go-api-project-template/internal/modules/category\"\n"
	domainImportProbe := "package wishlist\n\nimport _ \"github.com/residwi/go-api-project-template/internal/modules/category/domain\"\n"

	t.Run("check 4 allows a module importing another module's root package", func(t *testing.T) {
		out := runCheckWithoutError(t, filepath.Join(wishlistDir, "probe_rootimport.go"), rootImportProbe)
		assert.Contains(t, out, "Boundaries OK")
	})

	t.Run("check 4 still reports a module importing another module's domain", func(t *testing.T) {
		out := runCheckWithProbe(t, filepath.Join(wishlistDir, "probe_domainimport.go"), domainImportProbe)
		assert.Contains(t, out, "imports another module's internals")
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

func runCheckWithoutError(t *testing.T, relPath, content string) string {
	t.Helper()
	full := filepath.Join(repoRoot(t), relPath)
	require.NoError(t, os.WriteFile(full, []byte(content), 0o600))
	t.Cleanup(func() { _ = os.Remove(full) })

	out, err := runCheck(t)
	require.NoError(t, err, "check-boundaries.sh must pass while the probe file exists:\n%s", out)
	return out
}
