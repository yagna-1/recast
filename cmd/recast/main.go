package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yagna-1/recast/internal/emitter"
	"github.com/yagna-1/recast/internal/ingestion"
	"github.com/yagna-1/recast/internal/optimizer"
	ir "github.com/yagna-1/recast/recast-ir"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		var cliErr *exitError
		if errors.As(err, &cliErr) {
			os.Exit(cliErr.code)
		}
		os.Exit(2)
	}
}

type exitError struct {
	code int
}

func (e *exitError) Error() string {
	return fmt.Sprintf("exit %d", e.code)
}

var rootCmd = &cobra.Command{
	Use:           "recast",
	Short:         "recast — AI browser workflow → Playwright compiler",
	SilenceUsage:  true,
	SilenceErrors: false,
	Long: `recast compiles AI browser agent recordings into clean, static Playwright test code.

Supported input formats: workflow-use JSON, HAR, CDP event log, MCP tool call log, AstraGraph audit trail
Supported output targets: playwright-ts, playwright-py, ir-json

Example:
  recast compile workflow.json
  recast compile workflow.json -t playwright-py -o ./tests/
  recast validate workflow.json
  recast formats`,
}

var (
	compileOutput      string
	compileTarget      string
	compileFrom        string
	compileNoOptimize  bool
	compileNoHarden    bool
	compileAssertions  bool
	compileReplayExact bool
	compileStrict      bool
	compileVerbose     bool
)

var compileCmd = &cobra.Command{
	Use:   "compile <input-file>",
	Short: "Compile an agent workflow to Playwright test code",
	Args:  cobra.ExactArgs(1),
	RunE:  runCompile,
}

func init() {
	compileCmd.Flags().StringVarP(&compileOutput, "output", "o", "./recast-out/", "Output directory")
	compileCmd.Flags().StringVarP(&compileTarget, "target", "t", "playwright-ts",
		"Output target: playwright-ts, playwright-py, ir-json")
	compileCmd.Flags().StringVar(&compileFrom, "from", "", "Force input format: workflow-use, har, cdp, mcp, astragraph-audit")
	compileCmd.Flags().BoolVar(&compileNoOptimize, "no-optimize", false, "Skip all optimizer passes")
	compileCmd.Flags().BoolVar(&compileNoHarden, "no-harden", false, "Skip selector hardening")
	compileCmd.Flags().BoolVar(&compileAssertions, "inject-assertions", false, "Inject post-action assertions")
	compileCmd.Flags().BoolVar(&compileReplayExact, "replay-exact", false, "Preserve recorded action sequence (disable behavior-changing heuristic passes)")
	compileCmd.Flags().BoolVar(&compileStrict, "strict", false, "Exit 2 on any warning (not just errors)")
	compileCmd.Flags().BoolVarP(&compileVerbose, "verbose", "v", false, "Print detailed compilation log")
	rootCmd.AddCommand(compileCmd)
}

