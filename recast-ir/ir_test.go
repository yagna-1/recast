package ir_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ir "github.com/yagna-1/recast/recast-ir"
)

func TestTraceBuilder_BasicWorkflow(t *testing.T) {
	trace, result := ir.NewTrace("test_workflow").
		WithBaseURL("https://example.com").
		WithSourceFormat("workflow-use").
		Navigate("https://example.com/login").
		Fill(
			ir.TargetFromLabel("Email address", "the email input"),
			"user@example.com",
		).
		Click(ir.TargetFromRole("button", "Sign in", "the Sign in button")).
		Build()

	require.NotNil(t, trace)
	assert.False(t, result.HasErrors())
	assert.Equal(t, "test_workflow", trace.Name)
	assert.Equal(t, ir.SchemaVersion, trace.SchemaVersion)
	assert.Len(t, trace.Steps, 3)

	assert.Equal(t, ir.StepNavigate, trace.Steps[0].Type)
	assert.Equal(t, "https://example.com/login", trace.Steps[0].Value)
	assert.Equal(t, "step_001", trace.Steps[0].ID)

	assert.Equal(t, ir.StepFill, trace.Steps[1].Type)
	assert.Equal(t, ir.LocatorLabel, trace.Steps[1].Target.Primary.Strategy)

	assert.Equal(t, ir.StepClick, trace.Steps[2].Type)
	assert.Equal(t, ir.LocatorRole, trace.Steps[2].Target.Primary.Strategy)
}

func TestValidate_EmptyTrace(t *testing.T) {
	trace := &ir.Trace{Name: "empty"}
	result := ir.Validate(trace)
	assert.True(t, result.HasErrors())
	assert.Contains(t, result.Error(), "no steps")
}

func TestValidate_NilTrace(t *testing.T) {
	result := ir.Validate(nil)
	assert.True(t, result.HasErrors())
}

func TestValidate_NavigateNoURL(t *testing.T) {
	trace := &ir.Trace{
		Name:          "test",
		SchemaVersion: ir.SchemaVersion,
		Steps: []ir.Step{
			{ID: "step_001", Type: ir.StepNavigate, Value: ""},
		},
	}
	result := ir.Validate(trace)
	assert.True(t, result.HasErrors())
	assert.Contains(t, result.Error(), "navigate step has no URL")
}

func TestValidate_ClickNoTarget(t *testing.T) {
	trace := &ir.Trace{
		Name:          "test",
		SchemaVersion: ir.SchemaVersion,
		Steps: []ir.Step{
			{ID: "step_001", Type: ir.StepClick},
		},
	}
	result := ir.Validate(trace)
	assert.True(t, result.HasErrors())
	assert.Contains(t, result.Error(), "click step has no target")
}

func TestValidate_ScreenshotNoTarget(t *testing.T) {
	trace := &ir.Trace{
		Name:          "test",
		SchemaVersion: ir.SchemaVersion,
		Steps: []ir.Step{
			{ID: "step_001", Type: ir.StepScreenshot, Value: "shot.png"},
		},
	}
	result := ir.Validate(trace)
	assert.False(t, result.HasErrors())
}

func TestValidate_CoordinatePrimaryRejected(t *testing.T) {
	trace := &ir.Trace{
		Name:          "test",
		SchemaVersion: ir.SchemaVersion,
		Steps: []ir.Step{
			{
				ID:   "step_001",
				Type: ir.StepClick,
				Target: &ir.Target{
					Primary: ir.Locator{
						Strategy:   ir.LocatorCoords,
						Value:      "100,200",
						Confidence: 0.05,
					},
				},
			},
		},
	}
	result := ir.Validate(trace)
	assert.True(t, result.HasErrors())
	assert.Contains(t, result.Error(), "coordinate-only locator cannot be primary")
}

func TestValidate_InternalURLRejected(t *testing.T) {
	trace := &ir.Trace{
		Name:          "test",
		SchemaVersion: ir.SchemaVersion,
		Steps: []ir.Step{
			{ID: "step_001", Type: ir.StepNavigate, Value: "chrome://settings"},
		},
	}
	result := ir.Validate(trace)
	assert.True(t, result.HasErrors())
	assert.Contains(t, result.Error(), "internal browser URL")
}

func TestMarshalUnmarshal(t *testing.T) {
	trace := ir.NewTrace("marshal_test").
		WithBaseURL("https://example.com").
		WithSourceFormat("test").
		Navigate("https://example.com").
		BuildUnchecked()
	trace.Metadata.RecordedAt = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	data, err := ir.Marshal(trace)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"schema_version"`)
	assert.Contains(t, string(data), `"marshal_test"`)

	restored, err := ir.Unmarshal(data)
	require.NoError(t, err)
	assert.Equal(t, trace.Name, restored.Name)
	assert.Equal(t, trace.BaseURL, restored.BaseURL)
	assert.Equal(t, len(trace.Steps), len(restored.Steps))
}

func TestTargetFromCSS_GeneratedClass(t *testing.T) {
	target := ir.TargetFromCSS("div.MuiButton-root.css-abc123", "test button")
	assert.True(t, target.Primary.Fragile)
	assert.Equal(t, ir.LocatorCSS, target.Primary.Strategy)
}

func TestTargetFromCSS_StableID(t *testing.T) {
	target := ir.TargetFromCSS("#submit-button", "submit")
	assert.False(t, target.Primary.Fragile)
}

func TestIsGeneratedSelector(t *testing.T) {
	cases := []struct {
		selector string
		expected bool
	}{
		{"div.css-abc123", true},
		{".MuiButton-root", false},
		{"#login-button", false},
		{"button[type=submit]", false},
		{".sc-bXCLTE", true},
	}
	for _, tc := range cases {
		t.Run(tc.selector, func(t *testing.T) {
			got := ir.IsGeneratedSelector(tc.selector)
			assert.Equal(t, tc.expected, got, "selector: %s", tc.selector)
		})
	}
}

func TestLocatorConfidence(t *testing.T) {
	assert.Equal(t, 1.0, ir.LocatorConfidence[ir.LocatorTestID])
	assert.Equal(t, 0.9, ir.LocatorConfidence[ir.LocatorRole])
	assert.Greater(t, ir.LocatorConfidence[ir.LocatorRole], ir.LocatorConfidence[ir.LocatorCSS])
	assert.Greater(t, ir.LocatorConfidence[ir.LocatorCSS], ir.LocatorConfidence[ir.LocatorCoords])
}
