package emitter

import (
	"fmt"
	"strconv"
	"strings"

	ir "github.com/yagna-1/recast/recast-ir"
)

type PlaywrightPyEmitter struct{}

func (e *PlaywrightPyEmitter) TargetName() string    { return TargetPlaywrightPy }
func (e *PlaywrightPyEmitter) FileExtension() string { return "py" }

func (e *PlaywrightPyEmitter) Emit(trace *ir.Trace, envVars map[string]string) (*EmitResult, error) {
	var sb strings.Builder

	sb.WriteString("import os\n")
	sb.WriteString("from playwright.sync_api import sync_playwright, expect\n\n")
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("def test_%s(page):\n", trace.Name))

	for _, step := range trace.Steps {
		lines := e.emitStep(step)
		for _, line := range lines {
			sb.WriteString(line)
		}
	}

	if len(trace.Steps) == 0 {
		sb.WriteString("    pass\n")
	}

	return &EmitResult{
		TestFile: sb.String(),
		AuxFiles: map[string]string{
			".env.example": generateEnvExample(envVars),
		},
	}, nil
}

func (e *PlaywrightPyEmitter) emitStep(step ir.Step) []string {
	var lines []string

	if step.Comment != "" {
		for _, line := range strings.Split(step.Comment, "\n") {
			comment := strings.TrimPrefix(line, "//")
			comment = strings.TrimSpace(comment)
			lines = append(lines, fmt.Sprintf("    # %s\n", comment))
		}
	}

	if step.Target != nil && step.Target.Primary.Fragile {
		lines = append(lines, fmt.Sprintf("    # FRAGILE: selector %q — consider adding data-testid\n",
			step.Target.Primary.Value))
	}

	if step.Target != nil && step.Target.HumanDescription != "" {
		lines = append(lines, fmt.Sprintf("    # %s\n", step.Target.HumanDescription))
	}

	switch step.Type {
	case ir.StepNavigate:
		lines = append(lines, fmt.Sprintf("    page.goto(%s)\n", pyString(step.Value)))
		lines = append(lines, e.emitWait(step.Wait))

	case ir.StepClick:
		loc := e.locatorExpr(step)
		if strings.Contains(loc, ".first") {
			lines = append(lines, "    # WARNING: auto-selected first matching element for a likely multi-match selector\n")
		}
		lines = append(lines, fmt.Sprintf("    %s.click()\n", loc))
		lines = append(lines, e.emitWait(step.Wait))

	case ir.StepFill:
		loc := e.locatorExpr(step)
		lines = append(lines, fmt.Sprintf("    %s.fill(%s)\n", loc, pyEnvOrString(step.Value)))
		lines = append(lines, e.emitWait(step.Wait))

	case ir.StepSelect:
		loc := e.locatorExpr(step)
		lines = append(lines, fmt.Sprintf("    %s.select_option(%s)\n", loc, pyString(step.Value)))

	case ir.StepHover:
		loc := e.locatorExpr(step)
		lines = append(lines, fmt.Sprintf("    %s.hover()\n", loc))
		lines = append(lines, "    # WARNING: hover may be vestigial from agent exploration\n")

	case ir.StepCheck:
		loc := e.locatorExpr(step)
		lines = append(lines, fmt.Sprintf("    %s.check()\n", loc))

	case ir.StepUncheck:
		loc := e.locatorExpr(step)
		lines = append(lines, fmt.Sprintf("    %s.uncheck()\n", loc))

	case ir.StepWaitForEl:
		if step.Target != nil {
			loc := e.locatorExpr(step)
			timeout := step.Wait.Timeout
			if timeout == 0 {
				timeout = 30000
			}
			lines = append(lines, fmt.Sprintf("    %s.wait_for(state='visible', timeout=%d)\n", loc, timeout))
		}

	case ir.StepWaitForURL:
		lines = append(lines, fmt.Sprintf("    page.wait_for_url(%s)\n", pyString(step.Value)))

	case ir.StepKeyboard:
		lines = append(lines, fmt.Sprintf("    page.keyboard.press(%s)\n", pyString(step.Value)))

	case ir.StepScroll:
		lines = append(lines, "    page.evaluate('window.scrollBy(0, 500)')\n")

	case ir.StepScreenshot:
		path := step.Value
		if path == "" {
			path = "screenshot.png"
		}
		lines = append(lines, fmt.Sprintf("    page.screenshot(path=%s)\n", pyString(path)))

	case ir.StepFileUpload:
		loc := e.locatorExpr(step)
		lines = append(lines, "    # TODO: replace with actual file path\n")
		lines = append(lines, fmt.Sprintf("    %s.set_input_files(os.environ['UPLOAD_FILE_PATH'])\n", loc))

	case ir.StepDialog:
		action := step.Value
		if action == "" {
			action = "accept"
		}
		lines = append(lines, fmt.Sprintf("    page.on('dialog', lambda dialog: dialog.%s())\n", action))

	case ir.StepAssert:
		lines = append(lines, e.emitAssertion(step)...)

	case ir.StepBranch:
		lines = append(lines, e.emitBranch(step)...)

	default:
		lines = append(lines, fmt.Sprintf("    # TODO: unsupported step type: %s\n", step.Type))
	}

	lines = append(lines, "\n")
	return lines
}

