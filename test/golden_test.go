// Package golden provides golden-file integration tests.
package golden

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourorg/recast/internal/emitter"
	"github.com/yourorg/recast/internal/ingestion"
	"github.com/yourorg/recast/internal/optimizer"
	ir "github.com/yourorg/recast/recast-ir"
)

var update = flag.Bool("update", false, "update golden files instead of asserting")

type goldenCase struct {
	fixture string // relative to testdata/fixtures/
	target  string // emitter target name
	golden  string // relative to testdata/golden/
}

var goldenCases = []goldenCase{
	{
		fixture: "workflow_use_login.json",
		target:  "playwright-ts",
		golden:  "workflow_use_login.playwright-ts.golden",
	},
	{
		fixture: "workflow_use_login.json",
		target:  "playwright-py",
		golden:  "workflow_use_login.playwright-py.golden",
	},
	{
		fixture: "mcp_login.jsonl",
		target:  "playwright-ts",
		golden:  "mcp_login.playwright-ts.golden",
	},
}

const fixturesDir = "../testdata/fixtures"
const goldenDir = "../testdata/golden"

func TestGolden(t *testing.T) {
	for _, tc := range goldenCases {
		tc := tc
		t.Run(tc.fixture+"_"+tc.target, func(t *testing.T) {
			fixturePath := filepath.Join(fixturesDir, tc.fixture)
			goldenPath := filepath.Join(goldenDir, tc.golden)

			trace, _, err := ingestion.ParseFile(fixturePath)
			require.NoError(t, err)

			valResult := ir.Validate(trace)
			require.False(t, valResult.HasErrors(), "IR validation failed: %s", valResult.Error())

			optResult := optimizer.Run(trace, optimizer.DefaultOptions())

			em, err := emitter.Get(tc.target)
			require.NoError(t, err)

			emitResult, err := em.Emit(optResult.Trace, optResult.EnvVars)
			require.NoError(t, err)

			actual := emitResult.TestFile

			if *update {
				err := os.MkdirAll(filepath.Dir(goldenPath), 0755)
				require.NoError(t, err)
				err = os.WriteFile(goldenPath, []byte(actual), 0644)
				require.NoError(t, err)
				t.Logf("updated golden file: %s", goldenPath)
				return
			}

			expected, err := os.ReadFile(goldenPath)
			if os.IsNotExist(err) {
				t.Fatalf("golden file not found: %s\nRun 'make golden-update' to generate it.", goldenPath)
			}
			require.NoError(t, err)

			assert.Equal(t, string(expected), actual,
				"output doesn't match golden file %s\nRun 'make golden-update' to regenerate.", tc.golden)
		})
	}
}

func TestGolden_Deterministic(t *testing.T) {
	fixturePath := filepath.Join(fixturesDir, "workflow_use_login.json")

	compile := func() string {
		trace, _, err := ingestion.ParseFile(fixturePath)
		require.NoError(t, err)
		optResult := optimizer.Run(trace, optimizer.DefaultOptions())
		em, err := emitter.Get("playwright-ts")
		require.NoError(t, err)
		result, err := em.Emit(optResult.Trace, optResult.EnvVars)
		require.NoError(t, err)
		return result.TestFile
	}

	first := compile()
	second := compile()
	assert.Equal(t, first, second, "compilation is not deterministic")
}
