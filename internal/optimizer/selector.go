package optimizer

import (
	"fmt"
	"regexp"
	"strings"

	ir "github.com/yourorg/recast/recast-ir"
)

func runSelectorHardening(trace *ir.Trace) (*ir.Trace, []ir.Warning, int) {
	var warnings []ir.Warning
	hardened := 0

	steps := make([]ir.Step, len(trace.Steps))
	copy(steps, trace.Steps)

	for i, step := range steps {
		if step.Target == nil {
			continue
		}

		newTarget, changed, w := hardenTarget(step.Target, step.ID)
		if changed {
			steps[i].Target = newTarget
			hardened++
		}
		warnings = append(warnings, w...)
	}

	out := *trace
	out.Steps = steps
	return &out, warnings, hardened
}

func hardenTarget(target *ir.Target, stepID string) (*ir.Target, bool, []ir.Warning) {
	var warnings []ir.Warning
	primary := target.Primary

	if isStableStrategy(primary.Strategy) {
		return target, false, nil
	}

	if primary.Strategy != ir.LocatorCSS {
		return target, false, nil
	}

	selector := primary.Value
	newTarget := *target

	upgraded, strategy, comment := inferLocatorFromCSS(selector)
	if upgraded == "" || (strategy == primary.Strategy && upgraded == selector) {
		if ir.IsGeneratedSelector(selector) && !primary.Fragile {
			newTarget.Primary.Fragile = true
			newTarget.Primary.Confidence = 0.3
			warnings = append(warnings, ir.Warning{
				StepID:  stepID,
				Pass:    "selector",
				Message: fmt.Sprintf("selector %q contains generated CSS class — consider adding data-testid", selector),
			})
		}
		return &newTarget, false, warnings
	}

	newTarget.Fallbacks = append([]ir.Locator{primary}, target.Fallbacks...)
	newTarget.Primary = ir.Locator{
		Strategy:   strategy,
		Value:      upgraded,
		Confidence: ir.LocatorConfidence[strategy],
	}
	if comment != "" {
		newTarget.Primary.Fragile = false
	}

	if target.HumanDescription == "" && comment != "" {
		newTarget.HumanDescription = comment
	}

	warnings = append(warnings, ir.Warning{
		StepID:  stepID,
		Pass:    "selector",
		Message: fmt.Sprintf("hardened %q → %s %q", selector, strategy, upgraded),
	})

	return &newTarget, true, warnings
}

func inferLocatorFromCSS(selector string) (string, ir.LocatorStrategy, string) {
	if m := reTestID.FindStringSubmatch(selector); len(m) > 1 {
		return fmt.Sprintf("[data-testid=%q]", m[1]), ir.LocatorTestID, ""
	}
	if m := reDataPW.FindStringSubmatch(selector); len(m) > 1 {
		return fmt.Sprintf("[data-pw=%q]", m[1]), ir.LocatorTestID, ""
	}

	if reSubmitBtn.MatchString(selector) {
		return `role=button[name="Submit"]`, ir.LocatorRole, "the Submit button"
	}

	if reEmailInput.MatchString(selector) {
		return `input[type="email"]`, ir.LocatorCSS, "the email input field"
	}

	if rePasswordInput.MatchString(selector) {
		return `input[type="password"]`, ir.LocatorCSS, "the password input field"
	}

	if m := reAriaLabel.FindStringSubmatch(selector); len(m) > 1 {
		label := strings.Trim(m[1], `"'`)
		return label, ir.LocatorLabel, label
	}

	if strings.HasPrefix(selector, "#") && !strings.ContainsAny(selector[1:], " .[]>#:") {
		id := selector[1:]
		if !looksGenerated(id) {
			return selector, ir.LocatorCSS, ""
		}
	}

	return "", "", ""
}

func isStableStrategy(s ir.LocatorStrategy) bool {
	return s == ir.LocatorTestID ||
		s == ir.LocatorRole ||
		s == ir.LocatorLabel ||
		s == ir.LocatorText ||
		s == ir.LocatorAltText ||
		s == ir.LocatorTitle ||
		s == ir.LocatorPlaceholder
}

func looksGenerated(s string) bool {
	for _, p := range generatedIDPatterns {
		if p.MatchString(s) {
			return true
		}
	}
	return false
}

var (
	reTestID        = regexp.MustCompile(`\[data-testid=["']?([^"'\]]+)["']?\]`)
	reDataPW        = regexp.MustCompile(`\[data-pw=["']?([^"'\]]+)["']?\]`)
	reSubmitBtn     = regexp.MustCompile(`button\[type=["']?submit["']?\]`)
	reEmailInput    = regexp.MustCompile(`input\[type=["']?email["']?\]`)
	rePasswordInput = regexp.MustCompile(`input\[type=["']?password["']?\]`)
	reAriaLabel     = regexp.MustCompile(`\[aria-label=["']?([^"'\]]+)["']?\]`)

	generatedIDPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^[a-f0-9]{8,}$`),     // hex IDs
		regexp.MustCompile(`^[a-z]+-[a-f0-9]+$`), // prefixed hex
		regexp.MustCompile(`^\d+$`),              // numeric IDs
	}
)
