package emitter

import (
	"fmt"
	"strings"

	ir "github.com/yagna-1/recast/recast-ir"
)

type PlaywrightTSEmitter struct{}

func (e *PlaywrightTSEmitter) TargetName() string    { return TargetPlaywrightTS }
func (e *PlaywrightTSEmitter) FileExtension() string { return "spec.ts" }

func (e *PlaywrightTSEmitter) Emit(trace *ir.Trace, envVars map[string]string) (*EmitResult, error) {
	var sb strings.Builder

	sb.WriteString("import { test, expect } from '@playwright/test';\n\n")
	sb.WriteString(fmt.Sprintf("test('%s', async ({ page }) => {\n", trace.Name))

	for _, step := range trace.Steps {
		lines := e.emitStep(step)
		for _, line := range lines {
			sb.WriteString(line)
		}
	}

	sb.WriteString("});\n")

	return &EmitResult{
		TestFile: sb.String(),
		AuxFiles: map[string]string{
			".env.example": generateEnvExample(envVars),
		},
	}, nil
}

func (e *PlaywrightTSEmitter) emitStep(step ir.Step) []string {
	var lines []string

	if step.Comment != "" {
		for _, line := range strings.Split(step.Comment, "\n") {
			lines = append(lines, fmt.Sprintf("  %s\n", line))
		}
	}

	if step.Target != nil && step.Target.Primary.Fragile {
		lines = append(lines, fmt.Sprintf("  // FRAGILE: selector %q — consider adding data-testid\n",
			step.Target.Primary.Value))
	}

	if step.Target != nil && step.Target.HumanDescription != "" {
		lines = append(lines, fmt.Sprintf("  // %s\n", step.Target.HumanDescription))
	}

	switch step.Type {
	case ir.StepNavigate:
		lines = append(lines, fmt.Sprintf("  await page.goto(%s);\n", jsString(step.Value)))
		lines = append(lines, e.emitWait(step.Wait, "page"))

	case ir.StepClick:
		loc := e.locatorExpr(step)
		clickSuffix := ".click()"
		if shouldForceClick(step) {
			clickSuffix = ".click({ force: true })"
		}
		if strings.Contains(loc, "\n") {
			lines = append(lines, fmt.Sprintf("  await %s\n    %s;\n", loc, clickSuffix))
		} else {
			lines = append(lines, fmt.Sprintf("  await %s%s;\n", loc, clickSuffix))
		}
		lines = append(lines, e.emitWait(step.Wait, "page"))

	case ir.StepFill:
		loc := e.locatorExpr(step)
		lines = append(lines, fmt.Sprintf("  await %s.fill(%s);\n", loc, jsEnvOrString(step.Value)))
		lines = append(lines, e.emitWait(step.Wait, "page"))

	case ir.StepSelect:
		loc := e.locatorExpr(step)
		lines = append(lines, fmt.Sprintf("  await %s.selectOption(%s);\n", loc, jsString(step.Value)))

	case ir.StepHover:
		loc := e.locatorExpr(step)
		lines = append(lines, fmt.Sprintf("  await %s.hover();\n", loc))
		lines = append(lines, "  // WARNING: hover may be vestigial from agent exploration\n")

	case ir.StepCheck:
		loc := e.locatorExpr(step)
		lines = append(lines, fmt.Sprintf("  await %s.check();\n", loc))

	case ir.StepUncheck:
		loc := e.locatorExpr(step)
		lines = append(lines, fmt.Sprintf("  await %s.uncheck();\n", loc))

	case ir.StepFocus:
		loc := e.locatorExpr(step)
		lines = append(lines, fmt.Sprintf("  await %s.focus();\n", loc))

	case ir.StepWaitForEl:
		if step.Target != nil {
			loc := e.locatorExpr(step)
			timeout := step.Wait.Timeout
			if timeout == 0 {
				timeout = 30000
			}
			lines = append(lines, fmt.Sprintf("  await %s.waitFor({ state: 'visible', timeout: %d });\n", loc, timeout))
		}

	case ir.StepWaitForURL:
		lines = append(lines, fmt.Sprintf("  await page.waitForURL(%s);\n", jsString(step.Value)))

	case ir.StepKeyboard:
		lines = append(lines, fmt.Sprintf("  await page.keyboard.press(%s);\n", jsString(step.Value)))

	case ir.StepScroll:
		if step.Value == "" || step.Value == "down" {
			lines = append(lines, "  await page.evaluate(() => window.scrollBy(0, 500));\n")
		} else {
			lines = append(lines, fmt.Sprintf("  await page.evaluate(() => window.scrollBy(0, %s));\n", step.Value))
		}

	case ir.StepScreenshot:
		path := step.Value
		if path == "" {
			path = "screenshot.png"
		}
		lines = append(lines, fmt.Sprintf("  await page.screenshot({ path: %s });\n", jsString(path)))

	case ir.StepFileUpload:
		loc := e.locatorExpr(step)
		lines = append(lines, "  // TODO: replace with actual file path\n")
		lines = append(lines, fmt.Sprintf("  await %s.setInputFiles(process.env.UPLOAD_FILE_PATH!);\n", loc))

	case ir.StepDialog:
		action := step.Value
		if action == "" {
			action = "accept"
		}
		lines = append(lines, fmt.Sprintf("  page.on('dialog', dialog => dialog.%s());\n", action))

	case ir.StepFrame:
		if step.Target != nil {
			lines = append(lines, fmt.Sprintf("  // Switching to iframe: %s\n", step.Target.Primary.Value))
		}

	case ir.StepAssert:
		lines = append(lines, e.emitAssertion(step)...)

	case ir.StepBranch:
		lines = append(lines, e.emitBranch(step)...)

	default:
		lines = append(lines, fmt.Sprintf("  // TODO: unsupported step type: %s\n", step.Type))
		if step.Target != nil {
			lines = append(lines, fmt.Sprintf("  // Original target: %s %s\n",
				step.Target.Primary.Strategy, step.Target.Primary.Value))
		}
	}

	lines = append(lines, "\n")
	return lines
}

