package ingestion

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	ir "github.com/yagna-1/recast/recast-ir"
)

type workflowUseJSON struct {
	WorkflowName string         `json:"workflow_name"`
	Name         string         `json:"name,omitempty"`
	Steps        []workflowStep `json:"steps"`
	BaseURL      string         `json:"base_url,omitempty"`
	Metadata     *workflowMeta  `json:"metadata,omitempty"`
}

type workflowStep struct {
	Type        string  `json:"type"`
	URL         string  `json:"url,omitempty"`
	Selector    string  `json:"selector,omitempty"`
	CSSSelector string  `json:"cssSelector,omitempty"`
	TargetText  string  `json:"target_text,omitempty"`
	Value       string  `json:"value,omitempty"`
	Description string  `json:"description,omitempty"`
	X           float64 `json:"x,omitempty"`
	Y           float64 `json:"y,omitempty"`
	ScrollY     float64 `json:"scrollY,omitempty"`
	Key         string  `json:"key,omitempty"`
	Option      string  `json:"option,omitempty"`
	TimestampMs int64   `json:"timestamp_ms,omitempty"`
}

type workflowMeta struct {
	AgentModel string `json:"agent_model,omitempty"`
	RecordedAt string `json:"recorded_at,omitempty"`
}

type WorkflowUseIngester struct{}

func (w *WorkflowUseIngester) FormatName() string {
	return "workflow-use JSON"
}

func (w *WorkflowUseIngester) CanHandle(path string, data []byte) bool {
	if !bytes.Contains(data, []byte(`"steps"`)) {
		return false
	}
	return bytes.Contains(data, []byte(`"workflow_name"`)) ||
		(bytes.Contains(data, []byte(`"steps"`)) && bytes.Contains(data, []byte(`"type"`)))
}

func (w *WorkflowUseIngester) Parse(data []byte) (*ir.Trace, error) {
	var raw workflowUseJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("workflow-use: JSON parse error: %w", err)
	}

	if len(raw.Steps) == 0 {
		return nil, fmt.Errorf("workflow-use: workflow contains no steps")
	}

	name := raw.WorkflowName
	if name == "" {
		name = raw.Name
	}
	if name == "" {
		name = "unnamed_workflow"
	}
	name = strings.ReplaceAll(name, " ", "_")

	builder := ir.NewTrace(name).
		WithSourceFormat("workflow-use").
		WithBaseURL(raw.BaseURL)

	if raw.Metadata != nil {
		builder.WithAgentModel(raw.Metadata.AgentModel)
	}

	for i, step := range raw.Steps {
		irStep, err := convertWorkflowStep(step, i+1)
		if err != nil {
			builder.AddStep(ir.Step{
				ID:      fmt.Sprintf("step_%03d", i+1),
				Type:    ir.StepType(step.Type),
				Comment: fmt.Sprintf("// TODO: unsupported step type: %s — Original: %+v", step.Type, step),
			})
			continue
		}
		builder.AddStep(*irStep)
	}

	return builder.BuildUnchecked(), nil
}

