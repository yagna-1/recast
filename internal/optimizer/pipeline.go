// Package optimizer contains the six compiler optimization passes.
package optimizer

import (
	ir "github.com/yourorg/recast/recast-ir"
)

type Options struct {
	Dedup            bool // Pass 1: remove duplicate consecutive actions
	HardenSelectors  bool // Pass 2: upgrade fragile selectors
	InferWaits       bool // Pass 3: inject explicit waits
	DetectBranches   bool // Pass 5: detect missing conditional branches
	InjectAssertions bool // Pass 6: inject post-action assertions (opt-in)
}

func DefaultOptions() Options {
	return Options{
		Dedup:            true,
		HardenSelectors:  true,
		InferWaits:       true,
		DetectBranches:   true,
		InjectAssertions: false,
	}
}

type Result struct {
	Trace                *ir.Trace
	Warnings             []ir.Warning
	CredentialsSanitized int
	SelectorsHardened    int
	StepsRemoved         int
	EnvVars              map[string]string // varName -> comment about what was sanitized
}

func Run(trace *ir.Trace, opts Options) *Result {
	result := &Result{
		Trace:   trace,
		EnvVars: make(map[string]string),
	}

	if opts.Dedup {
		t, warnings, removed := runDedup(result.Trace)
		result.Trace = t
		result.Warnings = append(result.Warnings, warnings...)
		result.StepsRemoved += removed
	}

	if opts.HardenSelectors {
		t, warnings, hardened := runSelectorHardening(result.Trace)
		result.Trace = t
		result.Warnings = append(result.Warnings, warnings...)
		result.SelectorsHardened += hardened
	}

	if opts.InferWaits {
		t, warnings := runWaitInference(result.Trace)
		result.Trace = t
		result.Warnings = append(result.Warnings, warnings...)
	}

	{
		t, warnings, sanitized, envVars := runCredentialSanitization(result.Trace)
		result.Trace = t
		result.Warnings = append(result.Warnings, warnings...)
		result.CredentialsSanitized += sanitized
		for k, v := range envVars {
			result.EnvVars[k] = v
		}
	}

	if opts.DetectBranches {
		t, warnings := runBranchDetection(result.Trace)
		result.Trace = t
		result.Warnings = append(result.Warnings, warnings...)
	}

	if opts.InjectAssertions {
		t, warnings := runAssertionInjection(result.Trace)
		result.Trace = t
		result.Warnings = append(result.Warnings, warnings...)
	}

	return result
}
