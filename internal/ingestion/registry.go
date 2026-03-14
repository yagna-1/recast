// Package ingestion contains adapters that parse various input formats into IR.
package ingestion

import (
	"fmt"
	"os"

	ir "github.com/yagna-1/recast/recast-ir"
)

type Ingester interface {
	CanHandle(path string, data []byte) bool

	Parse(data []byte) (*ir.Trace, error)

	FormatName() string
}

var registry []Ingester

func init() {
	registry = []Ingester{
		&WorkflowUseIngester{},
		&AstraGraphAuditIngester{},
		&MCPIngester{},
		&CDPIngester{},
		&HARIngester{},
	}
}

func Detect(path string, data []byte) (Ingester, error) {
	for _, ing := range registry {
		if ing.CanHandle(path, data) {
			return ing, nil
		}
	}
	return nil, fmt.Errorf("ingestion: no adapter found for %q — run 'recast formats' to see supported formats", path)
}

func ParseFile(path string) (*ir.Trace, string, error) {
	return parseFile(path, "")
}

func ParseFileWithFormat(path string, from string) (*ir.Trace, string, error) {
	return parseFile(path, from)
}

func parseFile(path string, from string) (*ir.Trace, string, error) {
	const maxSize = 52_428_800 // 50 MB

	// Guard file size before loading into memory.
	if info, err := os.Stat(path); err == nil {
		if info.Size() > maxSize {
			return nil, "", fmt.Errorf("ingestion: file %q exceeds 50MB limit (%d bytes)", path, info.Size())
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("ingestion: cannot read %q: %w", path, err)
	}

	// Keep a post-read check as a fallback for non-regular paths.
	if len(data) > maxSize {
		return nil, "", fmt.Errorf("ingestion: file %q exceeds 50MB limit (%d bytes)", path, len(data))
	}

	var ing Ingester
	if from != "" {
		ing, err = ingesterByFormat(from)
		if err != nil {
			return nil, "", err
		}
	} else {
		ing, err = Detect(path, data)
		if err != nil {
			return nil, "", err
		}
	}

	trace, err := ing.Parse(data)
	if err != nil {
		return nil, "", fmt.Errorf("ingestion: %s: %w", ing.FormatName(), err)
	}

	return trace, ing.FormatName(), nil
}

func ingesterByFormat(from string) (Ingester, error) {
	switch normalizeFormat(from) {
	case "workflow-use", "workflow_use", "workflow":
		return &WorkflowUseIngester{}, nil
	case "har":
		return &HARIngester{}, nil
	case "cdp":
		return &CDPIngester{}, nil
	case "mcp":
		return &MCPIngester{}, nil
	case "astragraph-audit", "astragraph_audit", "astragraph":
		return &AstraGraphAuditIngester{}, nil
	default:
		return nil, fmt.Errorf("ingestion: unsupported --from %q", from)
	}
}

func normalizeFormat(s string) string {
	return normalizeDashesAndCase(s)
}

func normalizeDashesAndCase(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			r = r - 'A' + 'a'
		}
		if r == '_' {
			r = '-'
		}
		out = append(out, r)
	}
	return string(out)
}

func AllFormats() []FormatInfo {
	infos := make([]FormatInfo, len(registry))
	for i, ing := range registry {
		infos[i] = FormatInfo{Name: ing.FormatName()}
	}
	return infos
}

type FormatInfo struct {
	Name string
}
