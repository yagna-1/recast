package ingestion

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	ir "github.com/yagna-1/recast/recast-ir"
)

type astragraphAuditEntry struct {
	WorkflowID     string          `json:"workflow_id"`
	AgentID        string          `json:"agent_id"`
	ToolName       string          `json:"tool_name"`
	Arguments      json.RawMessage `json:"arguments"`
	Status         string          `json:"status"` // "allowed" | "blocked"
	DeviationScore float32         `json:"deviation_score"`
	RuleID         string          `json:"rule_id,omitempty"`
	Timestamp      string          `json:"timestamp"`
}

type AstraGraphAuditIngester struct{}

func (a *AstraGraphAuditIngester) FormatName() string {
	return "AstraGraph Audit Trail"
}

func (a *AstraGraphAuditIngester) CanHandle(path string, data []byte) bool {
	lowerPath := strings.ToLower(path)
	if strings.Contains(lowerPath, "astragraph") && strings.Contains(lowerPath, "audit") {
		return true
	}
	return bytes.Contains(data, []byte(`"workflow_id"`)) &&
		bytes.Contains(data, []byte(`"tool_name"`)) &&
		bytes.Contains(data, []byte(`"status"`))
}

func (a *AstraGraphAuditIngester) Parse(data []byte) (*ir.Trace, error) {
	var entries []astragraphAuditEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		lines := bytes.Split(data, []byte("\n"))
		for _, line := range lines {
			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				continue
			}
			var entry astragraphAuditEntry
			if err := json.Unmarshal(line, &entry); err != nil {
				continue
			}
			entries = append(entries, entry)
		}
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("AstraGraph: no parseable audit entries found")
	}

	traceName := "astragraph_audit"
	if entries[0].WorkflowID != "" {
		traceName = sanitizeTraceName(entries[0].WorkflowID)
	}
	builder := ir.NewTrace(traceName).WithSourceFormat("astragraph-audit")

	for i, entry := range entries {
		step, ok := convertAstraGraphEntry(entry, i+1)
		if !ok {
			continue
		}
		builder.AddStep(*step)
	}

	trace := builder.BuildUnchecked()
	if len(trace.Steps) == 0 {
		return nil, fmt.Errorf("AstraGraph: no usable audit entries found")
	}
	return trace, nil
}

func convertAstraGraphEntry(entry astragraphAuditEntry, idx int) (*ir.Step, bool) {
	toolName := strings.TrimSpace(entry.ToolName)
	if toolName == "" {
		return nil, false
	}

	var params mcpToolParams
	if len(entry.Arguments) > 0 {
		_ = json.Unmarshal(entry.Arguments, &params)
	}

	step := convertMCPEntry(mcpEntry{
		Type:   "tool_call",
		Tool:   toolName,
		Params: params,
	}, idx)

	status := strings.ToLower(strings.TrimSpace(entry.Status))
	comment := blockedComment(entry.RuleID, entry.DeviationScore)

	if status == "blocked" {
		if step == nil {
			step = &ir.Step{
				ID:      fmt.Sprintf("step_%03d", idx),
				Type:    ir.StepKeyboard,
				Value:   "blocked",
				Comment: comment,
			}
			return step, true
		}
		if step.Comment == "" {
			step.Comment = comment
		} else {
			step.Comment = step.Comment + "; " + comment
		}
		return step, true
	}

	if status == "allowed" && step != nil {
		return step, true
	}

	return nil, false
}

func blockedComment(ruleID string, score float32) string {
	rule := strings.TrimSpace(ruleID)
	if rule == "" {
		rule = "unknown"
	}
	return fmt.Sprintf("BLOCKED by AstraGraph: rule_id=%s, score=%.2f", rule, score)
}

func sanitizeTraceName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return "astragraph_audit"
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteRune('_')
			lastUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "astragraph_audit"
	}
	return out
}
