// Package emitter contains code generators that transform IR into target language output.
package emitter

import (
	"fmt"
	"strconv"
	"strings"

	ir "github.com/yagna-1/recast/recast-ir"
)

type Emitter interface {
	TargetName() string

	Emit(trace *ir.Trace, envVars map[string]string) (*EmitResult, error)

	FileExtension() string
}

type EmitResult struct {
	TestFile string
	AuxFiles map[string]string
}

const (
	TargetPlaywrightTS = "playwright-ts"
	TargetPlaywrightPy = "playwright-py"
	TargetIRJSON       = "ir-json"
)

var registry []Emitter

func init() {
	registry = []Emitter{
		&PlaywrightTSEmitter{},
		&PlaywrightPyEmitter{},
	}
}

func Get(target string) (Emitter, error) {
	for _, e := range registry {
		if e.TargetName() == target {
			return e, nil
		}
	}
	return nil, fmt.Errorf("emitter: unknown target %q — supported: %s", target, AllTargets())
}

func AllTargets() string {
	names := make([]string, len(registry))
	for i, e := range registry {
		names[i] = e.TargetName()
	}
	return strings.Join(names, ", ")
}

func jsString(s string) string {
	// strconv.Quote handles control chars, newlines, unicode, and quotes safely.
	return strconv.Quote(s)
}

func jsEnvOrString(s string) string {
	if strings.HasPrefix(s, "process.env.") {
		return s + "!"
	}
	return jsString(s)
}

func pyEnvOrString(s string) string {
	if strings.HasPrefix(s, "process.env.") {
		varName := strings.TrimPrefix(s, "process.env.")
		return fmt.Sprintf("os.environ[%s]", pyString(varName))
	}
	return pyString(s)
}

func renderLocatorTS(target *ir.Target) string {
	if target == nil {
		return "page"
	}

	primary := renderSingleLocatorTS(target.Primary)

	if len(target.Fallbacks) > 0 && target.Fallbacks[0].Strategy != ir.LocatorCoords {
		fallback := renderSingleLocatorTS(target.Fallbacks[0])
		return fmt.Sprintf("%s\n      .or(%s)", primary, fallback)
	}

	return primary
}

func renderSingleLocatorTS(loc ir.Locator) string {
	switch loc.Strategy {
	case ir.LocatorTestID:
		if id := extractAttrValue(loc.Value, "data-testid"); id != "" {
			return fmt.Sprintf("page.getByTestId(%s)", jsString(id))
		}
		if id := extractAttrValue(loc.Value, "data-pw"); id != "" {
			return fmt.Sprintf("page.getByTestId(%s)", jsString(id))
		}
		return fmt.Sprintf("page.locator(%s)", jsString(loc.Value))

	case ir.LocatorRole:
		role, name := parseRoleLocator(loc.Value)
		if name != "" {
			return fmt.Sprintf("page.getByRole(%s, { name: %s })", jsString(role), jsString(name))
		}
		return fmt.Sprintf("page.getByRole(%s)", jsString(role))

	case ir.LocatorLabel:
		return fmt.Sprintf("page.getByLabel(%s)", jsString(loc.Value))

	case ir.LocatorText:
		return fmt.Sprintf("page.getByText(%s)", jsString(loc.Value))

	case ir.LocatorAltText:
		return fmt.Sprintf("page.getByAltText(%s)", jsString(loc.Value))

	case ir.LocatorTitle:
		return fmt.Sprintf("page.getByTitle(%s)", jsString(loc.Value))

	case ir.LocatorPlaceholder:
		return fmt.Sprintf("page.getByPlaceholder(%s)", jsString(loc.Value))

	case ir.LocatorCSS, ir.LocatorXPathRel, ir.LocatorXPathAbs:
		base := fmt.Sprintf("page.locator(%s)", jsString(loc.Value))
		if isLikelyMultiMatchSelector(loc.Value) {
			return base + ".first()"
		}
		return base

	case ir.LocatorCoords:
		return fmt.Sprintf("page.locator('*') /* FRAGILE: coordinate click at %s */", loc.Value)

	default:
		return fmt.Sprintf("page.locator(%s)", jsString(loc.Value))
	}
}

func renderLocatorPy(target *ir.Target) string {
	if target == nil {
		return "page"
	}
	return renderSingleLocatorPy(target.Primary)
}

