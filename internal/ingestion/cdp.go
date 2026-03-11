package ingestion

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	ir "github.com/yourorg/recast/recast-ir"
)

type cdpEvent struct {
	Method    string          `json:"method"`
	Params    json.RawMessage `json:"params"`
	Timestamp float64         `json:"timestamp,omitempty"`
}

type cdpMouseEvent struct {
	Type      string  `json:"type"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Button    string  `json:"button"`
	Modifiers int     `json:"modifiers"`
}

type cdpKeyEvent struct {
	Type           string `json:"type"`
	Key            string `json:"key"`
	Text           string `json:"text"`
	UnmodifiedText string `json:"unmodifiedText"`
	AutoRepeat     bool   `json:"autoRepeat"`
}

type cdpNavigateEvent struct {
	URL string `json:"url"`
}

type cdpInputInsertText struct {
	Text string `json:"text"`
}

type CDPIngester struct{}

func (c *CDPIngester) FormatName() string {
	return "CDP Event Log"
}

func (c *CDPIngester) CanHandle(path string, data []byte) bool {
	return bytes.Contains(data, []byte(`"method"`)) &&
		(bytes.Contains(data, []byte(`"Input.dispatchMouseEvent"`)) ||
			bytes.Contains(data, []byte(`"Page.navigate"`)) ||
			bytes.Contains(data, []byte(`"Input.dispatchKeyEvent"`)))
}

func (c *CDPIngester) Parse(data []byte) (*ir.Trace, error) {
	var events []cdpEvent

	if err := json.Unmarshal(data, &events); err != nil {
		lines := bytes.Split(data, []byte("\n"))
		for _, line := range lines {
			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				continue
			}
			var event cdpEvent
			if err := json.Unmarshal(line, &event); err != nil {
				continue
			}
			events = append(events, event)
		}
	}

	if len(events) == 0 {
		return nil, fmt.Errorf("CDP: no parseable events found")
	}

	builder := ir.NewTrace("cdp_workflow").WithSourceFormat("cdp")

	steps := convertCDPEvents(events)
	for _, step := range steps {
		builder.AddStep(step)
	}

	trace := builder.BuildUnchecked()
	if len(trace.Steps) == 0 {
		return nil, fmt.Errorf("CDP: no actionable events found in log")
	}

	return trace, nil
}

func convertCDPEvents(events []cdpEvent) []ir.Step {
	var steps []ir.Step
	var pendingText strings.Builder
	var lastClickCoords string
	var lastClickDesc string

	flush := func() {
		if pendingText.Len() == 0 {
			return
		}
		text := pendingText.String()
		pendingText.Reset()

		if len(steps) > 0 {
			prev := &steps[len(steps)-1]
			if prev.Type == ir.StepClick && prev.Target != nil &&
				prev.Target.Primary.Strategy == ir.LocatorCoords {
				prev.Type = ir.StepFill
				prev.Value = text
				prev.Comment = strings.ReplaceAll(prev.Comment, "CDP: click", "CDP: fill")
				lastClickCoords = ""
				lastClickDesc = ""
				return
			}
		}

		if lastClickCoords != "" {
			steps = append(steps, ir.Step{
				Type: ir.StepFill,
				Target: &ir.Target{
					Primary: ir.Locator{
						Strategy:   ir.LocatorCoords,
						Value:      lastClickCoords,
						Confidence: ir.LocatorConfidence[ir.LocatorCoords],
						Fragile:    true,
					},
					HumanDescription: lastClickDesc,
				},
				Value:   text,
				Comment: fmt.Sprintf("// CDP: fill at %s — selector hardening needed", lastClickCoords),
			})
		}
	}

	for _, event := range events {
		switch event.Method {
		case "Page.navigate":
			flush()
			var nav cdpNavigateEvent
			if err := json.Unmarshal(event.Params, &nav); err != nil || nav.URL == "" {
				continue
			}
			lastClickCoords = ""
			steps = append(steps, ir.Step{
				Type:  ir.StepNavigate,
				Value: nav.URL,
				Wait:  ir.WaitSpec{Type: ir.WaitNetworkIdle, Timeout: 30000},
			})

		case "Input.dispatchMouseEvent":
			var mouse cdpMouseEvent
			if err := json.Unmarshal(event.Params, &mouse); err != nil {
				continue
			}
			if mouse.Type == "mousePressed" && mouse.Button == "left" {
				flush()
				coords := fmt.Sprintf("%.0f,%.0f", mouse.X, mouse.Y)
				desc := fmt.Sprintf("element at (%.0f, %.0f)", mouse.X, mouse.Y)
				lastClickCoords = coords
				lastClickDesc = desc
				target := &ir.Target{
					Primary: ir.Locator{
						Strategy:   ir.LocatorCoords,
						Value:      coords,
						Confidence: ir.LocatorConfidence[ir.LocatorCoords],
						Fragile:    true,
					},
					HumanDescription: desc,
				}
				steps = append(steps, ir.Step{
					Type:    ir.StepClick,
					Target:  target,
					Comment: fmt.Sprintf("// CDP: click at (%s) — selector hardening needed", coords),
				})
			}

		case "Input.insertText":
			var insert cdpInputInsertText
			if err := json.Unmarshal(event.Params, &insert); err != nil {
				continue
			}
			pendingText.WriteString(insert.Text)

		case "Input.dispatchKeyEvent":
			var key cdpKeyEvent
			if err := json.Unmarshal(event.Params, &key); err != nil {
				continue
			}
			if key.Type == "keyDown" {
				switch key.Key {
				case "Enter", "Tab", "Escape":
					flush()
					lastClickCoords = ""
					steps = append(steps, ir.Step{
						Type:  ir.StepKeyboard,
						Value: key.Key,
					})
				}
			}
		}
	}

	flush()
	return steps
}
