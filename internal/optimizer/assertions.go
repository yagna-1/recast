package optimizer

import (
	"fmt"
	"strings"

	ir "github.com/yourorg/recast/recast-ir"
)

func runAssertionInjection(trace *ir.Trace) (*ir.Trace, []ir.Warning) {
	var warnings []ir.Warning
	result := make([]ir.Step, 0, len(trace.Steps)*2)
	assertIdx := 0

	isLoginSequence := detectLoginSequence(trace.Steps)

	for i, step := range trace.Steps {
		result = append(result, step)

		var assertion *ir.Step

		switch step.Type {
		case ir.StepNavigate:
			assertIdx++
			assertion = &ir.Step{
				ID:   fmt.Sprintf("assert_%03d", assertIdx),
				Type: ir.StepAssert,
				Target: &ir.Target{
					Primary: ir.Locator{
						Strategy:   ir.LocatorCSS,
						Value:      step.Value,
						Confidence: 0.9,
					},
					HumanDescription: "current URL",
				},
				Comment: fmt.Sprintf("// Assert: URL is %s", step.Value),
			}

		case ir.StepClick:
			if step.Target == nil {
				break
			}
			if isLoginSequence && i > 0 && isSubmitButtonByRole(step.Target.Primary) {
				assertIdx++
				assertion = &ir.Step{
					ID:   fmt.Sprintf("assert_%03d", assertIdx),
					Type: ir.StepAssert,
					Target: &ir.Target{
						Primary: ir.Locator{
							Strategy:   ir.LocatorRole,
							Value:      `role=navigation`,
							Confidence: 0.9,
						},
						HumanDescription: "main navigation (login success indicator)",
					},
					Comment: "// Assert: navigation is visible (login succeeded)",
				}
			}
			if isModalCloseButton(step.Target.Primary.Value, step.Target) {
				assertIdx++
				assertion = &ir.Step{
					ID:   fmt.Sprintf("assert_%03d", assertIdx),
					Type: ir.StepAssert,
					Target: &ir.Target{
						Primary: ir.Locator{
							Strategy:   ir.LocatorCSS,
							Value:      "[role=dialog],[role=alertdialog]",
							Confidence: 0.8,
						},
						HumanDescription: "modal dialog (should be hidden)",
					},
					Comment: "// Assert: modal is no longer visible",
				}
			}
		}

		if assertion != nil {
			result = append(result, *assertion)
			warnings = append(warnings, ir.Warning{
				StepID:  step.ID,
				Pass:    "assertions",
				Message: fmt.Sprintf("injected assertion after %s", step.Type),
			})
		}
	}

	out := *trace
	out.Steps = result
	return &out, warnings
}

func detectLoginSequence(steps []ir.Step) bool {
	hasEmailFill := false
	hasPasswordFill := false

	for _, step := range steps {
		if step.Type == ir.StepFill {
			val := strings.ToLower(step.Value)
			if strings.Contains(val, "process.env.test_email") || strings.Contains(val, "os.environ") {
				hasEmailFill = true
			}
			if strings.Contains(val, "process.env.test_password") {
				hasPasswordFill = true
			}
		}
	}
	return hasEmailFill && hasPasswordFill
}
