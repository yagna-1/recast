package optimizer_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yagna-1/recast/internal/optimizer"
	ir "github.com/yagna-1/recast/recast-ir"
)

func makeTrace(steps ...ir.Step) *ir.Trace {
	for i := range steps {
		if steps[i].ID == "" {
			steps[i].ID = fmt.Sprintf("step_%03d", i+1)
		}
	}
	return &ir.Trace{
		Name:          "test",
		SchemaVersion: ir.SchemaVersion,
		Steps:         steps,
	}
}

func clickStep(id, selector string) ir.Step {
	return ir.Step{
		ID:     id,
		Type:   ir.StepClick,
		Target: ir.TargetFromCSS(selector, ""),
	}
}

func fillStep(id, selector, value string) ir.Step {
	return ir.Step{
		ID:     id,
		Type:   ir.StepFill,
		Target: ir.TargetFromCSS(selector, ""),
		Value:  value,
	}
}

func navStep(id, url string) ir.Step {
	return ir.Step{ID: id, Type: ir.StepNavigate, Value: url}
}

func TestDedup_RemovesDuplicateClick(t *testing.T) {
	trace := makeTrace(
		clickStep("step_001", "#btn"),
		clickStep("step_002", "#btn"),
		navStep("step_003", "https://example.com"),
	)

	result := optimizer.Run(trace, optimizer.Options{Dedup: true})
	assert.Len(t, result.Trace.Steps, 2)
	assert.Equal(t, 1, result.StepsRemoved)
}

func TestDedup_RemovesConsecutiveNavigates(t *testing.T) {
	trace := makeTrace(
		navStep("step_001", "https://a.com"),
		navStep("step_002", "https://b.com"),
	)
	result := optimizer.Run(trace, optimizer.Options{Dedup: true})
	assert.Len(t, result.Trace.Steps, 1)
	assert.Equal(t, "https://b.com", result.Trace.Steps[0].Value)
}

func TestDedup_Idempotent(t *testing.T) {
	trace := makeTrace(
		clickStep("step_001", "#btn"),
		navStep("step_002", "https://example.com"),
	)
	opts := optimizer.Options{Dedup: true}
	result1 := optimizer.Run(trace, opts)
	result2 := optimizer.Run(result1.Trace, opts)
	assert.Len(t, result1.Trace.Steps, len(result2.Trace.Steps))
}

func TestDedup_PreservesDifferentSteps(t *testing.T) {
	trace := makeTrace(
		clickStep("step_001", "#btn1"),
		clickStep("step_002", "#btn2"),
		navStep("step_003", "https://example.com"),
	)
	result := optimizer.Run(trace, optimizer.Options{Dedup: true})
	assert.Len(t, result.Trace.Steps, 3)
	assert.Equal(t, 0, result.StepsRemoved)
}

func TestSanitize_Email(t *testing.T) {
	trace := makeTrace(
		fillStep("step_001", "#email", "user@example.com"),
	)
	result := optimizer.Run(trace, optimizer.Options{})
	require.Len(t, result.Trace.Steps, 1)
	assert.Contains(t, result.Trace.Steps[0].Value, "process.env.")
	assert.Equal(t, 1, result.CredentialsSanitized)
	assert.NotEmpty(t, result.EnvVars)
}

func TestSanitize_Password(t *testing.T) {
	trace := makeTrace(
		fillStep("step_001", "#password", "my-secret-password"),
	)
	result := optimizer.Run(trace, optimizer.Options{})
	assert.Contains(t, result.Trace.Steps[0].Value, "process.env.")
	assert.Equal(t, 1, result.CredentialsSanitized)
}

func TestSanitize_LowEntropyNoMatch_NotSanitized(t *testing.T) {
	trace := makeTrace(
		fillStep("step_001", "#password", "hunter2"),
	)
	result := optimizer.Run(trace, optimizer.Options{})
	assert.Equal(t, "hunter2", result.Trace.Steps[0].Value, "hunter2 should not be sanitized")
	assert.Equal(t, 0, result.CredentialsSanitized)
}

