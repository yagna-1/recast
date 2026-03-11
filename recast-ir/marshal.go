package ir

import (
	"encoding/json"
	"fmt"
	"regexp"
)

func Marshal(trace *Trace) ([]byte, error) {
	return json.MarshalIndent(trace, "", "  ")
}

func Unmarshal(data []byte) (*Trace, error) {
	var trace Trace
	if err := json.Unmarshal(data, &trace); err != nil {
		return nil, fmt.Errorf("ir: unmarshal: %w", err)
	}
	return &trace, nil
}

var generatedClassPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^css-[a-z0-9]+$`),                       // MUI: css-abc123
	regexp.MustCompile(`^[a-zA-Z]+-[a-f0-9]{5,}$`),              // styled-components: sc-abc12
	regexp.MustCompile(`^sc-[A-Za-z0-9]+$`),                     // styled-components: sc-bXCLTE
	regexp.MustCompile(`^[a-z]+-module__[a-z]+__[a-zA-Z0-9]+$`), // CSS Modules
	regexp.MustCompile(`^_[a-z0-9]{4,}$`),                       // Svelte scoped classes
	regexp.MustCompile(`^[a-z]{1,2}[0-9]+[a-z]*$`),              // Tailwind JIT arbitrary
}

func IsGeneratedSelector(selector string) bool {
	classPattern := regexp.MustCompile(`\.([a-zA-Z_-][a-zA-Z0-9_-]*)`)
	matches := classPattern.FindAllStringSubmatch(selector, -1)
	for _, match := range matches {
		if len(match) > 1 {
			className := match[1]
			for _, pattern := range generatedClassPatterns {
				if pattern.MatchString(className) {
					return true
				}
			}
		}
	}
	return false
}
