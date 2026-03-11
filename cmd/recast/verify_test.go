package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	ir "github.com/yourorg/recast/recast-ir"
)

func TestExtractExpectedURL(t *testing.T) {
	trace := &ir.Trace{
		Steps: []ir.Step{
			{Type: ir.StepNavigate, Value: "https://example.com/login"},
			{Type: ir.StepClick},
			{Type: ir.StepWaitForURL, Value: "https://example.com/dashboard"},
		},
	}
	assert.Equal(t, "https://example.com/dashboard", extractExpectedURL(trace))
}

func TestExtractExpectedSelectors(t *testing.T) {
	trace := &ir.Trace{
		Steps: []ir.Step{
			{
				Type: ir.StepWaitForEl,
				Target: &ir.Target{
					Primary: ir.Locator{Value: ".dashboard"},
				},
				Wait: ir.WaitSpec{Type: ir.WaitSelector, Value: ".dashboard"},
			},
			{
				Type: ir.StepAssert,
				Target: &ir.Target{
					Primary: ir.Locator{Value: "nav.main"},
				},
			},
		},
	}
	got := extractExpectedSelectors(trace, 8)
	assert.Equal(t, []string{".dashboard", "nav.main"}, got)
}

func TestRunStaticVerifyContent(t *testing.T) {
	trace := &ir.Trace{
		Steps: []ir.Step{
			{Type: ir.StepNavigate, Value: "https://example.com/login"},
			{
				Type: ir.StepWaitForEl,
				Target: &ir.Target{
					Primary: ir.Locator{Value: ".dashboard"},
				},
				Wait: ir.WaitSpec{Type: ir.WaitSelector, Value: ".dashboard"},
			},
		},
	}
	content := "await page.goto('https://example.com/login');\nawait page.locator('.dashboard').waitFor();"
	result := runStaticVerifyContent(content, trace)
	assert.True(t, result.Passed)
}

func TestSummarizeOutput(t *testing.T) {
	out := "l1\nl2\nl3\nl4\nl5"
	s := summarizeOutput(out, 3)
	assert.Equal(t, "l3 | l4 | l5", s)
}

func TestCanSkipRuntimeFailure(t *testing.T) {
	assert.True(t, canSkipRuntimeFailure("No tests found"))
	assert.True(t, canSkipRuntimeFailure("Make sure that arguments are regular expressions matching test files"))
	assert.False(t, canSkipRuntimeFailure("some unrelated failure"))
}

func TestResolvePlaywrightFile_ExactExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test_recorded_workflow.spec.ts")
	assert.NoError(t, os.WriteFile(path, []byte("test"), 0o644))

	got, note, err := resolvePlaywrightFile(path)
	assert.NoError(t, err)
	assert.Equal(t, path, got)
	assert.Equal(t, "", note)
}

func TestResolvePlaywrightFile_ClosestMatch(t *testing.T) {
	dir := t.TempDir()
	actual := filepath.Join(dir, "test_recorded_workflow_semantic.spec.ts")
	assert.NoError(t, os.WriteFile(actual, []byte("test"), 0o644))
	requested := filepath.Join(dir, "test_recorded_workflow.spec.ts")

	got, note, err := resolvePlaywrightFile(requested)
	assert.NoError(t, err)
	assert.Equal(t, actual, got)
	assert.Contains(t, note, "not found")
}