func runCompile(cmd *cobra.Command, args []string) error {
	inputPath := args[0]

	logInfo("detected format: detecting...")

	var trace *ir.Trace
	var formatName string
	var err error
	if compileFrom != "" {
		trace, formatName, err = ingestion.ParseFileWithFormat(inputPath, compileFrom)
	} else {
		trace, formatName, err = ingestion.ParseFile(inputPath)
	}
	if err != nil {
		logError("", "ingestion", err.Error())
		return &exitError{code: 3}
	}
	logInfo(fmt.Sprintf("detected format: %s", formatName))
	logInfo(fmt.Sprintf("parsed %d steps", len(trace.Steps)))

	valResult := ir.Validate(trace)
	for _, ve := range valResult.Errors {
		logError(ve.StepID, "validate", ve.Message)
	}
	for _, w := range valResult.Warnings {
		logWarn(w.StepID, w.Pass, w.Message)
	}
	if valResult.HasErrors() {
		logError("", "", "compilation failed during IR validation")
		return &exitError{code: 2}
	}

	var optResult *optimizer.Result

	if compileReplayExact {
		if compileAssertions {
			logWarn("", "compile", "--inject-assertions ignored in --replay-exact mode")
		}
		optOpts := optimizer.Options{
			Dedup:            false,
			HardenSelectors:  false,
			InferWaits:       false,
			DetectBranches:   false,
			InjectAssertions: false,
		}
		optResult = optimizer.Run(trace, optOpts)
		logInfo("replay-exact mode: heuristic behavior-changing passes disabled")
	} else if compileTarget == "ir-json" || compileNoOptimize {
		optOpts := optimizer.Options{}
		optResult = optimizer.Run(trace, optOpts)
	} else {
		optOpts := optimizer.Options{
			Dedup:            !compileNoOptimize,
			HardenSelectors:  !compileNoHarden && !compileNoOptimize,
			InferWaits:       !compileNoOptimize,
			DetectBranches:   !compileNoOptimize,
			InjectAssertions: compileAssertions,
		}
		optResult = optimizer.Run(trace, optOpts)
	}

	trace = optResult.Trace

	for _, w := range optResult.Warnings {
		logWarn(w.StepID, w.Pass, w.Message)
	}

	if optResult.CredentialsSanitized > 0 {
		logInfo(fmt.Sprintf("[sanitize] %d credential(s) replaced with environment variables",
			optResult.CredentialsSanitized))
	}

	if compileTarget == "ir-json" {
		return emitIRJSON(trace, inputPath, optResult)
	}

	em, err := emitter.Get(compileTarget)
	if err != nil {
		logError("", "emit", err.Error())
		return &exitError{code: 4}
	}

	emitResult, err := em.Emit(trace, optResult.EnvVars)
	if err != nil {
		logError("", "emit", err.Error())
		return &exitError{code: 2}
	}

	if err := os.MkdirAll(compileOutput, 0755); err != nil {
		logError("", "emit", fmt.Sprintf("cannot create output dir: %v", err))
		return &exitError{code: 2}
	}

	baseName := makeTestFileName(trace.Name, em.FileExtension())
	testFilePath := filepath.Join(compileOutput, baseName)
	if err := os.WriteFile(testFilePath, []byte(emitResult.TestFile), 0644); err != nil {
		logError("", "emit", fmt.Sprintf("cannot write output file: %v", err))
		return &exitError{code: 2}
	}
	logInfo(fmt.Sprintf("[emit] wrote %s to %s", compileTarget, testFilePath))

	for auxName, auxContent := range emitResult.AuxFiles {
		safeAuxName := sanitizeOutputFileName(auxName)
		if safeAuxName == "" {
			logWarn("", "emit", fmt.Sprintf("skipping unsafe auxiliary filename: %q", auxName))
			continue
		}
		auxPath := filepath.Join(compileOutput, safeAuxName)
		if err := os.WriteFile(auxPath, []byte(auxContent), 0644); err != nil {
			logWarn("", "emit", fmt.Sprintf("cannot write auxiliary file %s: %v", safeAuxName, err))
			continue
		}
		logInfo(fmt.Sprintf("[emit] wrote %s to %s", safeAuxName, auxPath))
	}

	totalWarnings := len(valResult.Warnings) + len(optResult.Warnings)
	fmt.Fprintf(os.Stderr, "\nrecast: SUMMARY\n")
	fmt.Fprintf(os.Stderr, "  steps compiled:     %d\n", len(trace.Steps))
	fmt.Fprintf(os.Stderr, "  selectors hardened: %d\n", optResult.SelectorsHardened)
	fmt.Fprintf(os.Stderr, "  credentials sanitized: %d\n", optResult.CredentialsSanitized)
	fmt.Fprintf(os.Stderr, "  warnings:           %d\n", totalWarnings)

	if compileStrict && totalWarnings > 0 {
		fmt.Fprintf(os.Stderr, "\nExit code: 2  (--strict: warnings treated as errors)\n")
		return &exitError{code: 2}
	}
	if totalWarnings > 0 {
		fmt.Fprintf(os.Stderr, "\nExit code: 1  (partial success — review warnings before running)\n")
		return &exitError{code: 1}
	}

	return nil
}

func emitIRJSON(trace *ir.Trace, inputPath string, optResult *optimizer.Result) error {
	data, err := ir.Marshal(trace)
	if err != nil {
		logError("", "emit", fmt.Sprintf("marshal IR: %v", err))
		return &exitError{code: 2}
	}

	if err := os.MkdirAll(compileOutput, 0755); err != nil {
		logError("", "emit", fmt.Sprintf("cannot create output dir: %v", err))
		return &exitError{code: 2}
	}

	baseName := sanitizeNameSegment(trace.Name) + ".ir.json"
	outPath := filepath.Join(compileOutput, baseName)
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		logError("", "emit", fmt.Sprintf("cannot write IR JSON: %v", err))
		return &exitError{code: 2}
	}
	logInfo(fmt.Sprintf("[emit] wrote ir-json to %s", outPath))
	return nil
}

