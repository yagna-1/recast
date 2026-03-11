package ingestion_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yagna-1/recast/internal/ingestion"
	ir "github.com/yagna-1/recast/recast-ir"
)

var loginWorkflowJSON = []byte(`{
  "workflow_name": "login_workflow",
  "base_url": "https://app.example.com",
  "steps": [
    {"type": "navigate", "url": "https://app.example.com/login"},
    {"type": "fill", "selector": "#email", "value": "user@example.com"},
    {"type": "fill", "selector": "#password", "value": "secret"},
    {"type": "click", "selector": "button[type=submit]"},
    {"type": "wait_for", "selector": ".dashboard"}
  ]
}`)

func TestWorkflowUse_Parse_ValidLogin(t *testing.T) {
	ing := &ingestion.WorkflowUseIngester{}
	require.True(t, ing.CanHandle("workflow.json", loginWorkflowJSON))

	trace, err := ing.Parse(loginWorkflowJSON)
	require.NoError(t, err)
	require.NotNil(t, trace)

	assert.Equal(t, "login_workflow", trace.Name)
	assert.Equal(t, "https://app.example.com", trace.BaseURL)
	assert.Equal(t, "workflow-use", trace.Metadata.SourceFormat)
	assert.Len(t, trace.Steps, 5)

	assert.Equal(t, ir.StepNavigate, trace.Steps[0].Type)
	assert.Equal(t, "https://app.example.com/login", trace.Steps[0].Value)
	assert.Equal(t, ir.WaitNetworkIdle, trace.Steps[0].Wait.Type)

	assert.Equal(t, ir.StepFill, trace.Steps[1].Type)
	assert.Equal(t, "user@example.com", trace.Steps[1].Value)
	assert.NotNil(t, trace.Steps[1].Target)
	assert.Equal(t, "#email", trace.Steps[1].Target.Primary.Value)

	assert.Equal(t, ir.StepClick, trace.Steps[3].Type)
	assert.NotNil(t, trace.Steps[3].Target)
}

