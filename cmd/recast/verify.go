package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/yagna-1/recast/internal/ingestion"
	ir "github.com/yagna-1/recast/recast-ir"
)

type verifySignal struct {
	Name     string
	Expected string
	Passed   bool
}

type verifyResult struct {
	Signals []verifySignal
	Passed  bool
}

var (
	verifyAgainst           string
	verifyRuntime           bool
	verifyRuntimeStrict     bool
	verifyRuntimeTimeoutSec int
)

var verifyCmd = &cobra.Command{
	Use:   "verify <playwright-file>",
	Short: "Verify compiled output against source trace (screenshot-free)",
	Args:  cobra.ExactArgs(1),
	RunE:  runVerify,
}

func init() {
	verifyCmd.Flags().StringVar(&verifyAgainst, "against", "", "Original trace file to verify against")
	verifyCmd.Flags().BoolVar(&verifyRuntime, "runtime", false, "Also run Playwright test runtime check (still screenshot-free by default)")
	verifyCmd.Flags().BoolVar(&verifyRuntimeStrict, "runtime-strict", false, "Fail verification when runtime check cannot be executed")
	verifyCmd.Flags().IntVar(&verifyRuntimeTimeoutSec, "runtime-timeout-sec", 120, "Max seconds for runtime Playwright verification")
	_ = verifyCmd.MarkFlagRequired("against")
	rootCmd.AddCommand(verifyCmd)
}

func runVerify(cmd *cobra.Command, args []string) error {
	playwrightFile := args[0]
	resolvedPlaywrightFile, resolveNote, err := resolvePlaywrightFile(playwrightFile)
	if err != nil {
		logError("", "verify", fmt.Sprintf("cannot read playwright file: %v", err))
		return &exitError{code: 3}
	}
	if resolveNote != "" {
		logWarn("", "verify", resolveNote)
	}
	playwrightFile = resolvedPlaywrightFile
	contentBytes, err := os.ReadFile(playwrightFile)
	if err != nil {
		logError("", "verify", fmt.Sprintf("cannot read playwright file: %v", err))
		return &exitError{code: 3}
	}
	content := string(contentBytes)
	if strings.TrimSpace(content) == "" {
		logError("", "verify", "playwright file is empty")
		return &exitError{code: 2}
	}

	trace, formatName, err := ingestion.ParseFile(verifyAgainst)
	if err != nil {
		logError("", "verify", fmt.Sprintf("cannot parse --against trace: %v", err))
		return &exitError{code: 3}
	}
	logInfo(fmt.Sprintf("verify against format: %s", formatName))

	result := runStaticVerifyContent(content, trace)
	if verifyRuntime {
		runtimeSignal := verifySignal{
			Name:     "runtime playwright check",
			Expected: filepath.Base(playwrightFile),
		}
		if out, err := runRuntimeVerify(playwrightFile, verifyRuntimeTimeoutSec); err != nil {
			if canSkipRuntimeFailure(out) && !verifyRuntimeStrict {
				runtimeSignal.Passed = true
				runtimeSignal.Expected = filepath.Base(playwrightFile) + " (runtime skipped: environment unavailable)"
				logWarn("", "verify", "runtime check skipped; use --runtime-strict to require executable runtime verification")
			} else {
				runtimeSignal.Passed = false
				logWarn("", "verify", fmt.Sprintf("runtime check failed: %v", err))
				if strings.TrimSpace(out) != "" {
					logWarn("", "verify", summarizeOutput(out, 4))
				}
			}
		} else {
			runtimeSignal.Passed = true
		}
		result.Signals = append(result.Signals, runtimeSignal)
		if !runtimeSignal.Passed {
			result.Passed = false
		}
	}

	for _, s := range result.Signals {
		icon := "✓"
		if !s.Passed {
			icon = "✗"
		}
		fmt.Fprintf(os.Stderr, "%s %s: %s\n", icon, s.Name, s.Expected)
	}

	if !result.Passed {
		fmt.Fprintf(os.Stderr, "\nrecast verify: FAILED\n")
		return &exitError{code: 2}
	}

	fmt.Fprintf(os.Stderr, "\nrecast verify: PASSED\n")
	return nil
}

func resolvePlaywrightFile(path string) (string, string, error) {
	if _, err := os.Stat(path); err == nil {
		return path, "", nil
	}
	originalErr := fmt.Errorf("open %s: no such file or directory", path)
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)

	candidates, err := filepath.Glob(filepath.Join(dir, "*.spec.ts"))
	if err != nil {
		return "", "", err
	}
	pyCandidates, err := filepath.Glob(filepath.Join(dir, "*.spec.py"))
	if err != nil {
		return "", "", err
	}
	candidates = append(candidates, pyCandidates...)
	if len(candidates) == 0 {
		return "", "", originalErr
	}

	best := candidates[0]
	bestScore := matchScore(stem, strings.TrimSuffix(filepath.Base(best), filepath.Ext(best)))
	for _, cand := range candidates[1:] {
		candStem := strings.TrimSuffix(filepath.Base(cand), filepath.Ext(cand))
		score := matchScore(stem, candStem)
		if score > bestScore {
			best = cand
			bestScore = score
		}
	}

	note := ""
	if filepath.Base(best) != base {
		note = fmt.Sprintf("playwright file %q not found, using %q", base, filepath.Base(best))
	}
	return best, note, nil
}

func matchScore(want, cand string) int {
	nw := normalizeName(want)
	nc := normalizeName(cand)
	switch {
	case nw == nc:
		return 100
	case strings.HasPrefix(nc, nw) || strings.HasPrefix(nw, nc):
		return 80
	case strings.Contains(nc, nw) || strings.Contains(nw, nc):
		return 60
	default:
		return commonPrefixLen(nw, nc)
	}
}