func (e *PlaywrightPyEmitter) locatorExpr(step ir.Step) string {
	if step.Target == nil {
		return "page"
	}

	if step.Target.Context != nil && step.Target.Context.Type == ir.ContextIframe {
		if step.Target.Context.FrameTarget != nil {
			frameSel := step.Target.Context.FrameTarget.Primary.Value
			inner := renderSingleLocatorPy(step.Target.Primary)
			inner = strings.Replace(inner, "page.", fmt.Sprintf("page.frame_locator(%s).", pyString(frameSel)), 1)
			return inner
		}
	}

	return renderLocatorPy(step.Target)
}

func (e *PlaywrightPyEmitter) emitWait(wait ir.WaitSpec) string {
	timeout := wait.Timeout
	if timeout == 0 {
		timeout = 30000
	}

	switch wait.Type {
	case ir.WaitNetworkIdle:
		return "    page.wait_for_load_state('networkidle')\n"
	case ir.WaitDOMContentLoad:
		return "    page.wait_for_load_state('domcontentloaded')\n"
	case ir.WaitNavigation:
		return fmt.Sprintf("    page.wait_for_url('**/*', timeout=%d)\n", timeout)
	case ir.WaitSelector:
		if wait.Value != "" {
			return fmt.Sprintf("    page.wait_for_selector(%s, timeout=%d)\n",
				pyString(wait.Value), timeout)
		}
	case ir.WaitSelectorHidden:
		if wait.Value != "" {
			return fmt.Sprintf("    page.wait_for_selector(%s, state='hidden', timeout=%d)\n",
				pyString(wait.Value), timeout)
		}
	case ir.WaitDelay:
		ms := timeout
		if ms <= 0 {
			ms = 500
		}
		return fmt.Sprintf("    page.wait_for_timeout(%d)\n", ms)
	}
	return ""
}

func (e *PlaywrightPyEmitter) emitAssertion(step ir.Step) []string {
	if step.Target == nil {
		return nil
	}
	desc := step.Target.HumanDescription
	if desc == "current URL" {
		url := step.Target.Primary.Value
		return []string{fmt.Sprintf("    expect(page).to_have_url(%s)\n", pyString(url))}
	}
	if strings.Contains(desc, "hidden") {
		loc := renderSingleLocatorPy(step.Target.Primary)
		return []string{fmt.Sprintf("    expect(%s).to_be_hidden()\n", loc)}
	}
	loc := renderSingleLocatorPy(step.Target.Primary)
	return []string{fmt.Sprintf("    expect(%s).to_be_visible()\n", loc)}
}

func (e *PlaywrightPyEmitter) emitBranch(step ir.Step) []string {
	if step.Branch == nil {
		return nil
	}
	b := step.Branch

	var lines []string
	lines = append(lines, fmt.Sprintf("    # TODO: conditional branch — %s\n", b.Condition))
	lines = append(lines, "    # Only one branch was recorded. Implement the else-branch manually.\n")
	lines = append(lines, "    # if page.locator('[condition-selector]').is_visible():\n")

	for _, s := range b.ThenSteps {
		inner := e.emitStep(s)
		for _, line := range inner {
			trimmed := strings.TrimPrefix(line, "    ")
			lines = append(lines, "    #     "+trimmed)
		}
	}

	lines = append(lines, "    # else:\n")
	lines = append(lines, "    #     # TODO: implement else-branch\n")
	lines = append(lines, "    pass  # branch not yet implemented\n")

	return lines
}

func pyString(s string) string {
	return strconv.Quote(s)
}
