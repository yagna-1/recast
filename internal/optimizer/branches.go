package optimizer

import (
	"fmt"
	"strings"

	ir "github.com/yagna-1/recast/recast-ir"
)

func runBranchDetection(trace *ir.Trace) (*ir.Trace, []ir.Warning) {
	var warnings []ir.Warning
	steps := make([]ir.Step, 0, len(trace.Steps))

	i := 0
	for i < len(trace.Steps) {
		step := trace.Steps[i]

		if step.Type == ir.StepClick && step.Target != nil && isInterruptHandler(step.Target) {
			branch := ir.BranchStep{
				Condition:  inferBranchCondition(step.Target),
				ThenSteps:  []ir.Step{step},
				Incomplete: true,
			}

			j := i + 1
			for j < len(trace.Steps) {
				next := trace.Steps[j]
				if next.Type == ir.StepClick && next.Target != nil && isInterruptHandler(next.Target) {
					branch.ThenSteps = append(branch.ThenSteps, next)
					j++
				} else {
					break
				}
			}

			branchStep := ir.Step{
				ID:      fmt.Sprintf("%s_branch", step.ID),
				Type:    ir.StepBranch,
				Branch:  &branch,
				Comment: fmt.Sprintf("// TODO: implement else-branch for: %s", branch.Condition),
			}

			steps = append(steps, branchStep)
			warnings = append(warnings, ir.Warning{
				StepID:  step.ID,
				Pass:    "branches",
				Message: fmt.Sprintf("detected conditional: %q — only one branch was recorded; implement else-branch manually", branch.Condition),
			})

			i = j
			continue
		}

		steps = append(steps, step)
		i++
	}

	out := *trace
	out.Steps = steps
	return &out, warnings
}

func isInterruptHandler(target *ir.Target) bool {
	if target.HumanDescription != "" {
		desc := strings.ToLower(target.HumanDescription)
		for _, kw := range interruptKeywords {
			if strings.Contains(desc, kw) {
				return true
			}
		}
	}

	val := strings.ToLower(target.Primary.Value)
	for _, kw := range interruptSelectorKeywords {
		if strings.Contains(val, kw) {
			return true
		}
	}

	return false
}

func inferBranchCondition(target *ir.Target) string {
	desc := target.HumanDescription
	if desc == "" {
		desc = string(target.Primary.Strategy) + " " + target.Primary.Value
	}

	lower := strings.ToLower(desc)
	for _, kw := range conditionKeywordMap {
		if strings.Contains(lower, kw.keyword) {
			return kw.condition
		}
	}
	return fmt.Sprintf("%q element visible", desc)
}

var interruptKeywords = []string{
	"close", "dismiss", "accept", "decline", "cookie", "consent",
	"captcha", "verify", "popup", "modal", "dialog", "banner",
	"notification", "alert",
}

var interruptSelectorKeywords = []string{
	"cookie", "consent", "captcha", "popup", "modal", "dialog",
	"close-btn", "dismiss", "accept-all", "decline",
	`aria-label="close"`, `aria-label="dismiss"`,
}

type conditionKeyword struct {
	keyword   string
	condition string
}

var conditionKeywordMap = []conditionKeyword{
	{"cookie", "cookie consent dialog visible"},
	{"captcha", "CAPTCHA challenge visible"},
	{"consent", "consent dialog visible"},
	{"popup", "popup/overlay visible"},
	{"modal", "modal dialog visible"},
	{"banner", "notification banner visible"},
	{"alert", "alert dialog visible"},
	{"close", "overlay/dialog visible"},
}
