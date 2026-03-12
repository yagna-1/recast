package ingestion

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseFile_SizeGuardStat(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "too-big.json")

	f, err := os.Create(p)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(52_428_801))
	require.NoError(t, f.Close())

	_, _, err = ParseFile(p)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds 50MB limit")
}
