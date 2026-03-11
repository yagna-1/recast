package ir

import (
	"fmt"
	"strings"
)

type ValidationError struct {
	StepID  string
	Message string
}

func (e *ValidationError) Error() string {
	if e.StepID != "" {
		return fmt.Sprintf("ir: [%s] %s", e.StepID, e.Message)
	}
	return fmt.Sprintf("ir: %s", e.Message)
}

type ValidationResult struct {
	Errors   []*ValidationError
	Warnings []Warning
}

func (r *ValidationResult) HasErrors() bool {
	return len(r.Errors) > 0
}

func (r *ValidationResult) Error() string {
	if len(r.Errors) == 0 {
		return ""
	}
	msgs := make([]string, len(r.Errors))
	for i, e := range r.Errors {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "; ")
}

func Validate(trace *Trace) *ValidationResult {
	result := &ValidationResult{}

	if trace == nil {
		result.Errors = append(result.Errors, &ValidationError{Message: "trace is nil"})
		return result
	}

	if len(trace.Steps) == 0 {
		result.Errors = append(result.Errors, &ValidationError{Message: "trace contains no steps"})
		return result
	}

	seenIDs := make(map[string]bool)

	for i, step := range trace.Steps {
		stepID := step.ID
		if stepID == "" {
			result.Errors = append(result.Errors, &ValidationError{
				Message: fmt.Sprintf("step at index %d has no ID", i),
			})
			stepID = fmt.Sprintf("step_%03d", i+1)
		}

		if seenIDs[stepID] {
			result.Errors = append(result.Errors, &ValidationError{
				StepID:  stepID,
				Message: "duplicate step ID",
			})
		}
		seenIDs[stepID] = true

		validateStep(step, stepID, trace.BaseURL, result)
	}

	return result
}

func validateStep(step Step, stepID string, baseURL string, result *ValidationResult) {
	switch step.Type {
	case StepNavigate:
		if step.Value == "" {
			result.Errors = append(result.Errors, &ValidationError{
				StepID:  stepID,
				Message: "navigate step has no URL",
			})
		} else if strings.HasPrefix(step.Value, "/") && baseURL == "" {
			result.Errors = append(result.Errors, &ValidationError{
				StepID:  stepID,
				Message: fmt.Sprintf("navigate has relative URL %q but trace has no base_url", step.Value),
			})
		} else if step.Value == "about:blank" || strings.HasPrefix(step.Value, "chrome://") {
			result.Errors = append(result.Errors, &ValidationError{
				StepID:  stepID,
				Message: fmt.Sprintf("internal browser URL %q is not reproducible", step.Value),
			})
		}

	case StepFill:
		if step.Target == nil {
			result.Errors = append(result.Errors, &ValidationError{
				StepID:  stepID,
				Message: "fill step has no target",
			})
		}
		if step.Value == "" {
			result.Warnings = append(result.Warnings, Warning{
				StepID:  stepID,
				Pass:    "validate",
				Message: "fill step has no value — will emit empty string",
			})
		}

	case StepBranch:
		if step.Branch == nil {
			result.Errors = append(result.Errors, &ValidationError{
				StepID:  stepID,
				Message: "branch step has no BranchStep data",
			})
		} else if step.Branch.Incomplete {
			result.Warnings = append(result.Warnings, Warning{
				StepID:  stepID,
				Pass:    "validate",
				Message: fmt.Sprintf("branch condition %q has only one observed branch — else-branch must be implemented manually", step.Branch.Condition),
			})
		}

	case StepKeyboard, StepWaitForURL, StepScreenshot, StepScroll:
		if step.Value == "" {
			result.Warnings = append(result.Warnings, Warning{
				StepID:  stepID,
				Pass:    "validate",
				Message: fmt.Sprintf("%s step has no value", step.Type),
			})
		}

	case "":
		result.Errors = append(result.Errors, &ValidationError{
			StepID:  stepID,
			Message: "step has empty type",
		})

	default:
		if step.Target == nil {
			result.Errors = append(result.Errors, &ValidationError{
				StepID:  stepID,
				Message: fmt.Sprintf("%s step has no target", step.Type),
			})
		}
	}

	if step.Target != nil {
		validateTarget(step.Target, stepID, result)
	}
}

func validateTarget(target *Target, stepID string, result *ValidationResult) {
	if target.Primary.Value == "" && target.Primary.Strategy == "" {
		result.Errors = append(result.Errors, &ValidationError{
			StepID:  stepID,
			Message: "target has no primary locator",
		})
		return
	}

	if target.Primary.Strategy == LocatorCoords {
		result.Errors = append(result.Errors, &ValidationError{
			StepID:  stepID,
			Message: "coordinate-only locator cannot be primary — add a semantic locator",
		})
	}

	if target.Context != nil && target.Context.FrameTarget != nil {
		if target.Context.FrameTarget.Context != nil {
			result.Errors = append(result.Errors, &ValidationError{
				StepID:  stepID,
				Message: "frame context references a nested frame context — circular reference suspected",
			})
		}
	}
}