func TestSanitize_HighEntropy(t *testing.T) {
	trace := makeTrace(
		fillStep("step_001", "#token", "xK9mN2pQrT8vWyZ4"),
	)
	result := optimizer.Run(trace, optimizer.Options{})
	assert.Contains(t, result.Trace.Steps[0].Value, "process.env.")
	assert.Equal(t, 1, result.CredentialsSanitized)
}

func TestSanitize_AlwaysRuns(t *testing.T) {
	trace := makeTrace(
		fillStep("step_001", "#email", "user@example.com"),
	)
	result := optimizer.Run(trace, optimizer.Options{
		Dedup:           false,
		HardenSelectors: false,
		InferWaits:      false,
	})
	assert.Contains(t, result.Trace.Steps[0].Value, "process.env.")
}

func TestSanitize_DoesNotSanitizeEnvRef(t *testing.T) {
	trace := makeTrace(
		fillStep("step_001", "#email", "process.env.TEST_EMAIL"),
	)
	result := optimizer.Run(trace, optimizer.Options{})
	assert.Equal(t, "process.env.TEST_EMAIL", result.Trace.Steps[0].Value)
}

func TestSanitize_DuplicateValueReuse(t *testing.T) {
	trace := makeTrace(
		fillStep("step_001", "#email1", "user@example.com"),
		fillStep("step_002", "#email2", "user@example.com"),
	)
	result := optimizer.Run(trace, optimizer.Options{})
	assert.Equal(t, result.Trace.Steps[0].Value, result.Trace.Steps[1].Value)
	assert.Equal(t, 2, result.CredentialsSanitized)
}

func TestWaitInference_NavigateGetsNetworkIdle(t *testing.T) {
	trace := makeTrace(
		navStep("step_001", "https://example.com"),
	)
	result := optimizer.Run(trace, optimizer.Options{InferWaits: true})
	assert.Equal(t, ir.WaitNetworkIdle, result.Trace.Steps[0].Wait.Type)
}

func TestWaitInference_SubmitButtonGetsNavigation(t *testing.T) {
	trace := makeTrace(
		clickStep("step_001", "button[type=submit]"),
	)
	result := optimizer.Run(trace, optimizer.Options{InferWaits: true})
	assert.Equal(t, ir.WaitNavigation, result.Trace.Steps[0].Wait.Type)
}

func TestWaitInference_ConsecutiveFillsNoWait(t *testing.T) {
	trace := makeTrace(
		fillStep("step_001", "#email", "test"),
		fillStep("step_002", "#password", "test"),
	)
	result := optimizer.Run(trace, optimizer.Options{InferWaits: true})
	assert.Equal(t, ir.WaitNone, result.Trace.Steps[0].Wait.Type)
}

func TestWaitInference_Idempotent(t *testing.T) {
	trace := makeTrace(
		navStep("step_001", "https://example.com"),
		clickStep("step_002", "button[type=submit]"),
	)
	opts := optimizer.Options{InferWaits: true}
	result1 := optimizer.Run(trace, opts)
	result2 := optimizer.Run(result1.Trace, opts)
	assert.Equal(t, result1.Trace.Steps[0].Wait.Type, result2.Trace.Steps[0].Wait.Type)
}

func TestPipeline_DefaultOptions(t *testing.T) {
	trace := makeTrace(
		navStep("step_001", "https://example.com/login"),
		fillStep("step_002", "#email", "user@example.com"),
		fillStep("step_003", "#password", "secret123"),
		clickStep("step_004", "button[type=submit]"),
		clickStep("step_005", "button[type=submit]"), // duplicate
	)

	result := optimizer.Run(trace, optimizer.DefaultOptions())
	require.NotNil(t, result)

	assert.Less(t, len(result.Trace.Steps), 5)

	assert.Equal(t, 2, result.CredentialsSanitized)
	assert.Contains(t, result.EnvVars, "TEST_EMAIL")

	assert.Equal(t, ir.WaitNetworkIdle, result.Trace.Steps[0].Wait.Type)
}

