package globalvar

// White-box tests for the allow-list and the file filter. Both decide what is
// even considered, so a mistake produces silence rather than a wrong finding —
// the failure mode with a green gate.

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"
)

// TestDefaultAllowExemptsTheEcosystemStandardVarsFromBothChecks names
// defaultAllow's claim. These three names are structural across the fleet: an
// analyzer's Analyzer and Registration are exported by design (the whole point
// is that a suite imports them), and version is written by -ldflags at link
// time, which IS a reassignment from outside a test. Without the exemption the
// rule would report every analyzer in the ecosystem for existing.
//
// The exemption covers BOTH checks — exported AND reassigned — so an
// allow-listed name is never added to the watch set either.
func TestDefaultAllowExemptsTheEcosystemStandardVarsFromBothChecks(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"version", "Analyzer", "Registration"} {
		assert.True(t, defaultAllow[name], "%s is standard across the ecosystem and must be exempt", name)
	}

	for _, name := range []string{"Config", "Default", "logger", "state"} {
		assert.False(t, defaultAllow[name], "%s is an ordinary package-level var and must not be exempt", name)
	}
}

// TestPackageVarsSkipsTestFilesEntirely names packageVars' claim. A test file's
// exported var is the export_test.go seam — not importable by anyone — and its
// unexported vars are test-only state production code cannot reach. Judging
// either would report a violation the author cannot act on without breaking the
// seam the Go toolchain sanctions.
func TestPackageVarsSkipsTestFilesEntirely(t *testing.T) {
	t.Parallel()

	sourceOnly := checkedPass(t, map[string]string{
		"source.go": "package p\n\nvar Exported = 1\nvar unexported = 2\n",
	})
	both := checkedPass(t, map[string]string{
		"source.go":      "package p\n\nvar Exported = 1\nvar unexported = 2\n",
		"source_test.go": "package p\n\nvar ExportedInTest = 1\nvar unexportedInTest = 2\n",
	})
	testOnly := checkedPass(t, map[string]string{
		"source_test.go": "package p\n\nvar ExportedInTest = 1\nvar unexportedInTest = 2\n",
	})

	fromSourceOnly := packageVars(sourceOnly, map[string]bool{})
	fromBoth := packageVars(both, map[string]bool{})

	require.Len(t, fromSourceOnly, 1, "the source file's unexported var is watched")
	assert.Len(t, fromBoth, len(fromSourceOnly),
		"adding a test file must not add anything to the watch set")
	assert.Empty(t, packageVars(testOnly, map[string]bool{}),
		"a package of nothing but test files watches nothing")
}

// checkedPass type-checks the named fixture sources into one package and
// returns a pass carrying their syntax and types. collectDecl reads TypesInfo,
// so a real type-check is required rather than a synthetic AST.
func checkedPass(t *testing.T, sources map[string]string) *analysis.Pass {
	t.Helper()
	fset := token.NewFileSet()
	names := slices.Sorted(maps.Keys(sources))
	files := make([]*ast.File, 0, len(sources))
	for _, name := range names {
		file, err := parser.ParseFile(fset, name, sources[name], 0)
		require.NoError(t, err)
		files = append(files, file)
	}
	info := &types.Info{Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}}
	conf := types.Config{Importer: importer.Default()}
	pkg, err := conf.Check("example.test/p", fset, files, info)
	require.NoError(t, err)
	return &analysis.Pass{
		Fset:      fset,
		Files:     files,
		Pkg:       pkg,
		TypesInfo: info,
		// Exported vars are reported as they are collected, so the pass needs a
		// sink; what it reports is asserted by the corpus tests, not here.
		Report: func(analysis.Diagnostic) {},
	}
}