func shouldForceClick(step ir.Step) bool {
	if step.Target == nil {
		return false
	}
	primary := strings.ToLower(strings.TrimSpace(step.Target.Primary.Value))
	for _, fb := range step.Target.Fallbacks {
		if isHiddenInputSelector(primary) || isHiddenInputSelector(strings.ToLower(strings.TrimSpace(fb.Value))) {
			return true
		}
	}
	return isHiddenInputSelector(primary)
}

func isHiddenInputSelector(sel string) bool {
	return sel == `input[type="radio"]` ||
		sel == "input[type='radio']" ||
		sel == `input[type="checkbox"]` ||
		sel == "input[type='checkbox']"
}

func (e *PlaywrightTSEmitter) locatorExpr(step ir.Step) string {
	if step.Target == nil {
		return "page"
	}

	if step.Target.Context != nil && step.Target.Context.Type == ir.ContextIframe {
		frameTarget := step.Target.Context.FrameTarget
		if frameTarget != nil {
			frameLocator := fmt.Sprintf("page.frameLocator(%s)", jsString(frameTarget.Primary.Value))
			innerLoc := renderSingleLocatorTS(step.Target.Primary)
			innerLoc = strings.Replace(innerLoc, "page.", frameLocator+".", 1)
			return innerLoc
		}
	}

	return renderLocatorTS(step.Target)
}

func (e *PlaywrightTSEmitter) emitWait(wait ir.WaitSpec, pageExpr string) string {
	timeout := wait.Timeout
	if timeout == 0 {
		timeout = 30000
	}

	switch wait.Type {
	case ir.WaitNetworkIdle:
		return fmt.Sprintf("  await page.waitForLoadState('networkidle');\n")
	case ir.WaitDOMContentLoad:
		return fmt.Sprintf("  await page.waitForLoadState('domcontentloaded');\n")
	case ir.WaitNavigation:
		return fmt.Sprintf("  await page.waitForNavigation({ timeout: %d });\n", timeout)
	case ir.WaitSelector:
		if wait.Value != "" {
			return fmt.Sprintf("  await page.waitForSelector(%s, { timeout: %d });\n",
				jsString(wait.Value), timeout)
		}
	case ir.WaitSelectorHidden:
		if wait.Value != "" {
			return fmt.Sprintf("  await page.waitForSelector(%s, { state: 'hidden', timeout: %d });\n",
				jsString(wait.Value), timeout)
		}
	case ir.WaitURL:
		if wait.Value != "" {
			return fmt.Sprintf("  await page.waitForURL(%s, { timeout: %d });\n",
				jsString(wait.Value), timeout)
		}
	case ir.WaitDelay:
		ms := timeout
		if ms <= 0 {
			ms = 500
		}
		return fmt.Sprintf("  await page.waitForTimeout(%d);\n", ms)
	}

	return ""
}

func (e *PlaywrightTSEmitter) emitAssertion(step ir.Step) []string {
	if step.Target == nil {
		return nil
	}

	desc := step.Target.HumanDescription
	if desc == "current URL" {
		url := step.Target.Primary.Value
		return []string{
			fmt.Sprintf("  await expect(page).toHaveURL(%s);\n", jsString(url)),
		}
	}

	if strings.Contains(desc, "hidden") {
		loc := renderSingleLocatorTS(step.Target.Primary)
		return []string{
			fmt.Sprintf("  await expect(%s).toBeHidden();\n", loc),
		}
	}

	loc := renderSingleLocatorTS(step.Target.Primary)
	return []string{
		fmt.Sprintf("  await expect(%s).toBeVisible();\n", loc),
	}
}

func (e *PlaywrightTSEmitter) emitBranch(step ir.Step) []string {
	if step.Branch == nil {
		return nil
	}
	b := step.Branch

	var lines []string
	lines = append(lines, fmt.Sprintf("  // TODO: conditional branch — %s\n", b.Condition))
	lines = append(lines, fmt.Sprintf("  // Only one branch was recorded. Implement the else-branch manually.\n"))
	lines = append(lines, fmt.Sprintf("  // if (await page.locator('[condition-selector]').isVisible()) {\n"))

	for _, s := range b.ThenSteps {
		inner := e.emitStep(s)
		for _, line := range inner {
			lines = append(lines, "  //   "+strings.TrimPrefix(line, "  "))
		}
	}

	lines = append(lines, "  // } else {\n")
	lines = append(lines, "  //   // TODO: implement else-branch\n")
	lines = append(lines, "  // }\n")

	return lines
}