func TestWorkflowUse_Parse_EmptySteps(t *testing.T) {
	data := []byte(`{"workflow_name": "empty", "steps": []}`)
	ing := &ingestion.WorkflowUseIngester{}
	_, err := ing.Parse(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no steps")
}

func TestWorkflowUse_Parse_MalformedJSON(t *testing.T) {
	data := []byte(`{not valid json`)
	ing := &ingestion.WorkflowUseIngester{}
	_, err := ing.Parse(data)
	assert.Error(t, err)
}

func TestWorkflowUse_UnsupportedStepType(t *testing.T) {
	data := []byte(`{
		"workflow_name": "test",
		"steps": [
			{"type": "navigate", "url": "https://example.com"},
			{"type": "frobnicate", "selector": "#foo"}
		]
	}`)
	ing := &ingestion.WorkflowUseIngester{}
	trace, err := ing.Parse(data)
	require.NoError(t, err)
	assert.Len(t, trace.Steps, 2)
	assert.Contains(t, trace.Steps[1].Comment, "TODO")
}

func TestWorkflowUse_CoordinateOnlyClick(t *testing.T) {
	data := []byte(`{
		"workflow_name": "test",
		"steps": [
			{"type": "click", "x": 100, "y": 200}
		]
	}`)
	ing := &ingestion.WorkflowUseIngester{}
	trace, err := ing.Parse(data)
	require.NoError(t, err)
	require.Len(t, trace.Steps, 1)
	step := trace.Steps[0]
	assert.Equal(t, ir.StepClick, step.Type)
	assert.NotNil(t, step.Target)
}

func TestWorkflowUse_Parse_SemanticWorkflow(t *testing.T) {
	data := []byte(`{
		"name": "semantic_flow",
		"base_url": "https://example.com",
		"steps": [
			{"type": "navigation", "url": "https://example.com/login"},
			{"type": "click", "target_text": "Sign in"},
			{"type": "input", "target_text": "Email", "value": "user@example.com"},
			{"type": "scroll", "scrollY": 250},
			{"type": "click", "cssSelector": "button[type=submit]"}
		]
	}`)
	ing := &ingestion.WorkflowUseIngester{}
	trace, err := ing.Parse(data)
	require.NoError(t, err)
	require.Len(t, trace.Steps, 5)

	assert.Equal(t, "semantic_flow", trace.Name)
	assert.Equal(t, ir.StepNavigate, trace.Steps[0].Type)
	assert.Equal(t, ir.StepClick, trace.Steps[1].Type)
	assert.Equal(t, ir.LocatorText, trace.Steps[1].Target.Primary.Strategy)
	assert.Equal(t, "Sign in", trace.Steps[1].Target.Primary.Value)
	assert.Equal(t, ir.StepFill, trace.Steps[2].Type)
	assert.Equal(t, ir.LocatorCSS, trace.Steps[2].Target.Primary.Strategy)
	assert.Equal(t, "#Email", trace.Steps[2].Target.Primary.Value)
	assert.Equal(t, ir.StepScroll, trace.Steps[3].Type)
	assert.Equal(t, "250", trace.Steps[3].Value)
	assert.Equal(t, "button[type=submit]", trace.Steps[4].Target.Primary.Value)
}

func TestDetect_WorkflowUseJSON(t *testing.T) {
	ing, err := ingestion.Detect("workflow.json", loginWorkflowJSON)
	require.NoError(t, err)
	assert.Equal(t, "workflow-use JSON", ing.FormatName())
}

func TestDetect_UnknownFormat(t *testing.T) {
	data := []byte(`{"completely": "unknown", "format": true}`)
	_, err := ingestion.Detect("unknown.xyz", data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no adapter found")
}

func TestHAR_Parse_Valid(t *testing.T) {
	data := []byte(`{
		"log": {
			"version": "1.2",
			"browser": {"name": "Chrome", "version": "120"},
			"pages": [{"id": "page_1", "title": "My App"}],
			"entries": [
				{
					"request": {"method": "GET", "url": "https://example.com/login", "headers": []},
					"response": {"status": 200}
				},
				{
					"request": {"method": "GET", "url": "https://example.com/dashboard", "headers": []},
					"response": {"status": 200}
				}
			]
		}
	}`)
	ing := &ingestion.HARIngester{}
	require.True(t, ing.CanHandle("trace.har", data))
	trace, err := ing.Parse(data)
	require.NoError(t, err)
	assert.Equal(t, "my_app", trace.Name)
	assert.Len(t, trace.Steps, 2)
	assert.Equal(t, ir.StepNavigate, trace.Steps[0].Type)
}

func TestMCP_Parse_Valid(t *testing.T) {
	data := []byte(`[
		{"type": "tool_call", "tool": "browser_navigate", "params": {"url": "https://example.com"}},
		{"type": "tool_call", "tool": "browser_click", "params": {"element": "Login", "selector": "#login"}},
		{"type": "tool_result", "tool": "browser_click", "result": {"success": true}}
	]`)
	ing := &ingestion.MCPIngester{}
	require.True(t, ing.CanHandle("log.jsonl", data))
	trace, err := ing.Parse(data)
	require.NoError(t, err)
	assert.Len(t, trace.Steps, 2) // tool_result should be skipped
	assert.Equal(t, ir.StepNavigate, trace.Steps[0].Type)
	assert.Equal(t, ir.StepClick, trace.Steps[1].Type)
}

func TestAllFormats(t *testing.T) {
	formats := ingestion.AllFormats()
	assert.Greater(t, len(formats), 0)
	names := make([]string, len(formats))
	for i, f := range formats {
		names[i] = f.Name
	}
	assert.Contains(t, names, "workflow-use JSON")
	assert.Contains(t, names, "HAR (HTTP Archive)")
	assert.Contains(t, names, "CDP Event Log")
	assert.Contains(t, names, "MCP Tool Call Log")
}
