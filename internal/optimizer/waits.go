package optimizer

import (
	"regexp"
	"strings"

	ir "github.com/yagna-1/recast/recast-ir"
)

func runWaitInference(trace *ir.Trace) (*ir.Trace, []ir.Warning) {
	var warnings []ir.Warning
	steps := make([]ir.Step, len(trace.Steps))
	copy(steps, trace.Steps)

	for i := range steps {
		step := &steps[i]

		if step.Wait.Type != "" {
			continue
		}

		var next *ir.Step
		if i+1 < len(steps) {
			next = &steps[i+1]
		}

		wait := inferWait(step, next)
		if wait.Type != "" {
			step.Wait = wait
		}
	}

	out := *trace
	out.Steps = steps
	return &out, warnings
}

func inferWait(step *ir.Step, next *ir.Step) ir.WaitSpec {
	switch step.Type {
	case ir.StepNavigate:
		return ir.WaitSpec{
			Type:    ir.WaitNetworkIdle,
			Timeout: 30000,
		}

	case ir.StepClick:
		if step.Target == nil {
			return ir.WaitSpec{}
		}
		sel := step.Target.Primary.Value

		if isSubmitButton(sel) || isSubmitButtonByRole(step.Target.Primary) {
			return ir.WaitSpec{
				Type:    ir.WaitNavigation,
				Timeout: 10000,
			}
		}

		if isModalCloseButton(sel, step.Target) {
			return ir.WaitSpec{
				Type:    ir.WaitSelectorHidden,
				Value:   "[role=dialog],[role=alertdialog],.modal,.dialog",
				Timeout: 5000,
			}
		}

		if next != nil && next.Type == ir.StepNavigate {
			return ir.WaitSpec{
				Type:    ir.WaitNavigation,
				Timeout: 10000,
			}
		}

		if isDropdownTrigger(sel, step.Target) {
			return ir.WaitSpec{
				Type:    ir.WaitSelector,
				Value:   "[role=listbox],[role=option],[role=menu]",
				Timeout: 5000,
			}
		}

		if next != nil && next.Target != nil {
			if nextSel, ok := waitSelectorForTarget(next.Target); ok {
				return ir.WaitSpec{
					Type:    ir.WaitSelector,
					Value:   nextSel,
					Timeout: 5000,
				}
			}
		}

	case ir.StepFill:
		if next != nil && next.Type == ir.StepFill {
			return ir.WaitSpec{Type: ir.WaitNone}
		}

	case ir.StepSelect:
		if next != nil && next.Type != ir.StepFill {
			return ir.WaitSpec{
				Type:    ir.WaitDelay,
				Value:   "500",
				Timeout: 500,
			}
		}
	}

	return ir.WaitSpec{}
}

func waitSelectorForTarget(target *ir.Target) (string, bool) {
	if target == nil {
		return "", false
	}
	loc := target.Primary
	switch loc.Strategy {
	case ir.LocatorCSS, ir.LocatorXPathRel, ir.LocatorXPathAbs, ir.LocatorTestID:
		if strings.TrimSpace(loc.Value) == "" {
			return "", false
		}
		return loc.Value, true
	default:
		return "", false
	}
}

func isSubmitButton(sel string) bool {
	return reSubmitSelector.MatchString(sel)
}

func isSubmitButtonByRole(loc ir.Locator) bool {
	if loc.Strategy != ir.LocatorRole {
		return false
	}
	lower := strings.ToLower(loc.Value)
	for _, name := range submitNames {
		if strings.Contains(lower, name) {
			return true
		}
	}
	return false
}

func isModalCloseButton(sel string, target *ir.Target) bool {
	if reModalClose.MatchString(sel) {
		return true
	}
	if target != nil && target.HumanDescription != "" {
		desc := strings.ToLower(target.HumanDescription)
		for _, kw := range modalCloseKeywords {
			if strings.Contains(desc, kw) {
				return true
			}
		}
	}
	return false
}

func isDropdownTrigger(sel string, target *ir.Target) bool {
	if reDropdown.MatchString(sel) {
		return true
	}
	if target != nil {
		desc := strings.ToLower(target.HumanDescription)
		for _, kw := range dropdownKeywords {
			if strings.Contains(desc, kw) {
				return true
			}
		}
	}
	return false
}

var (
	reSubmitSelector = regexp.MustCompile(`(?i)(button\[type=.?submit.?\]|input\[type=.?submit.?\]|\.submit|#submit|submit-btn|submit_btn)`)
	reModalClose     = regexp.MustCompile(`(?i)(close|dismiss|modal.*close|dialog.*close|\[aria-label=.?(close|dismiss).?\])`)
	reDropdown       = regexp.MustCompile(`(?i)(select|dropdown|combobox|\[role=.?combobox.?\]|\[role=.?listbox.?\])`)

	submitNames = []string{"sign in", "login", "log in", "submit", "save", "continue",
		"next", "confirm", "proceed", "checkout", "place order", "buy"}

	modalCloseKeywords = []string{"close", "dismiss", "accept", "decline", "cancel"}
	dropdownKeywords   = []string{"dropdown", "select", "choose", "option", "menu"}
)