func renderSingleLocatorPy(loc ir.Locator) string {
	switch loc.Strategy {
	case ir.LocatorTestID:
		if id := extractAttrValue(loc.Value, "data-testid"); id != "" {
			return fmt.Sprintf("page.get_by_test_id(%s)", pyString(id))
		}
		return fmt.Sprintf("page.locator(%s)", pyString(loc.Value))

	case ir.LocatorRole:
		role, name := parseRoleLocator(loc.Value)
		if name != "" {
			return fmt.Sprintf("page.get_by_role(%s, name=%s)", pyString(role), pyString(name))
		}
		return fmt.Sprintf("page.get_by_role(%s)", pyString(role))

	case ir.LocatorLabel:
		return fmt.Sprintf("page.get_by_label(%s)", pyString(loc.Value))

	case ir.LocatorText:
		return fmt.Sprintf("page.get_by_text(%s)", pyString(loc.Value))

	case ir.LocatorAltText:
		return fmt.Sprintf("page.get_by_alt_text(%s)", pyString(loc.Value))

	case ir.LocatorTitle:
		return fmt.Sprintf("page.get_by_title(%s)", pyString(loc.Value))

	case ir.LocatorPlaceholder:
		return fmt.Sprintf("page.get_by_placeholder(%s)", pyString(loc.Value))

	default:
		base := fmt.Sprintf("page.locator(%s)", pyString(loc.Value))
		if isLikelyMultiMatchSelector(loc.Value) {
			return base + ".first"
		}
		return base
	}
}

func renderFramePrefixTS(target *ir.Target) string {
	if target == nil || target.Context == nil {
		return "page"
	}
	if target.Context.Type == ir.ContextIframe && target.Context.FrameTarget != nil {
		frameSel := target.Context.FrameTarget.Primary.Value
		return fmt.Sprintf("page.frameLocator(%s)", jsString(frameSel))
	}
	return "page"
}

func parseRoleLocator(value string) (string, string) {
	value = strings.TrimPrefix(value, "role=")
	if idx := strings.Index(value, "[name="); idx != -1 {
		role := value[:idx]
		name := value[idx+6:]
		name = strings.TrimSuffix(name, "]")
		name = strings.Trim(name, `"'`)
		return role, name
	}
	return value, ""
}

func extractAttrValue(selector, attr string) string {
	prefix := fmt.Sprintf(`[%s=`, attr)
	idx := strings.Index(selector, prefix)
	if idx == -1 {
		return ""
	}
	rest := selector[idx+len(prefix):]
	if len(rest) == 0 {
		return ""
	}
	quote := rest[0]
	if quote == '"' || quote == '\'' {
		end := strings.Index(rest[1:], string(quote))
		if end >= 0 {
			return rest[1 : end+1]
		}
	}
	end := strings.IndexAny(rest, `]"' `)
	if end > 0 {
		return rest[:end]
	}
	return rest
}

func isLikelyMultiMatchSelector(sel string) bool {
	s := strings.ToLower(strings.TrimSpace(sel))
	return s == `input[type="radio"]` ||
		s == "input[type='radio']" ||
		s == `input[type="checkbox"]` ||
		s == "input[type='checkbox']" ||
		s == `[role="radio"]` ||
		s == `[role='radio']` ||
		s == `[role="checkbox"]` ||
		s == `[role='checkbox']`
}

func generateEnvExample(envVars map[string]string) string {
	if len(envVars) == 0 {
		return "# No credentials detected — no environment variables required.\n"
	}

	var sb strings.Builder
	sb.WriteString("# Generated by recast — fill in actual values before running tests\n")
	sb.WriteString("# DO NOT commit this file with real values\n\n")

	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sortStrings(keys)

	for _, k := range keys {
		comment := envVars[k]
		sb.WriteString(comment + "\n")
		sb.WriteString(k + "=\n\n")
	}

	return sb.String()
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func indent(n int) string {
	return strings.Repeat(" ", n)
}

func testFileName(name, ext string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		name = "workflow"
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range name {
		isAlphaNum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlphaNum {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	safe := strings.Trim(b.String(), "_")
	if safe == "" {
		safe = "workflow"
	}
	return fmt.Sprintf("test_%s.%s", safe, ext)
}
