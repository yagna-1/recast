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
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("ingestion: cannot read %q: %w", path, err)
	}

	const maxSize = 52_428_800 // 50 MB
	if len(data) > maxSize {
		return nil, "", fmt.Errorf("ingestion: file %q exceeds 50MB limit (%d bytes)", path, len(data))
	}

	ing, err := Detect(path, data)
	if err != nil {
		return nil, "", err
	}

	trace, err := ing.Parse(data)
	if err != nil {
		return nil, "", fmt.Errorf("ingestion: %s: %w", ing.FormatName(), err)
	}

	return trace, ing.FormatName(), nil
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