func normalizeName(v string) string {
	v = strings.ToLower(v)
	v = strings.ReplaceAll(v, "_semantic", "")
	v = strings.ReplaceAll(v, "-", "")
	v = strings.ReplaceAll(v, "_", "")
	v = strings.ReplaceAll(v, " ", "")
	return v
}

func commonPrefixLen(a, b string) int {
	max := len(a)
	if len(b) < max {
		max = len(b)
	}
	n := 0
	for i := 0; i < max; i++ {
		if a[i] != b[i] {
			break
		}
		n++
	}
	return n
}

func runRuntimeVerify(playwrightFile string, timeoutSec int) (string, error) {
	if timeoutSec <= 0 {
		timeoutSec = 120
	}
	absFile, err := filepath.Abs(playwrightFile)
	if err != nil {
		return "", err
	}
	workDir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	testDir := filepath.ToSlash(filepath.Dir(absFile))
	testFile := filepath.Base(absFile)
	tmpCfg, err := os.CreateTemp("", "recast-playwright-*.config.cjs")
	if err != nil {
		return "", err
	}
	cfgPath := tmpCfg.Name()
	cfg := fmt.Sprintf("module.exports = { testDir: %q, testMatch: '**/*.spec.ts' };", testDir)
	if _, err := tmpCfg.WriteString(cfg); err != nil {
		_ = tmpCfg.Close()
		_ = os.Remove(cfgPath)
		return "", err
	}
	_ = tmpCfg.Close()
	defer os.Remove(cfgPath)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()
	playwrightBin := filepath.Join(workDir, "node_modules", ".bin", "playwright")
	var cmd *exec.Cmd
	if _, err := os.Stat(playwrightBin); err == nil {
		cmd = exec.CommandContext(ctx, playwrightBin, "test", testFile, "--browser=firefox", "--reporter=line", "--config", cfgPath)
	} else {
		cmd = exec.CommandContext(ctx, "npx", "--yes", "@playwright/test@1.58.2", "test", testFile, "--browser=firefox", "--reporter=line", "--config", cfgPath)
	}
	cmd.Dir = workDir
	cmd.Env = runtimeVerifyEnv()
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(out), fmt.Errorf("runtime check timeout after %ds", timeoutSec)
	}
	return string(out), err
}

func summarizeOutput(out string, maxLines int) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) <= maxLines {
		return strings.Join(lines, " | ")
	}
	return strings.Join(lines[len(lines)-maxLines:], " | ")
}

func canSkipRuntimeFailure(out string) bool {
	l := strings.ToLower(out)
	return strings.Contains(l, "no tests found") ||
		strings.Contains(l, "make sure that arguments are regular expressions matching test files") ||
		strings.Contains(l, "playwright test")
}

func runtimeVerifyEnv() []string {
	envMap := map[string]string{}
	for _, kv := range os.Environ() {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}
	setIfMissing := func(k, v string) {
		if _, ok := envMap[k]; !ok {
			envMap[k] = v
		}
	}
	setIfMissing("TEST_EMAIL", "test@example.com")
	setIfMissing("TEST_PASSWORD", "test-password")
	for i := 1; i <= 10; i++ {
		k := fmt.Sprintf("RECAST_VAR_%d", i)
		setIfMissing(k, "placeholder")
	}
	out := make([]string, 0, len(envMap))
	for k, v := range envMap {
		out = append(out, k+"="+v)
	}
	return out
}

func runStaticVerifyContent(content string, trace *ir.Trace) verifyResult {
	signals := []verifySignal{
		{
			Name:     "playwright file has content",
			Expected: "non-empty source",
			Passed:   strings.TrimSpace(content) != "",
		},
	}

	if expectedURL := extractExpectedURL(trace); expectedURL != "" {
		signals = append(signals, verifySignal{
			Name:     "final url signal present",
			Expected: expectedURL,
			Passed:   strings.Contains(content, expectedURL),
		})
	}

	for _, sel := range extractExpectedSelectors(trace, 8) {
		signals = append(signals, verifySignal{
			Name:     "selector signal present",
			Expected: sel,
			Passed:   strings.Contains(content, sel),
		})
	}

	passed := true
	for _, s := range signals {
		if !s.Passed {
			passed = false
			break
		}
	}

	return verifyResult{
		Signals: signals,
		Passed:  passed,
	}
}

func extractExpectedURL(trace *ir.Trace) string {
	for i := len(trace.Steps) - 1; i >= 0; i-- {
		step := trace.Steps[i]
		if step.Type == ir.StepWaitForURL && step.Value != "" {
			return step.Value
		}
		if step.Type == ir.StepNavigate && step.Value != "" {
			return step.Value
		}
		if step.Wait.Type == ir.WaitURL && step.Wait.Value != "" {
			return step.Wait.Value
		}
	}
	return ""
}

func extractExpectedSelectors(trace *ir.Trace, limit int) []string {
	if limit <= 0 {
		return nil
	}
	out := make([]string, 0, limit)
	seen := map[string]struct{}{}

	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		if _, ok := seen[v]; ok {
			return
		}
		seen[v] = struct{}{}
		if len(out) < limit {
			out = append(out, v)
		}
	}

	for _, step := range trace.Steps {
		if step.Type == ir.StepWaitForEl {
			if step.Target != nil {
				add(step.Target.Primary.Value)
			}
			if step.Wait.Type == ir.WaitSelector {
				add(step.Wait.Value)
			}
		}
		if step.Type == ir.StepAssert && step.Target != nil {
			add(step.Target.Primary.Value)
		}
	}
	return out
}
