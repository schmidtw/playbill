package writer_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/schmidtw/playbill/internal/writer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteNFO_CreatesWhenAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "The Matrix (1999).nfo")

	res, err := writer.WriteNFO(path, []byte("data"), false, false)
	require.NoError(t, err)
	assert.Equal(t, writer.Created, res)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "data", string(got))
}

func TestWriteNFO_SkipsWhenPresent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "The Matrix (1999).nfo")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))

	res, err := writer.WriteNFO(path, []byte("new"), false, false)
	require.NoError(t, err)
	assert.Equal(t, writer.Skipped, res)

	// The existing file is left untouched.
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "original", string(got))
}

func TestWriteNFO_ForceOverwritesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "The Matrix (1999).nfo")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))

	res, err := writer.WriteNFO(path, []byte("new"), true, false)
	require.NoError(t, err)
	assert.Equal(t, writer.Created, res)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "new", string(got), "force re-writes the existing file")
}

func TestWriteNFO_DryRunDoesNotWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "The Matrix (1999).nfo")

	res, err := writer.WriteNFO(path, []byte("data"), false, true)
	require.NoError(t, err)
	assert.Equal(t, writer.Planned, res)

	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err), "dry-run must not create the file")
}

func TestWriteNFO_DryRunStillSkipsExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "The Matrix (1999).nfo")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))

	res, err := writer.WriteNFO(path, []byte("new"), false, true)
	require.NoError(t, err)
	assert.Equal(t, writer.Skipped, res)
}

func TestWriteNFO_ForceDryRunPlansOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "The Matrix (1999).nfo")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))

	res, err := writer.WriteNFO(path, []byte("new"), true, true)
	require.NoError(t, err)
	assert.Equal(t, writer.Planned, res, "force + dry-run would overwrite, but writes nothing")

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "original", string(got))
}

// artServer serves the given bytes for any request, recording how many times it
// was hit so a test can assert a download was (or was not) performed.
func artServer(t *testing.T, body []byte) (*httptest.Server, *int) {
	t.Helper()
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestWriteArt_DownloadsWhenAbsent(t *testing.T) {
	srv, hits := artServer(t, []byte("\xff\xd8image"))
	path := filepath.Join(t.TempDir(), "The Matrix (1999)-poster.jpg")

	res, err := writer.WriteArt(srv.Client(), path, srv.URL, false, false)
	require.NoError(t, err)
	assert.Equal(t, writer.Created, res)
	assert.Equal(t, 1, *hits)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "\xff\xd8image", string(got))
}

func TestWriteArt_SkipsWhenPresentWithoutDownloading(t *testing.T) {
	srv, hits := artServer(t, []byte("new"))
	path := filepath.Join(t.TempDir(), "The Matrix (1999)-poster.jpg")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))

	res, err := writer.WriteArt(srv.Client(), path, srv.URL, false, false)
	require.NoError(t, err)
	assert.Equal(t, writer.Skipped, res)
	assert.Equal(t, 0, *hits, "an existing art file is not re-downloaded")

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "original", string(got))
}

func TestWriteArt_ForceReDownloadsExisting(t *testing.T) {
	srv, hits := artServer(t, []byte("new"))
	path := filepath.Join(t.TempDir(), "The Matrix (1999)-poster.jpg")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))

	res, err := writer.WriteArt(srv.Client(), path, srv.URL, true, false)
	require.NoError(t, err)
	assert.Equal(t, writer.Created, res)
	assert.Equal(t, 1, *hits)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "new", string(got))
}

func TestWriteArt_DryRunPlansWithoutDownloading(t *testing.T) {
	srv, hits := artServer(t, []byte("img"))
	path := filepath.Join(t.TempDir(), "The Matrix (1999)-poster.jpg")

	res, err := writer.WriteArt(srv.Client(), path, srv.URL, false, true)
	require.NoError(t, err)
	assert.Equal(t, writer.Planned, res)
	assert.Equal(t, 0, *hits, "dry-run does not download")

	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr))
}

func TestWriteArt_HTTPErrorIsReturned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	path := filepath.Join(t.TempDir(), "The Matrix (1999)-poster.jpg")

	_, err := writer.WriteArt(srv.Client(), path, srv.URL, false, false)
	require.Error(t, err)

	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "a failed download writes no file")
}
