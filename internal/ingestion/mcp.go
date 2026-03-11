package ingestion

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	ir "github.com/yagna-1/recast/recast-ir"
)

type mcpEntry struct {
	Type   string         `json:"type"` // "tool_call" or "tool_result"
	Tool   string         `json:"tool"`
	Params mcpToolParams  `json:"params"`
	Result *mcpToolResult `json:"result,omitempty"`
}

type mcpToolParams struct {
	URL string `json:"url,omitempty"`

	Element string `json:"element,omitempty"`
	Ref     string `json:"ref,omitempty"`

	Text     string `json:"text,omitempty"`
	Value    string `json:"value,omitempty"`
	Selector string `json:"selector,omitempty"`

	Option string `json:"option,omitempty"`

	Key string `json:"key,omitempty"`

	Direction string `json:"direction,omitempty"`
	Amount    int    `json:"amount,omitempty"`
}

type mcpToolResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

type MCPIngester struct{}

func (m *MCPIngester) FormatName() string {
	return "MCP Tool Call Log"
}

func (m *MCPIngester) CanHandle(path string, data []byte) bool {
	return bytes.Contains(data, []byte("browser_")) &&
		(bytes.Contains(data, []byte(`"tool":`)) || bytes.Contains(data, []byte(`"tool_call"`)))
}

func (m *MCPIngester) Parse(data []byte) (*ir.Trace, error) {
	var entries []mcpEntry

	if err := json.Unmarshal(data, &entries); err != nil {
		lines := bytes.Split(data, []byte("\n"))
		for _, line := range lines {
			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				continue
			}
			var entry mcpEntry
			if err := json.Unmarshal(line, &entry); err != nil {
				continue
			}
			entries = append(entries, entry)
		}
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("MCP: no parseable entries found")
	}

	builder := ir.NewTrace("mcp_workflow").WithSourceFormat("mcp")

	for i, entry := range entries {
		if entry.Type == "tool_result" {
			continue
		}
		if !strings.HasPrefix(entry.Tool, "browser_") {
			continue
		}

		step := convertMCPEntry(entry, i+1)
		if step != nil {
			builder.AddStep(*step)
		}
	}

	trace := builder.BuildUnchecked()
	if len(trace.Steps) == 0 {
		return nil, fmt.Errorf("MCP: no browser tool calls found in log")
	}

	return trace, nil
}

func convertMCPEntry(entry mcpEntry, idx int) *ir.Step {
	id := fmt.Sprintf("step_%03d", idx)
	p := entry.Params

	switch entry.Tool {
	case "browser_navigate":
		if p.URL == "" {
			return nil
		}
		return &ir.Step{
			ID:    id,
			Type:  ir.StepNavigate,
			Value: p.URL,
			Wait:  ir.WaitSpec{Type: ir.WaitNetworkIdle, Timeout: 30000},
		}

	case "browser_click":
		description := p.Element
		var target *ir.Target
		if p.Selector != "" {
			target = ir.TargetFromCSS(p.Selector, description)
		} else if p.Element != "" {
			target = ir.TargetFromText(p.Element, description)
		} else {
			return nil
		}
		return &ir.Step{
			ID:     id,
			Type:   ir.StepClick,
			Target: target,
		}

	case "browser_type", "browser_fill":
		value := p.Text
		if value == "" {
			value = p.Value
		}
		var target *ir.Target
		if p.Selector != "" {
			target = ir.TargetFromCSS(p.Selector, p.Element)
		} else if p.Element != "" {
			target = ir.TargetFromLabel(p.Element, p.Element)
		} else {
			return nil
		}
		return &ir.Step{
			ID:     id,
			Type:   ir.StepFill,
			Target: target,
			Value:  value,
		}

	case "browser_select":
		var target *ir.Target
		if p.Selector != "" {
			target = ir.TargetFromCSS(p.Selector, p.Element)
		} else {
			return nil
		}
		return &ir.Step{
			ID:     id,
			Type:   ir.StepSelect,
			Target: target,
			Value:  p.Option,
		}

	case "browser_key_press":
		return &ir.Step{
			ID:    id,
			Type:  ir.StepKeyboard,
			Value: p.Key,
		}

	case "browser_screenshot":
		return &ir.Step{
			ID:    id,
			Type:  ir.StepScreenshot,
			Value: "screenshot.png",
		}

	case "browser_scroll":
		dir := p.Direction
		if dir == "" {
			dir = "down"
		}
		return &ir.Step{
			ID:    id,
			Type:  ir.StepScroll,
			Value: dir,
		}

	case "browser_wait_for":
		if p.Selector != "" {
			target := ir.TargetFromCSS(p.Selector, "")
			return &ir.Step{
				ID:     id,
				Type:   ir.StepWaitForEl,
				Target: target,
			}
		}
		if p.URL != "" {
			return &ir.Step{
				ID:    id,
				Type:  ir.StepWaitForURL,
				Value: p.URL,
			}
		}
		return nil
	}

	return nil
}
