package optimizer

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	ir "github.com/yagna-1/recast/recast-ir"
)

type credentialPattern struct {
	Name    string
	Pattern *regexp.Regexp
	EnvName string
}

var credentialPatterns = []credentialPattern{
	{
		Name:    "email",
		Pattern: regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`),
		EnvName: "TEST_EMAIL",
	},
	{
		Name:    "password",
		Pattern: regexp.MustCompile(`(?i)(password|passwd|secret|pass)`),
		EnvName: "TEST_PASSWORD",
	},
	{
		Name:    "api_key",
		Pattern: regexp.MustCompile(`^[A-Za-z0-9_\-]{20,}$`),
		EnvName: "API_KEY",
	},
	{
		Name:    "jwt",
		Pattern: regexp.MustCompile(`^eyJ[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+\.`),
		EnvName: "AUTH_TOKEN",
	},
	{
		Name:    "credit_card",
		Pattern: regexp.MustCompile(`^\d{4}[\s\-]?\d{4}[\s\-]?\d{4}[\s\-]?\d{4}$`),
		EnvName: "CC_NUMBER",
	},
}

const shannonEntropyThreshold = 3.5

func runCredentialSanitization(trace *ir.Trace) (*ir.Trace, []ir.Warning, int, map[string]string) {
	var warnings []ir.Warning
	envVars := make(map[string]string)
	sanitized := 0
	varCounter := 0

	valueToVar := make(map[string]string)

	steps := make([]ir.Step, len(trace.Steps))
	copy(steps, trace.Steps)

	for i, step := range steps {
		if step.Type != ir.StepFill {
			continue
		}
		value := step.Value

		if strings.HasPrefix(value, "process.env.") || strings.HasPrefix(value, "os.environ") {
			continue
		}

		if existing, ok := valueToVar[value]; ok {
			steps[i].Value = envVarRef(existing)
			steps[i].Comment = addSanitizeComment(step.Comment, existing, "duplicate sanitized value")
			sanitized++
			continue
		}

		detectedName := ""
		reason := ""

		for _, pat := range credentialPatterns {
			if pat.Pattern.MatchString(value) {
				detectedName = pat.EnvName
				reason = fmt.Sprintf("detected: %s pattern", pat.Name)
				break
			}
		}

		if detectedName == "" && shannonEntropy(value) > shannonEntropyThreshold && len(value) >= 8 {
			varCounter++
			detectedName = fmt.Sprintf("RECAST_VAR_%d", varCounter)
			reason = fmt.Sprintf("detected: high-entropy string (entropy: %.1f bits/char)", shannonEntropy(value))
		}

		if detectedName == "" {
			continue
		}

		finalName := ensureUnique(detectedName, envVars)

		envVars[finalName] = fmt.Sprintf("# step %s: fill value was sanitized (%s)", step.ID, reason)
		valueToVar[value] = finalName
		steps[i].Value = envVarRef(finalName)
		steps[i].Comment = addSanitizeComment(step.Comment, finalName, reason)

		warnings = append(warnings, ir.Warning{
			StepID:  step.ID,
			Pass:    "sanitize",
			Message: fmt.Sprintf("replaced fill value with %s (%s)", envVarRef(finalName), reason),
		})
		sanitized++
	}

	out := *trace
	out.Steps = steps
	return &out, warnings, sanitized, envVars
}

func envVarRef(name string) string {
	return fmt.Sprintf("process.env.%s", name)
}

func shannonEntropy(s string) float64 {
	if len(s) == 0 {
		return 0
	}
	freq := make(map[rune]int)
	for _, c := range s {
		freq[c]++
	}
	n := float64(len([]rune(s)))
	entropy := 0.0
	for _, count := range freq {
		p := float64(count) / n
		entropy -= p * math.Log2(p)
	}
	return entropy
}

func ensureUnique(name string, existing map[string]string) string {
	if _, ok := existing[name]; !ok {
		return name
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s_%d", name, i)
		if _, ok := existing[candidate]; !ok {
			return candidate
		}
	}
}

func addSanitizeComment(existing, varName, reason string) string {
	note := fmt.Sprintf("// sanitized → %s (%s)", varName, reason)
	if existing != "" {
		return existing + " | " + note
	}
	return note
}
