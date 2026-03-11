package ir

import (
	"fmt"
	"time"
)

type TraceBuilder struct {
	trace   Trace
	stepIdx int
}

func NewTrace(name string) *TraceBuilder {
	return &TraceBuilder{
		trace: Trace{
			SchemaVersion: SchemaVersion,
			Name:          name,
			Metadata: TraceMetadata{
				RecastVersion: "0.1.0",
				RecordedAt:    time.Now(),
			},
		},
	}
}

func (b *TraceBuilder) WithBaseURL(url string) *TraceBuilder {
	b.trace.BaseURL = url
	return b
}

func (b *TraceBuilder) WithSourceFormat(format string) *TraceBuilder {
	b.trace.Metadata.SourceFormat = format
	return b
}

func (b *TraceBuilder) WithAgentModel(model string) *TraceBuilder {
	b.trace.Metadata.AgentModel = model
	return b
}

func (b *TraceBuilder) WithRecordedAt(t time.Time) *TraceBuilder {
	b.trace.Metadata.RecordedAt = t
	return b
}

func (b *TraceBuilder) AddStep(step Step) *TraceBuilder {
	b.stepIdx++
	if step.ID == "" {
		step.ID = fmt.Sprintf("step_%03d", b.stepIdx)
	}
	b.trace.Steps = append(b.trace.Steps, step)
	return b
}

func (b *TraceBuilder) Navigate(url string) *TraceBuilder {
	return b.AddStep(Step{
		Type:  StepNavigate,
		Value: url,
		Wait:  WaitSpec{Type: WaitNetworkIdle, Timeout: 30000},
	})
}

func (b *TraceBuilder) Click(target *Target) *TraceBuilder {
	return b.AddStep(Step{
		Type:   StepClick,
		Target: target,
	})
}

func (b *TraceBuilder) Fill(target *Target, value string) *TraceBuilder {
	return b.AddStep(Step{
		Type:   StepFill,
		Target: target,
		Value:  value,
	})
}

func (b *TraceBuilder) Build() (*Trace, *ValidationResult) {
	result := Validate(&b.trace)
	if result.HasErrors() {
		return nil, result
	}
	t := b.trace
	return &t, result
}

func (b *TraceBuilder) BuildUnchecked() *Trace {
	t := b.trace
	return &t
}

func TargetFromCSS(selector string, description string) *Target {
	confidence := LocatorConfidence[LocatorCSS]
	fragile := IsGeneratedSelector(selector)
	return &Target{
		Primary: Locator{
			Strategy:   LocatorCSS,
			Value:      selector,
			Confidence: confidence,
			Fragile:    fragile,
		},
		HumanDescription: description,
	}
}

func TargetFromRole(role, name string, description string) *Target {
	value := fmt.Sprintf("role=%s[name=%q]", role, name)
	return &Target{
		Primary: Locator{
			Strategy:   LocatorRole,
			Value:      value,
			Confidence: LocatorConfidence[LocatorRole],
		},
		HumanDescription: description,
	}
}

func TargetFromLabel(label string, description string) *Target {
	return &Target{
		Primary: Locator{
			Strategy:   LocatorLabel,
			Value:      label,
			Confidence: LocatorConfidence[LocatorLabel],
		},
		HumanDescription: description,
	}
}

func TargetFromTestID(testID string, description string) *Target {
	return &Target{
		Primary: Locator{
			Strategy:   LocatorTestID,
			Value:      fmt.Sprintf("[data-testid=%q]", testID),
			Confidence: LocatorConfidence[LocatorTestID],
		},
		HumanDescription: description,
	}
}

func TargetFromText(text string, description string) *Target {
	return &Target{
		Primary: Locator{
			Strategy:   LocatorText,
			Value:      text,
			Confidence: LocatorConfidence[LocatorText],
		},
		HumanDescription: description,
	}
}

func (t *Target) WithFallback(strategy LocatorStrategy, value string) *Target {
	confidence := LocatorConfidence[strategy]
	fragile := strategy == LocatorXPathAbs || strategy == LocatorCoords
	t.Fallbacks = append(t.Fallbacks, Locator{
		Strategy:   strategy,
		Value:      value,
		Confidence: confidence,
		Fragile:    fragile,
	})
	return t
}

func (t *Target) WithDescription(desc string) *Target {
	t.HumanDescription = desc
	return t
}

func (t *Target) WithContext(ctxType ContextType, frameTarget *Target) *Target {
	t.Context = &TargetContext{
		Type:        ctxType,
		FrameTarget: frameTarget,
	}
	return t
}
