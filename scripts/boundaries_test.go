package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const wireTagProbe = "package add\n\ntype probe struct {\n\tName string `json:\"name\"`\n}\n"

const siblingProbe = "package add\n\nimport _ \"github.com/residwi/go-api-project-template/internal/modules/wishlist/query\"\n"

const transportProbe = "package add\n\nimport _ \"github.com/residwi/go-api-project-template/internal/transport/http/middleware\"\n"

func TestCheckBoundaries(t *testing.T) {
	t.Run("clean tree passes", func(t *testing.T) {
		out, err := runCheck(t)
		require.NoError(t, err, out)
		assert.Contains(t, out, "Boundaries OK")
	})

	t.Run("check 1 catches a json tag outside a slice http adapter", func(t *testing.T) {
		out := runCheckWithProbe(t, "internal/modules/wishlist/add/probe_wiretag.go", wireTagProbe)
		assert.Contains(t, out, "json tag outside an http adapter")
	})

	t.Run("check 5 catches a sibling slice import", func(t *testing.T) {
		out := runCheckWithProbe(t, "internal/modules/wishlist/add/probe_sibling.go", siblingProbe)
		assert.Contains(t, out, "imports sibling slice")
	})

	t.Run("check 6 catches a module importing internal/transport", func(t *testing.T) {
		out := runCheckWithProbe(t, "internal/modules/wishlist/add/probe_transport.go", transportProbe)
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
