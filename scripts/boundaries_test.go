package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every check the script registers gets a probe here. A check that has stopped
// matching anything still prints "Boundaries OK", so a green run is only worth
// something if each check has been shown to fail on demand -- this branch
// shipped a check that could no longer match, and the probes are what would
// have caught it.
func TestCheckBoundaries(t *testing.T) {
	t.Run("clean tree passes", func(t *testing.T) {
		out, err := runCheck(t)
		require.NoError(t, err, out)
		assert.Contains(t, out, "Boundaries OK")
	})

	// Probes land at a module root, or in an adapter directory that already
	// exists. Nothing plants a directory: a probe file is removed by cleanup,
	// a probe directory would linger if the test failed hard.
	wishlistDir := filepath.Join("internal", "features", "wishlist")
	wishlistHTTPDir := filepath.Join(wishlistDir, "adapter", "http")

	t.Run("check 1a catches a json tag outside an http adapter", func(t *testing.T) {
		probe := "package wishlist\n\ntype boundaryProbe struct {\n\tName string `json:\"name\"`\n}\n"
		out := runCheckWithProbe(t, filepath.Join(wishlistDir, "probe_wiretag.go"), probe)
		assert.Contains(t, out, "json tag outside an http adapter")
	})

	// The exempt arm, proved from the other side. It is the one exemption in
	// check 1 that fifteen real modules depend on, so a probe that passes
	// inside adapter/http is what distinguishes "the exemption works" from
	// "the walk no longer reaches this path at all".
	t.Run("check 1a exempts a json tag inside adapter/http", func(t *testing.T) {
		probe := "package http\n\ntype boundaryProbe struct {\n\tName string `json:\"name\"`\n}\n"
		out := runCheckWithoutError(t, filepath.Join(wishlistHTTPDir, "probe_wiretag.go"), probe)
		assert.Contains(t, out, "Boundaries OK")
	})

	t.Run("check 1b catches a json:\"-\" tag outside an http adapter", func(t *testing.T) {
		probe := "package wishlist\n\ntype boundaryProbe struct {\n\tSecret string `json:\"-\"`\n}\n"
		out := runCheckWithProbe(t, filepath.Join(wishlistDir, "probe_hidden.go"), probe)
		assert.Contains(t, out, `json:"-" found outside an http adapter`)
	})

	t.Run("check 1c catches a resurrected dto.go", func(t *testing.T) {
		out := runCheckWithProbe(t, filepath.Join(wishlistDir, "dto.go"), "package wishlist\n")
		assert.Contains(t, out, "resurrected DTO file")
	})

	// Check 2 validates the ownership document itself, so its probe patches
	// that document rather than planting a file. Adding a migration instead
	// would be the more direct probe and is deliberately not used: `go test
	// ./...` runs packages concurrently, and a stray migration file would be
	// applied by whichever package started a container while it existed.
	t.Run("check 2 catches an ownership row no migration backs", func(t *testing.T) {
		out := runCheckWithPatchedFile(t, filepath.Join("db", "OWNERSHIP.md"),
			"<!-- END OWNERSHIP TABLE -->",
			"| `probe_boundary_table` | `wishlist` |\n<!-- END OWNERSHIP TABLE -->")
		assert.Contains(t, out, "records a table no migration creates")
	})

	// Check 3 scans every non-test .go file under a module, not only its
	// postgres adapter, which is why a probe at the module root is enough.
	t.Run("check 3 catches a module querying a table it does not own", func(t *testing.T) {
		probe := "package wishlist\n\nconst probeBoundaryQuery = `SELECT id FROM orders`\n"
		out := runCheckWithProbe(t, filepath.Join(wishlistDir, "probe_sql.go"), probe)
		assert.Contains(t, out, "cross-module table reference")
	})

	// Check 4's post-refactor rule: a module's root package is importable,
	// the way a contract/ package used to be. domain/ and every adapter stay
	// private.
	t.Run("check 4 allows a module importing another module's root package", func(t *testing.T) {
		probe := "package wishlist\n\nimport _ \"github.com/residwi/go-api-project-template/internal/features/category\"\n"
		out := runCheckWithoutError(t, filepath.Join(wishlistDir, "probe_rootimport.go"), probe)
		assert.Contains(t, out, "Boundaries OK")
	})

	t.Run("check 4 catches a module importing another module's domain", func(t *testing.T) {
		probe := "package wishlist\n\nimport _ \"github.com/residwi/go-api-project-template/internal/features/category/domain\"\n"
		out := runCheckWithProbe(t, filepath.Join(wishlistDir, "probe_domainimport.go"), probe)
		assert.Contains(t, out, "imports another module's internals")
	})

	t.Run("check 4 catches a module importing another module's postgres adapter", func(t *testing.T) {
		probe := "package wishlist\n\nimport _ \"github.com/residwi/go-api-project-template/internal/features/order/adapter/postgres\"\n"
		out := runCheckWithProbe(t, filepath.Join(wishlistDir, "probe_adapterimport.go"), probe)
		assert.Contains(t, out, "adapter/postgres")
	})

	// The wiring layer is exempt as an importer, and that exemption is what
	// lets app construct adapters at all: empty WIRING_DIRS and check 4
	// reports 30 imports in internal/app alone.
	t.Run("check 4 exempts the wiring layer as an importer", func(t *testing.T) {
		probe := "package app\n\nimport _ \"github.com/residwi/go-api-project-template/internal/features/order/adapter/postgres\"\n"
		out := runCheckWithoutError(t, filepath.Join("internal", "app", "probe_wiring.go"), probe)
		assert.Contains(t, out, "Boundaries OK")
	})

	// internal/worker is the third WIRING_DIRS entry: it constructs each
	// module's adapter/jobs worker directly, the same way app
	// constructs adapter/postgres.
	t.Run("check 4 exempts internal/worker as an importer", func(t *testing.T) {
		probe := "package worker\n\nimport _ \"github.com/residwi/go-api-project-template/internal/features/order/adapter/postgres\"\n"
		out := runCheckWithoutError(t, filepath.Join("internal", "worker", "probe_wiring.go"), probe)
		assert.Contains(t, out, "Boundaries OK")
	})

	// checkout is the script's only per-target grant: it alone may import
	// order/domain, because order.Service.Place's signature names
	// order/domain.NewOrder and order/domain.Order directly. Both subtests
	// below pin behaviour the review already confirmed holds today -- they
	// are regression pins, not a TDD cycle -- so that the grant's silent-death
	// mode (order.NewOrder and order.Order moving into order's published
	// surface, leaving the exemption an unnoticed permanent weakening) has
	// something to break.
	checkoutDir := filepath.Join("internal", "features", "checkout")

	t.Run("check 4 exempts checkout importing order/domain", func(t *testing.T) {
		probe := "package checkout\n\nimport _ \"github.com/residwi/go-api-project-template/internal/features/order/domain\"\n"
		out := runCheckWithoutError(t, filepath.Join(checkoutDir, "probe_orderdomain.go"), probe)
		assert.Contains(t, out, "Boundaries OK")
	})

	// The grant is domain/-only, not a blanket pass for checkout into order:
	// reaching into any other part of order's internals still fails.
	t.Run("check 4 does not extend checkout's exemption to order's postgres adapter", func(t *testing.T) {
		probe := "package checkout\n\nimport _ \"github.com/residwi/go-api-project-template/internal/features/order/adapter/postgres\"\n"
		out := runCheckWithProbe(t, filepath.Join(checkoutDir, "probe_orderadapter.go"), probe)
		assert.Contains(t, out, "imports another module's internals")
	})

	t.Run("check 6 catches a module importing internal/server", func(t *testing.T) {
		probe := "package wishlist\n\n" +
			"import _ \"github.com/residwi/go-api-project-template/internal/server\"\n"
		out := runCheckWithProbe(t, filepath.Join(wishlistDir, "probe_transport.go"), probe)
		assert.Contains(t, out, "imports internal/server")
	})

	// Check 8 protects the one property this refactor bought: internal/platform
	// copies into a fresh module and compiles. Nothing else proves it -- an
	// import of a module, of internal/server or of internal/apperror from
	// anywhere under platform compiles cleanly and passes every other check.
	t.Run("check 8 catches platform importing a module", func(t *testing.T) {
		probe := "package errs\n\nimport _ \"github.com/residwi/go-api-project-template/internal/features/order\"\n"
		out := runCheckWithProbe(t, filepath.Join("internal", "platform", "errs", "probe_platform.go"), probe)
		assert.Contains(t, out, "platform must not import")
	})

	// The same probe pointed at internal/app. Check 8 used to name three
	// trees it forbade -- modules, server, apperror -- which left app and
	// cmd/mockgateway/mockserver able to end the leaf property in silence. The
	// test is inverted now, so this subtest is what proves the widening is real
	// and not just a reworded comment.
	t.Run("check 8 catches platform importing the wiring layer", func(t *testing.T) {
		probe := "package errs\n\nimport _ \"github.com/residwi/go-api-project-template/internal/app\"\n"
		out := runCheckWithProbe(t, filepath.Join("internal", "platform", "errs", "probe_platform_wiring.go"), probe)
		assert.Contains(t, out, "platform must not import")
	})

	t.Run("check 8 permits platform importing platform", func(t *testing.T) {
		probe := "package errs\n\nimport _ \"github.com/residwi/go-api-project-template/internal/platform/paging\"\n"
		out := runCheckWithoutError(t, filepath.Join("internal", "platform", "errs", "probe_platform_ok.go"), probe)
		assert.Contains(t, out, "Boundaries OK")
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

// runCheckWithPatchedFile edits a tracked file in place for the duration of one
// subtest. Editing in place rather than pointing the script at a copy matters:
// the script resolves its own repository root and every path it reports is
// relative to that, so a probe outside the tree is a probe of a different tree.
func runCheckWithPatchedFile(t *testing.T, relPath, old, replacement string) string {
	t.Helper()
	full := filepath.Join(repoRoot(t), relPath)
	original, err := os.ReadFile(full)
	require.NoError(t, err)
	require.Contains(t, string(original), old, "%s no longer contains the text this probe patches", relPath)

	patched := strings.Replace(string(original), old, replacement, 1)
	require.NoError(t, os.WriteFile(full, []byte(patched), 0o600))
	t.Cleanup(func() { _ = os.WriteFile(full, original, 0o600) })

	out, err := runCheck(t)
	require.Error(t, err, "check-boundaries.sh must fail while %s is patched:\n%s", relPath, out)
	return out
}
