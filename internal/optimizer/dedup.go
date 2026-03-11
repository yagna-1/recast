package optimizer

import (
	"fmt"

	ir "github.com/yagna-1/recast/recast-ir"
)

func runDedup(trace *ir.Trace) (*ir.Trace, []ir.Warning, int) {
	if len(trace.Steps) <= 1 {
		return trace, nil, 0
	}

	var warnings []ir.Warning
	removed := 0
	result := make([]ir.Step, 0, len(trace.Steps))

	for i, step := range trace.Steps {
		if i == 0 {
			result = append(result, step)
			continue
		}

		prev := result[len(result)-1]
		if isDuplicate(step, prev) {
			warnings = append(warnings, ir.Warning{
				StepID:  step.ID,
				Pass:    "dedup",
				Message: fmt.Sprintf("removed duplicate %s (same as %s)", step.Type, prev.ID),
			})
			removed++
			continue
		}

		if step.Type == ir.StepNavigate && prev.Type == ir.StepNavigate {
			warnings = append(warnings, ir.Warning{
				StepID:  prev.ID,
				Pass:    "dedup",
				Message: fmt.Sprintf("removed navigate to %q (immediately followed by navigate to %q)", prev.Value, step.Value),
			})
			result[len(result)-1] = step
			removed++
			continue
		}

		result = append(result, step)
	}

	out := *trace
	out.Steps = result
	return &out, warnings, removed
}

func isDuplicate(a, b ir.Step) bool {
	if a.Type != b.Type {
		return false
	}

	switch a.Type {
	case ir.StepNavigate:
		return a.Value == b.Value

	case ir.StepClick, ir.StepHover, ir.StepFocus, ir.StepBlur,
		ir.StepCheck, ir.StepUncheck:
		return targetsMatch(a.Target, b.Target)

	case ir.StepFill:
		return targetsMatch(a.Target, b.Target) && a.Value == b.Value

	case ir.StepKeyboard:
		return a.Value == b.Value
	}

	return false
}

func targetsMatch(a, b *ir.Target) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Primary.Value == b.Primary.Value &&
		a.Primary.Strategy == b.Primary.Strategy
}
