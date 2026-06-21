package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/schmidtw/playbill/internal/nfo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mkVideo(t *testing.T, root, folder string) {
	t.Helper()
	dir := filepath.Join(root, folder)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, folder+".mkv"), []byte("v"), 0o644))
}

func TestRun_WritesNFOForValidFolder(t *testing.T) {
	root := t.TempDir()
	mkVideo(t, root, "The Matrix (1999)")

	var out bytes.Buffer
	err := run(config{dir: root, out: &out})
	require.NoError(t, err)

	nfoPath := filepath.Join(root, "The Matrix (1999)", "The Matrix (1999).nfo")
	got, err := os.ReadFile(nfoPath)
	require.NoError(t, err)

	want, err := nfo.Marshal(nfo.Movie{Title: "The Matrix", Year: 1999})
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got))
	assert.Contains(t, out.String(), "enriched: 1")
}

func TestRun_IntegratesStreamDetailsFromVideo(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "The Matrix (1999)")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	fixture, err := os.ReadFile(filepath.Join("..", "..", "internal", "probe", "testdata", "sample.m4v"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "The Matrix (1999).m4v"), fixture, 0o644))

	var out bytes.Buffer
	require.NoError(t, run(config{dir: root, out: &out}))

	got, err := os.ReadFile(filepath.Join(dir, "The Matrix (1999).nfo"))
	require.NoError(t, err)

	body := string(got)
	assert.Contains(t, body, "<fileinfo>")
	assert.Contains(t, body, "<codec>h264</codec>")
	assert.Contains(t, body, "<width>320</width>")
	assert.Contains(t, body, "<language>eng</language>")
}

func TestRun_UnprobeableVideoOmitsStreamDetails(t *testing.T) {
	root := t.TempDir()
	mkVideo(t, root, "The Matrix (1999)") // writes a dummy .mkv we cannot probe yet

	var out bytes.Buffer
	require.NoError(t, run(config{dir: root, out: &out}))

	got, err := os.ReadFile(filepath.Join(root, "The Matrix (1999)", "The Matrix (1999).nfo"))
	require.NoError(t, err)
	assert.NotContains(t, string(got), "<fileinfo>")
	assert.Contains(t, out.String(), "enriched: 1")
}

func TestRun_LeavesExistingNFOUntouched(t *testing.T) {
	root := t.TempDir()
	mkVideo(t, root, "The Matrix (1999)")
	nfoPath := filepath.Join(root, "The Matrix (1999)", "The Matrix (1999).nfo")
	require.NoError(t, os.WriteFile(nfoPath, []byte("hand-tuned"), 0o644))

	var out bytes.Buffer
	err := run(config{dir: root, out: &out})
	require.NoError(t, err)

	got, err := os.ReadFile(nfoPath)
	require.NoError(t, err)
	assert.Equal(t, "hand-tuned", string(got))
	assert.Contains(t, out.String(), "skipped: 1")
}

func TestRun_DryRunWritesNothing(t *testing.T) {
	root := t.TempDir()
	mkVideo(t, root, "The Matrix (1999)")

	var out bytes.Buffer
	err := run(config{dir: root, dryRun: true, out: &out})
	require.NoError(t, err)

	nfoPath := filepath.Join(root, "The Matrix (1999)", "The Matrix (1999).nfo")
	_, statErr := os.Stat(nfoPath)
	assert.True(t, os.IsNotExist(statErr), "dry-run must not write the NFO")
	assert.Contains(t, out.String(), "planned: 1")
}

func TestRun_UnmatchedFolderReported(t *testing.T) {
	root := t.TempDir()
	mkVideo(t, root, "Random Folder")

	var out bytes.Buffer
	err := run(config{dir: root, out: &out})
	require.NoError(t, err)

	nfoPath := filepath.Join(root, "Random Folder", "Random Folder.nfo")
	_, statErr := os.Stat(nfoPath)
	assert.True(t, os.IsNotExist(statErr), "unmatched folder must not be enriched")
	assert.Contains(t, out.String(), "unmatched: 1")
	assert.Contains(t, out.String(), "Random Folder")
}

func TestRun_MissingDirIsError(t *testing.T) {
	var out bytes.Buffer
	err := run(config{dir: filepath.Join(t.TempDir(), "nope"), out: &out})
	assert.Error(t, err)
}

func TestParseArgs_DefaultsAndDir(t *testing.T) {
	var errOut bytes.Buffer
	cfg, err := parseArgs([]string{"--dir", "/movies"}, &errOut)
	require.NoError(t, err)
	assert.Equal(t, "/movies", cfg.dir)
	assert.False(t, cfg.dryRun)
}

func TestParseArgs_DryRun(t *testing.T) {
	var errOut bytes.Buffer
	cfg, err := parseArgs([]string{"--dir", "/movies", "--dry-run"}, &errOut)
	require.NoError(t, err)
	assert.True(t, cfg.dryRun)
}

func TestParseArgs_MissingDir(t *testing.T) {
	var errOut bytes.Buffer
	_, err := parseArgs([]string{}, &errOut)
	assert.ErrorIs(t, err, errMissingDir)
	assert.Contains(t, errOut.String(), "--dir is required")
}

func TestParseArgs_UnknownFlag(t *testing.T) {
	var errOut bytes.Buffer
	_, err := parseArgs([]string{"--bogus"}, &errOut)
	assert.Error(t, err)
}

func TestRealMain_SuccessReturnsZero(t *testing.T) {
	root := t.TempDir()
	mkVideo(t, root, "The Matrix (1999)")

	var out, errOut bytes.Buffer
	code := realMain([]string{"--dir", root}, &out, &errOut)
	assert.Equal(t, 0, code)
	assert.Contains(t, out.String(), "enriched: 1")
}

func TestRealMain_BadUsageReturnsTwo(t *testing.T) {
	var out, errOut bytes.Buffer
	code := realMain([]string{}, &out, &errOut)
	assert.Equal(t, 2, code)
}

func TestRealMain_ScanFailureReturnsOne(t *testing.T) {
	var out, errOut bytes.Buffer
	code := realMain([]string{"--dir", filepath.Join(t.TempDir(), "nope")}, &out, &errOut)
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut.String(), "error:")
}

func TestRun_WriteErrorIsReportedNotFatal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("write-permission errors do not occur when running as root")
	}

	root := t.TempDir()
	dir := filepath.Join(root, "The Matrix (1999)")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "The Matrix (1999).mkv"), []byte("v"), 0o644))
	// Make the folder read-only so the NFO cannot be created.
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	var out bytes.Buffer
	err := run(config{dir: root, out: &out})
	require.NoError(t, err) // one bad folder never aborts the run
	assert.Contains(t, out.String(), "errored: 1")
}