func convertWorkflowStep(step workflowStep, idx int) (*ir.Step, error) {
	id := fmt.Sprintf("step_%03d", idx)
	stepType := normalizeWorkflowStepType(step.Type)

	switch stepType {
	case "navigate":
		url := step.URL
		if url == "" {
			url = step.Value
		}
		if url == "" {
			return nil, fmt.Errorf("navigate step has no URL")
		}
		return &ir.Step{
			ID:    id,
			Type:  ir.StepNavigate,
			Value: url,
			Wait:  ir.WaitSpec{Type: ir.WaitNetworkIdle, Timeout: 30000},
		}, nil

	case "click":
		if !hasWorkflowTarget(step) && (step.X == 0 && step.Y == 0) {
			return nil, fmt.Errorf("click step has no selector or coordinates")
		}
		target := buildTarget(step, step.Description, "click")
		return &ir.Step{
			ID:     id,
			Type:   ir.StepClick,
			Target: target,
		}, nil

	case "fill", "type", "input":
		target := buildTarget(step, step.Description, "fill")
		return &ir.Step{
			ID:     id,
			Type:   ir.StepFill,
			Target: target,
			Value:  step.Value,
		}, nil

	case "select", "select_option":
		target := buildTarget(step, step.Description, "select")
		return &ir.Step{
			ID:     id,
			Type:   ir.StepSelect,
			Target: target,
			Value:  step.Value,
		}, nil

	case "hover":
		target := buildTarget(step, step.Description, "hover")
		return &ir.Step{
			ID:     id,
			Type:   ir.StepHover,
			Target: target,
		}, nil

	case "wait_for", "wait_for_selector", "wait":
		if primarySelector(step) != "" {
			target := buildTarget(step, step.Description, "wait")
			return &ir.Step{
				ID:     id,
				Type:   ir.StepWaitForEl,
				Target: target,
				Wait:   ir.WaitSpec{Type: ir.WaitSelector, Value: target.Primary.Value, Timeout: 30000},
			}, nil
		}
		if step.URL != "" {
			return &ir.Step{
				ID:    id,
				Type:  ir.StepWaitForURL,
				Value: step.URL,
			}, nil
		}
		return nil, fmt.Errorf("wait step has no selector or URL")

	case "screenshot":
		return &ir.Step{
			ID:    id,
			Type:  ir.StepScreenshot,
			Value: step.Value,
		}, nil

	case "scroll":
		val := step.Value
		if val == "" {
			val = fmt.Sprintf("%.0f", step.ScrollY)
		}
		return &ir.Step{
			ID:    id,
			Type:  ir.StepScroll,
			Value: val,
		}, nil

	case "key", "keyboard", "press":
		key := step.Key
		if key == "" {
			key = step.Value
		}
		return &ir.Step{
			ID:    id,
			Type:  ir.StepKeyboard,
			Value: key,
		}, nil

	case "check":
		target := buildTarget(step, step.Description, "check")
		return &ir.Step{
			ID:     id,
			Type:   ir.StepCheck,
			Target: target,
		}, nil

	case "uncheck":
		target := buildTarget(step, step.Description, "uncheck")
		return &ir.Step{
			ID:     id,
			Type:   ir.StepUncheck,
			Target: target,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported step type: %q", step.Type)
	}
}

func hasWorkflowTarget(step workflowStep) bool {
	return primarySelector(step) != "" || strings.TrimSpace(step.TargetText) != ""
}

func primarySelector(step workflowStep) string {
	if strings.TrimSpace(step.Selector) != "" {
		return step.Selector
	}
	if strings.TrimSpace(step.CSSSelector) != "" {
		return step.CSSSelector
	}
	return ""
}

func normalizeWorkflowStepType(stepType string) string {
	switch strings.ToLower(strings.TrimSpace(stepType)) {
	case "navigation":
		return "navigate"
	default:
		return strings.ToLower(strings.TrimSpace(stepType))
	}
}

func buildTarget(step workflowStep, description, action string) *ir.Target {
	if primarySelector(step) == "" && strings.TrimSpace(step.TargetText) == "" && (step.X != 0 || step.Y != 0) {
		return &ir.Target{
			Primary: ir.Locator{
				Strategy:   ir.LocatorCSS,
				Value:      "*", // placeholder, will be replaced by optimizer
				Confidence: 0.1,
				Fragile:    true,
			},
			Fallbacks: []ir.Locator{
				{
					Strategy:   ir.LocatorCoords,
					Value:      fmt.Sprintf("%.0f,%.0f", step.X, step.Y),
					Confidence: ir.LocatorConfidence[ir.LocatorCoords],
					Fragile:    true,
				},
			},
			HumanDescription: description,
		}
	}

	target := &ir.Target{HumanDescription: description}
	if target.HumanDescription == "" && step.TargetText != "" {
		target.HumanDescription = step.TargetText
	}
	selector := primarySelector(step)

	switch {
	case selector != "":
		confidence := ir.LocatorConfidence[ir.LocatorCSS]
		fragile := ir.IsGeneratedSelector(selector)
		target.Primary = ir.Locator{
			Strategy:   ir.LocatorCSS,
			Value:      selector,
			Confidence: confidence,
			Fragile:    fragile,
		}
	case strings.TrimSpace(step.TargetText) != "":
		txt := strings.TrimSpace(step.TargetText)
		strategy := ir.LocatorText
		value := txt
		if isIdentifierLike(txt) {
			strategy = ir.LocatorCSS
			value = "#" + txt
		} else if action == "fill" {
			strategy = ir.LocatorLabel
		}
		target.Primary = ir.Locator{
			Strategy:   strategy,
			Value:      value,
			Confidence: ir.LocatorConfidence[strategy],
		}
	default:
		target.Primary = ir.Locator{
			Strategy:   ir.LocatorCSS,
			Value:      "*",
			Confidence: 0.1,
			Fragile:    true,
		}
	}

	if step.X != 0 || step.Y != 0 {
		target.Fallbacks = append(target.Fallbacks, ir.Locator{
			Strategy:   ir.LocatorCoords,
			Value:      fmt.Sprintf("%.0f,%.0f", step.X, step.Y),
			Confidence: ir.LocatorConfidence[ir.LocatorCoords],
			Fragile:    true,
		})
	}

	return target
}

var reIdentifierLike = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

func isIdentifierLike(v string) bool {
	return reIdentifierLike.MatchString(strings.TrimSpace(v))
}
