package emitter_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yagna-1/recast/internal/emitter"
	ir "github.com/yagna-1/recast/recast-ir"
)

func buildLoginTrace() *ir.Trace {
	return ir.NewTrace("login_test").
		WithBaseURL("https://example.com").
		Navigate("https://example.com/login").
		Fill(ir.TargetFromLabel("Email", "the email field"), "process.env.TEST_EMAIL").
		Fill(ir.TargetFromCSS("#password", "the password field"), "process.env.TEST_PASSWORD").
		Click(ir.TargetFromRole("button", "Sign in", "the Sign in button")).
		BuildUnchecked()
}

func TestPlaywrightTS_Emit_ValidTrace(t *testing.T) {
	e := &emitter.PlaywrightTSEmitter{}
	trace := buildLoginTrace()

	result, err := e.Emit(trace, nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	code := result.TestFile
	assert.Contains(t, code, "import { test, expect } from '@playwright/test'")
	assert.Contains(t, code, "test('login_test'")
	assert.Contains(t, code, "page.goto('https://example.com/login')")
	assert.Contains(t, code, "page.getByLabel('Email')")
	assert.Contains(t, code, "process.env.TEST_EMAIL!")
	assert.Contains(t, code, "process.env.TEST_PASSWORD!")
	assert.Contains(t, code, "page.getByRole('button', { name: 'Sign in' })")
}

func TestPlaywrightTS_AllStepTypes(t *testing.T) {
	e := &emitter.PlaywrightTSEmitter{}

	steps := []ir.Step{
		{ID: "s1", Type: ir.StepNavigate, Value: "https://example.com",
			Wait: ir.WaitSpec{Type: ir.WaitNetworkIdle}},
		{ID: "s2", Type: ir.StepClick, Target: ir.TargetFromCSS("#btn", "")},
		{ID: "s3", Type: ir.StepFill, Target: ir.TargetFromCSS("#input", ""), Value: "hello"},
		{ID: "s4", Type: ir.StepSelect, Target: ir.TargetFromCSS("#sel", ""), Value: "option1"},
		{ID: "s5", Type: ir.StepHover, Target: ir.TargetFromCSS("#hover", "")},
		{ID: "s6", Type: ir.StepCheck, Target: ir.TargetFromCSS("#chk", "")},
		{ID: "s7", Type: ir.StepUncheck, Target: ir.TargetFromCSS("#chk", "")},
		{ID: "s8", Type: ir.StepKeyboard, Value: "Enter"},
		{ID: "s9", Type: ir.StepScroll, Value: "down"},
		{ID: "s10", Type: ir.StepScreenshot, Value: "test.png"},
		{ID: "s11", Type: ir.StepWaitForURL, Value: "https://example.com/done"},
		{ID: "s12", Type: ir.StepDialog, Value: "accept"},
		{ID: "s13", Type: ir.StepFileUpload, Target: ir.TargetFromCSS("#upload", "")},
	}

	trace := &ir.Trace{Name: "all_steps", SchemaVersion: ir.SchemaVersion, Steps: steps}
	result, err := e.Emit(trace, nil)
	require.NoError(t, err)

	code := result.TestFile
	assert.Contains(t, code, "page.goto")
	assert.Contains(t, code, ".click()")
	assert.Contains(t, code, ".fill(")
	assert.Contains(t, code, ".selectOption(")
	assert.Contains(t, code, ".hover()")
	assert.Contains(t, code, ".check()")
	assert.Contains(t, code, ".uncheck()")
	assert.Contains(t, code, "keyboard.press")
	assert.Contains(t, code, "scrollBy")
	assert.Contains(t, code, "screenshot")
	assert.Contains(t, code, "waitForURL")
	assert.Contains(t, code, "dialog")
	assert.Contains(t, code, "setInputFiles")
}

func TestPlaywrightTS_UnsupportedStepType(t *testing.T) {
	e := &emitter.PlaywrightTSEmitter{}
	trace := &ir.Trace{
		Name:          "test",
		SchemaVersion: ir.SchemaVersion,
		Steps: []ir.Step{
			{ID: "s1", Type: "frobnicate"},
		},
	}
	result, err := e.Emit(trace, nil)
	require.NoError(t, err)
	assert.Contains(t, result.TestFile, "TODO: unsupported step type: frobnicate")
}

func TestPlaywrightTS_GeneratesEnvExample(t *testing.T) {
	e := &emitter.PlaywrightTSEmitter{}
	trace := buildLoginTrace()
	envVars := map[string]string{
		"TEST_EMAIL":    "# step_002: fill value sanitized (email)",
		"TEST_PASSWORD": "# step_003: fill value sanitized (password)",
	}
	result, err := e.Emit(trace, envVars)
	require.NoError(t, err)
	envFile := result.AuxFiles[".env.example"]
	assert.Contains(t, envFile, "TEST_EMAIL=")
	assert.Contains(t, envFile, "TEST_PASSWORD=")
	assert.Contains(t, envFile, "DO NOT commit")
}

func TestPlaywrightPy_Emit_ValidTrace(t *testing.T) {
	e := &emitter.PlaywrightPyEmitter{}
	trace := buildLoginTrace()

	result, err := e.Emit(trace, nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	code := result.TestFile
	assert.Contains(t, code, "import os")
	assert.Contains(t, code, "from playwright.sync_api import")
	assert.Contains(t, code, "def test_login_test(page):")
	assert.Contains(t, code, `page.goto("https://example.com/login")`)
	assert.Contains(t, code, `page.get_by_label("Email")`)
	assert.Contains(t, code, `os.environ["TEST_EMAIL"]`)
	assert.Contains(t, code, `os.environ["TEST_PASSWORD"]`)
	assert.Contains(t, code, `page.get_by_role("button", name="Sign in")`)
}

func TestPlaywrightTS_FrameContext(t *testing.T) {
	e := &emitter.PlaywrightTSEmitter{}
	frameTarget := ir.TargetFromCSS("iframe#payment", "payment iframe")
	innerTarget := ir.TargetFromLabel("Card number", "card number field")
	innerTarget.WithContext(ir.ContextIframe, frameTarget)

	trace := &ir.Trace{
		Name:          "iframe_test",
		SchemaVersion: ir.SchemaVersion,
		Steps: []ir.Step{
			{ID: "s1", Type: ir.StepFill, Target: innerTarget, Value: "4111111111111111"},
		},
	}

	result, err := e.Emit(trace, nil)
	require.NoError(t, err)
	assert.Contains(t, result.TestFile, "frameLocator")
}

func TestPlaywrightTS_BranchStep(t *testing.T) {
	e := &emitter.PlaywrightTSEmitter{}
	branch := &ir.BranchStep{
		Condition:  "cookie consent dialog visible",
		ThenSteps:  []ir.Step{{ID: "b1", Type: ir.StepClick, Target: ir.TargetFromCSS(".accept-cookies", "")}},
		Incomplete: true,
	}
	trace := &ir.Trace{
		Name:          "branch_test",
		SchemaVersion: ir.SchemaVersion,
		Steps: []ir.Step{
			{ID: "s1", Type: ir.StepBranch, Branch: branch},
		},
	}
	result, err := e.Emit(trace, nil)
	require.NoError(t, err)
	assert.Contains(t, result.TestFile, "TODO")
	assert.Contains(t, result.TestFile, "cookie consent")
	assert.Contains(t, result.TestFile, "else")
}

func TestEmitterRegistry_Get_KnownTargets(t *testing.T) {
	for _, target := range []string{"playwright-ts", "playwright-py"} {
		e, err := emitter.Get(target)
		require.NoError(t, err, "target: %s", target)
		assert.Equal(t, target, e.TargetName())
	}
}

func TestEmitterRegistry_Get_UnknownTarget(t *testing.T) {
	_, err := emitter.Get("puppeteer-ts")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown target")
}

func TestPlaywrightTS_SingleQuoteEscape(t *testing.T) {
	e := &emitter.PlaywrightTSEmitter{}
	trace := &ir.Trace{
		Name:          "escape_test",
		SchemaVersion: ir.SchemaVersion,
		Steps: []ir.Step{
			{
				ID:     "s1",
				Type:   ir.StepFill,
				Target: ir.TargetFromCSS("#input", ""),
				Value:  "it's a test",
			},
		},
	}
	result, err := e.Emit(trace, nil)
	require.NoError(t, err)
	assert.False(t, strings.Contains(result.TestFile, "fill('it's a test')"))
}

func TestPlaywrightTS_WaitNetworkIdle(t *testing.T) {
	e := &emitter.PlaywrightTSEmitter{}
	trace := &ir.Trace{
		Name:          "wait_test",
		SchemaVersion: ir.SchemaVersion,
		Steps: []ir.Step{
			{
				ID:    "s1",
				Type:  ir.StepNavigate,
				Value: "https://example.com",
				Wait:  ir.WaitSpec{Type: ir.WaitNetworkIdle, Timeout: 30000},
			},
		},
	}
	result, err := e.Emit(trace, nil)
	require.NoError(t, err)
	assert.Contains(t, result.TestFile, "waitForLoadState('networkidle')")
}
