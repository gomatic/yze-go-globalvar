package globalvar_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis/analysistest"

	globalvar "github.com/gomatic/yze-go-globalvar"
)

func TestPackageLevelVarsAreReported(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), globalvar.Analyzer, "a")
}

// TestBuiltinWriteTargetsMatchTheBuiltinObjectNotItsName pins the builtin
// write-through check to the predeclared objects rather than to their spelling.
// Package c declares its own delete, clear and copy, so an analyzer matching on
// name alone reports three calls this one must stay silent on — beside an index
// write it must still report, so the silence cannot come from having stopped
// watching the var.
func TestBuiltinWriteTargetsMatchTheBuiltinObjectNotItsName(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), globalvar.Analyzer, "c")
}

func TestRegistrationIsWellFormed(t *testing.T) {
	assert.NoError(t, globalvar.Registration.Validate())
	assert.Equal(t, "yze/globalvar", globalvar.Registration.RuleID())
	assert.Same(t, globalvar.Analyzer, globalvar.Registration.Analyzer)
}

func TestAllowFlagPermitsConfiguredNames(t *testing.T) {
	require.NoError(t, globalvar.Analyzer.Flags.Set("allow", "extra"))
	t.Cleanup(func() { _ = globalvar.Analyzer.Flags.Set("allow", "") })

	analysistest.Run(t, analysistest.TestData(), globalvar.Analyzer, "b")
}
