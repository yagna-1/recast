// Package recast provides the public Go library API for the recast compiler.
package recast

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yourorg/recast/internal/emitter"
	"github.com/yourorg/recast/internal/ingestion"
	"github.com/yourorg/recast/internal/optimizer"
	ir "github.com/yourorg/recast/recast-ir"
)

const (
	TargetPlaywrightTS = emitter.TargetPlaywrightTS
	TargetPlaywrightPy = emitter.TargetPlaywrightPy
	TargetIRJSON       = emitter.TargetIRJSON
)

type CompileOptions struct {
	InputPath string

	OutputPath string

	Target string

	Optimize bool

	HardenSelectors bool

	InjectAssertions bool

	ReplayExact bool

	SanitizeCredentials bool

	MaxFileSize int64

	Verbose bool
}

type CompileResult struct {
	OutputFiles []string

	Stats CompileStats

	Warnings []ir.Warning
}

type CompileStats struct {
	StepsCompiled        int
	StepsSkipped         int
	SelectorsHardened    int
	CredentialsSanitized int
	WarningCount         int
}

func Compile(opts CompileOptions) (*CompileResult, error) {
	if opts.OutputPath == "" {
		opts.OutputPath = "./recast-out/"
	}
	if opts.Target == "" {
		opts.Target = TargetPlaywrightTS
	}
	if opts.MaxFileSize == 0 {
		opts.MaxFileSize = 52_428_800
	}

	trace, _, err := ingestion.ParseFile(opts.InputPath)
	if err != nil {
		return nil, fmt.Errorf("compile: ingestion: %w", err)
	}

	valResult := ir.Validate(trace)
	if valResult.HasErrors() {
		return nil, fmt.Errorf("compile: IR validation failed: %s", valResult.Error())
	}

	var optResult *optimizer.Result
	if opts.ReplayExact {
		optOpts := optimizer.Options{
			Dedup:            false,
			HardenSelectors:  false,
			InferWaits:       false,
			DetectBranches:   false,
			InjectAssertions: false,
		}
		optResult = optimizer.Run(trace, optOpts)
		trace = optResult.Trace
	} else if opts.Optimize {
		optOpts := optimizer.Options{
			Dedup:            true,
			HardenSelectors:  opts.HardenSelectors,
			InferWaits:       true,
			DetectBranches:   true,
			InjectAssertions: opts.InjectAssertions,
		}
		optResult = optimizer.Run(trace, optOpts)
		trace = optResult.Trace
	} else {
		noop := optimizer.Options{}
		optResult = optimizer.Run(trace, noop)
		trace = optResult.Trace
	}

	if opts.Target == TargetIRJSON {
		return emitIRJSON(trace, opts, optResult)
	}

	em, err := emitter.Get(opts.Target)
	if err != nil {
		return nil, fmt.Errorf("compile: %w", err)
	}

	emitResult, err := em.Emit(trace, optResult.EnvVars)
	if err != nil {
		return nil, fmt.Errorf("compile: emit: %w", err)
	}

	if err := os.MkdirAll(opts.OutputPath, 0755); err != nil {
		return nil, fmt.Errorf("compile: create output dir: %w", err)
	}

	baseName := testFileName(trace.Name, em.FileExtension())
	testFilePath := filepath.Join(opts.OutputPath, baseName)
	if err := os.WriteFile(testFilePath, []byte(emitResult.TestFile), 0644); err != nil {
		return nil, fmt.Errorf("compile: write test file: %w", err)
	}

	outputFiles := []string{testFilePath}

	for auxName, auxContent := range emitResult.AuxFiles {
		auxPath := filepath.Join(opts.OutputPath, auxName)
		if err := os.WriteFile(auxPath, []byte(auxContent), 0644); err != nil {
			return nil, fmt.Errorf("compile: write aux file %s: %w", auxName, err)
		}
		outputFiles = append(outputFiles, auxPath)
	}

	stepsTotal := len(trace.Steps)
	stepsSkipped := 0
	allWarnings := append(valResult.Warnings, optResult.Warnings...)

	return &CompileResult{
		OutputFiles: outputFiles,
		Stats: CompileStats{
			StepsCompiled:        stepsTotal - stepsSkipped,
			StepsSkipped:         stepsSkipped,
			SelectorsHardened:    optResult.SelectorsHardened,
			CredentialsSanitized: optResult.CredentialsSanitized,
			WarningCount:         len(allWarnings),
		},
		Warnings: allWarnings,
	}, nil
}

func emitIRJSON(trace *ir.Trace, opts CompileOptions, optResult *optimizer.Result) (*CompileResult, error) {
	data, err := ir.Marshal(trace)
	if err != nil {
		return nil, fmt.Errorf("compile: marshal IR: %w", err)
	}

	if err := os.MkdirAll(opts.OutputPath, 0755); err != nil {
		return nil, fmt.Errorf("compile: create output dir: %w", err)
	}

	baseName := strings.ReplaceAll(trace.Name, " ", "_") + ".ir.json"
	outPath := filepath.Join(opts.OutputPath, baseName)
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		return nil, fmt.Errorf("compile: write IR JSON: %w", err)
	}

	return &CompileResult{
		OutputFiles: []string{outPath},
		Stats: CompileStats{
			StepsCompiled:        len(trace.Steps),
			CredentialsSanitized: optResult.CredentialsSanitized,
		},
		Warnings: optResult.Warnings,
	}, nil
}

func testFileName(name, ext string) string {
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ToLower(name)
	return fmt.Sprintf("test_%s.%s", name, ext)
}