func TestSelectorHardening_Idempotent(t *testing.T) {
	trace := makeTrace(
		ir.Step{
			ID:   "step_001",
			Type: ir.StepClick,
			Target: &ir.Target{
				Primary: ir.Locator{
					Strategy:   ir.LocatorCSS,
					Value:      `[aria-label="Email"]`,
					Confidence: 0.5,
				},
			},
		},
	)
	opts := optimizer.Options{HardenSelectors: true}
	result1 := optimizer.Run(trace, opts)
	result2 := optimizer.Run(result1.Trace, opts)

	assert.Equal(t, result1.Trace.Steps[0].Target.Primary, result2.Trace.Steps[0].Target.Primary)
	assert.Equal(t, len(result1.Trace.Steps[0].Target.Fallbacks), len(result2.Trace.Steps[0].Target.Fallbacks))
}

func TestBranchDetection_Idempotent(t *testing.T) {
	trace := makeTrace(
		ir.Step{
			ID:   "step_001",
			Type: ir.StepClick,
			Target: &ir.Target{
				Primary:          ir.Locator{Strategy: ir.LocatorCSS, Value: ".cookie-consent-close", Confidence: 0.5},
				HumanDescription: "close cookie dialog",
			},
		},
		ir.Step{
			ID:   "step_002",
			Type: ir.StepClick,
			Target: &ir.Target{
				Primary:          ir.Locator{Strategy: ir.LocatorCSS, Value: ".cookie-consent-accept", Confidence: 0.5},
				HumanDescription: "accept cookie dialog",
			},
		},
		ir.Step{
			ID:    "step_003",
			Type:  ir.StepNavigate,
			Value: "https://example.com/dashboard",
		},
	)
	opts := optimizer.Options{DetectBranches: true}
	result1 := optimizer.Run(trace, opts)
	result2 := optimizer.Run(result1.Trace, opts)

	assert.Equal(t, len(result1.Trace.Steps), len(result2.Trace.Steps))
	assert.Equal(t, result1.Trace.Steps[0].Type, result2.Trace.Steps[0].Type)
}

func TestAssertionInjection_Idempotent(t *testing.T) {
	trace := makeTrace(
		navStep("step_001", "https://example.com/login"),
		fillStep("step_002", "#email", "process.env.TEST_EMAIL"),
		fillStep("step_003", "#password", "process.env.TEST_PASSWORD"),
		ir.Step{
			ID:   "step_004",
			Type: ir.StepClick,
			Target: &ir.Target{
				Primary: ir.Locator{
					Strategy: ir.LocatorRole,
					Value:    `role=button[name="Sign in"]`,
				},
				HumanDescription: "the Sign in button",
			},
		},
	)
	opts := optimizer.Options{InjectAssertions: true}
	result1 := optimizer.Run(trace, opts)
	result2 := optimizer.Run(result1.Trace, opts)

	assert.Equal(t, len(result1.Trace.Steps), len(result2.Trace.Steps))
}

func TestPipeline_LargeWorkflowPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skip perf stress test in short mode")
	}

	steps := make([]ir.Step, 0, 10000)
	for i := 0; i < 10000; i++ {
		id := fmt.Sprintf("step_%05d", i+1)
		switch i % 3 {
		case 0:
			steps = append(steps, ir.Step{
				ID:    id,
				Type:  ir.StepNavigate,
				Value: "https://example.com",
			})
		case 1:
			steps = append(steps, ir.Step{
				ID:   id,
				Type: ir.StepFill,
				Target: &ir.Target{
					Primary: ir.Locator{Strategy: ir.LocatorCSS, Value: "#email", Confidence: 0.5},
				},
				Value: "user@example.com",
			})
		default:
			steps = append(steps, ir.Step{
				ID:   id,
				Type: ir.StepClick,
				Target: &ir.Target{
					Primary: ir.Locator{Strategy: ir.LocatorCSS, Value: "button[type=submit]", Confidence: 0.5},
				},
			})
		}
	}

	trace := &ir.Trace{Name: "perf_10k", SchemaVersion: ir.SchemaVersion, Steps: steps}
	start := time.Now()
	result := optimizer.Run(trace, optimizer.DefaultOptions())
	elapsed := time.Since(start)

	require.NotNil(t, result)
	require.NotNil(t, result.Trace)
	assert.NotEmpty(t, result.Trace.Steps)
	assert.Less(t, elapsed, 8*time.Second, "optimizer should handle 10k-step workflows without pathological slowdown")
}
