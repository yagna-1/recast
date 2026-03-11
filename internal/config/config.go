// Package config manages recast configuration from flags, env vars, and config files.
package config

type Config struct {
	OutputDir        string
	Target           string
	NoOptimize       bool
	NoHarden         bool
	InjectAssertions bool
	Strict           bool
	Verbose          bool
	MaxFileSize      int64

	CompileTimeoutSec int
}

func Default() *Config {
	return &Config{
		OutputDir:         "./recast-out/",
		Target:            "playwright-ts",
		MaxFileSize:       52_428_800, // 50 MB
		CompileTimeoutSec: 30,
	}
}