var validateCmd = &cobra.Command{
	Use:   "validate <input-file>",
	Short: "Validate an input file without compiling",
	Long:  "Validates the input file structure, reports format detection and any issues.",
	Args:  cobra.ExactArgs(1),
	RunE:  runValidate,
}

func init() {
	rootCmd.AddCommand(validateCmd)
}

func runValidate(cmd *cobra.Command, args []string) error {
	inputPath := args[0]

	trace, formatName, err := ingestion.ParseFile(inputPath)
	if err != nil {
		logError("", "", err.Error())
		return &exitError{code: 3}
	}

	fmt.Fprintf(os.Stderr, "recast validate: %s\n", inputPath)
	fmt.Fprintf(os.Stderr, "  format:     %s\n", formatName)
	fmt.Fprintf(os.Stderr, "  steps:      %d\n", len(trace.Steps))

	valResult := ir.Validate(trace)

	if len(valResult.Errors) > 0 {
		fmt.Fprintf(os.Stderr, "  errors:     %d\n", len(valResult.Errors))
		for _, e := range valResult.Errors {
			logError(e.StepID, "validate", e.Message)
		}
	}

	if len(valResult.Warnings) > 0 {
		fmt.Fprintf(os.Stderr, "  warnings:   %d\n", len(valResult.Warnings))
		for _, w := range valResult.Warnings {
			logWarn(w.StepID, w.Pass, w.Message)
		}
	}

	noop := optimizer.Options{}
	optResult := optimizer.Run(trace, noop)
	if optResult.CredentialsSanitized > 0 {
		fmt.Fprintf(os.Stderr, "  credentials detected: %d (will be sanitized on compile)\n",
			optResult.CredentialsSanitized)
	}

	if valResult.HasErrors() {
		fmt.Fprintf(os.Stderr, "\nrecast validate: FAILED\n")
		return &exitError{code: 2}
	}
	fmt.Fprintf(os.Stderr, "\nrecast validate: OK\n")
	return nil
}

var formatsCmd = &cobra.Command{
	Use:   "formats",
	Short: "List all supported input formats",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Supported input formats:")
		fmt.Println()
		formats := ingestion.AllFormats()
		for _, f := range formats {
			fmt.Printf("  %-30s\n", f.Name)
		}
		fmt.Println()
		fmt.Println("Supported output targets:")
		fmt.Println()
		for _, t := range []string{"playwright-ts", "playwright-py", "ir-json"} {
			fmt.Printf("  %s\n", t)
		}
	},
}

func init() {
	rootCmd.AddCommand(formatsCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("recast %s (%s, %s)\n", version, commit, date)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

func logInfo(msg string) {
	fmt.Fprintf(os.Stderr, "recast: INFO  %s\n", msg)
}

func logWarn(stepID, pass, msg string) {
	if stepID != "" && pass != "" {
		fmt.Fprintf(os.Stderr, "recast: WARN  [%s] %s: %s\n", stepID, pass, msg)
	} else if stepID != "" {
		fmt.Fprintf(os.Stderr, "recast: WARN  [%s] %s\n", stepID, msg)
	} else {
		fmt.Fprintf(os.Stderr, "recast: WARN  %s\n", msg)
	}
}

func logError(stepID, pass, msg string) {
	if stepID != "" && pass != "" {
		fmt.Fprintf(os.Stderr, "recast: ERROR [%s] %s: %s\n", stepID, pass, msg)
	} else if stepID != "" {
		fmt.Fprintf(os.Stderr, "recast: ERROR [%s] %s\n", stepID, msg)
	} else {
		fmt.Fprintf(os.Stderr, "recast: ERROR %s\n", msg)
	}
}

func makeTestFileName(name, ext string) string {
	name = sanitizeNameSegment(name)
	return fmt.Sprintf("test_%s.%s", name, ext)
}

func sanitizeOutputFileName(name string) string {
	clean := filepath.Clean(name)
	if clean == "." || clean == "" {
		return ""
	}
	if filepath.IsAbs(clean) {
		return ""
	}
	if strings.HasPrefix(clean, "..") {
		return ""
	}
	if strings.ContainsAny(clean, `/\`) {
		return ""
	}
	return clean
}

func sanitizeNameSegment(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return "workflow"
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
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "workflow"
	}
	return out
}
