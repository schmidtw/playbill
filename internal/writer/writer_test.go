package writer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/schmidtw/playbill/internal/writer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteNFO_CreatesWhenAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "The Matrix (1999).nfo")

	res, err := writer.WriteNFO(path, []byte("data"), false)
	require.NoError(t, err)
	assert.Equal(t, writer.Created, res)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "data", string(got))
}

func TestWriteNFO_SkipsWhenPresent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "The Matrix (1999).nfo")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))

	res, err := writer.WriteNFO(path, []byte("new"), false)
	require.NoError(t, err)
	assert.Equal(t, writer.Skipped, res)

	// The existing file is left untouched.
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "original", string(got))
}

func TestWriteNFO_DryRunDoesNotWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "The Matrix (1999).nfo")

	res, err := writer.WriteNFO(path, []byte("data"), true)
	require.NoError(t, err)
	assert.Equal(t, writer.Planned, res)

	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err), "dry-run must not create the file")
}

func TestWriteNFO_DryRunStillSkipsExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "The Matrix (1999).nfo")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))

	res, err := writer.WriteNFO(path, []byte("new"), true)
	require.NoError(t, err)
	assert.Equal(t, writer.Skipped, res)
}
