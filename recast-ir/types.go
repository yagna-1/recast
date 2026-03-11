// Package ir defines the Intermediate Representation (IR) for recast.
package ir

import "time"

const SchemaVersion = "1.0"

type Trace struct {
	SchemaVersion string        `json:"schema_version"`
	Name          string        `json:"name"`
	BaseURL       string        `json:"base_url,omitempty"`
	Steps         []Step        `json:"steps"`
	Metadata      TraceMetadata `json:"metadata,omitempty"`
}

type TraceMetadata struct {
	SourceFormat  string    `json:"source_format,omitempty"` // "workflow-use", "har", "cdp", "mcp"
	RecordedAt    time.Time `json:"recorded_at,omitempty"`
	AgentModel    string    `json:"agent_model,omitempty"`   // e.g. "gpt-4o-2024-08-06"
	SiteSnapshot  string    `json:"site_snapshot,omitempty"` // snapshot ID if recorded against frozen site
	RecastVersion string    `json:"recast_version,omitempty"`
}

type Step struct {
	ID      string      `json:"id"`
	Type    StepType    `json:"type"`
	Target  *Target     `json:"target,omitempty"` // nil for navigate, keyboard, wait_for_url
	Value   string      `json:"value,omitempty"`  // fill value, URL, key name, timeout ms
	Wait    WaitSpec    `json:"wait,omitempty"`
	Options StepOptions `json:"options,omitempty"`
	Comment string      `json:"comment,omitempty"` // emitted verbatim as a code comment
	Branch  *BranchStep `json:"branch,omitempty"`
}

type StepType string

const (
	StepNavigate   StepType = "navigate"
	StepClick      StepType = "click"
	StepFill       StepType = "fill"
	StepSelect     StepType = "select"
	StepHover      StepType = "hover"
	StepFocus      StepType = "focus"
	StepBlur       StepType = "blur"
	StepCheck      StepType = "check"
	StepUncheck    StepType = "uncheck"
	StepWaitForEl  StepType = "wait_for_element"
	StepWaitForURL StepType = "wait_for_url"
	StepAssert     StepType = "assert"
	StepScreenshot StepType = "screenshot"
	StepScroll     StepType = "scroll"
	StepKeyboard   StepType = "keyboard"
	StepFileUpload StepType = "file_upload"
	StepDialog     StepType = "dialog"
	StepFrame      StepType = "frame"
	StepBranch     StepType = "branch"
)

type Target struct {
	Primary Locator `json:"primary"`

	Fallbacks []Locator `json:"fallbacks,omitempty"`

	Context *TargetContext `json:"context,omitempty"`

	HumanDescription string `json:"human_description,omitempty"`
}

type Locator struct {
	Strategy   LocatorStrategy `json:"strategy"`
	Value      string          `json:"value"`
	Confidence float64         `json:"confidence"`
	Fragile    bool            `json:"fragile,omitempty"`
}

type LocatorStrategy string

const (
	LocatorTestID LocatorStrategy = "test-id"
	LocatorRole   LocatorStrategy = "role"

	LocatorLabel       LocatorStrategy = "label"
	LocatorText        LocatorStrategy = "text"
	LocatorAltText     LocatorStrategy = "alt-text"
	LocatorTitle       LocatorStrategy = "title"
	LocatorPlaceholder LocatorStrategy = "placeholder"

	LocatorCSS      LocatorStrategy = "css"
	LocatorXPathRel LocatorStrategy = "xpath-rel"
	LocatorXPathAbs LocatorStrategy = "xpath-abs"

	LocatorCoords LocatorStrategy = "coords"
)

var LocatorConfidence = map[LocatorStrategy]float64{
	LocatorTestID:      1.0,
	LocatorRole:        0.9,
	LocatorLabel:       0.85,
	LocatorText:        0.8,
	LocatorAltText:     0.75,
	LocatorTitle:       0.75,
	LocatorPlaceholder: 0.7,
	LocatorCSS:         0.5,
	LocatorXPathRel:    0.3,
	LocatorXPathAbs:    0.1,
	LocatorCoords:      0.05,
}

type WaitSpec struct {
	Type    WaitType `json:"type,omitempty"`
	Value   string   `json:"value,omitempty"`   // selector, URL pattern, or empty
	Timeout int      `json:"timeout,omitempty"` // ms, 0 = use framework default (30s)
}

type WaitType string

const (
	WaitNone           WaitType = "none"
	WaitNetworkIdle    WaitType = "network_idle"
	WaitDOMContentLoad WaitType = "dom_content"
	WaitSelector       WaitType = "selector"
	WaitSelectorHidden WaitType = "selector_hidden"
	WaitNavigation     WaitType = "navigation"
	WaitURL            WaitType = "url"
	WaitDelay          WaitType = "delay"
)

type StepOptions struct {
	Timeout   int      `json:"timeout,omitempty"`
	Force     bool     `json:"force,omitempty"`
	Strict    bool     `json:"strict,omitempty"`
	Modifiers []string `json:"modifiers,omitempty"` // "Shift", "Control", "Alt"
}

type TargetContext struct {
	Type        ContextType `json:"type"`
	FrameTarget *Target     `json:"frame_target,omitempty"`
}

type ContextType string

const (
	ContextIframe    ContextType = "iframe"
	ContextShadowDOM ContextType = "shadow-dom"
)

type BranchStep struct {
	Condition  string `json:"condition"` // human-readable: "CAPTCHA dialog visible"
	ThenSteps  []Step `json:"then_steps"`
	ElseSteps  []Step `json:"else_steps,omitempty"`
	Incomplete bool   `json:"incomplete,omitempty"` // true if only one branch was observed
}

type Warning struct {
	StepID  string `json:"step_id,omitempty"`
	Pass    string `json:"pass,omitempty"`
	Message string `json:"message"`
}
