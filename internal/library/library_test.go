package library_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/schmidtw/playbill/internal/library"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFile creates an empty file, making parent directories as needed.
func writeFile(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o644))
}

func TestScan_FindsFoldersWithVideo(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "The Matrix (1999)", "The Matrix (1999).mkv"))
	writeFile(t, filepath.Join(root, "Amélie (2001)", "Amélie (2001).mp4"))

	folders, err := library.Scan(root)
	require.NoError(t, err)

	require.Len(t, folders, 2)
	// Results are sorted by folder name for deterministic processing.
	assert.Equal(t, "Amélie (2001)", folders[0].Name)
	assert.Equal(t, "Amélie (2001).mp4", folders[0].VideoFile)
	assert.Equal(t, filepath.Join(root, "Amélie (2001)"), folders[0].Path)
	assert.Equal(t, "The Matrix (1999)", folders[1].Name)
}

func TestScan_IgnoresFoldersWithoutVideo(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "The Matrix (1999)", "The Matrix (1999).mkv"))
	writeFile(t, filepath.Join(root, "Not A Movie", "readme.txt"))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "Empty Folder"), 0o755))

	folders, err := library.Scan(root)
	require.NoError(t, err)

	require.Len(t, folders, 1)
	assert.Equal(t, "The Matrix (1999)", folders[0].Name)
}

func TestScan_MissingRoot(t *testing.T) {
	_, err := library.Scan(filepath.Join(t.TempDir(), "does-not-exist"))
	assert.Error(t, err)
}

func TestScan_SingleMovieFolder(t *testing.T) {
	parent := t.TempDir()
	movie := filepath.Join(parent, "Brave (2012)")
	writeFile(t, filepath.Join(movie, "Brave (2012).m4v"))

	// Pointing -dir directly at a movie folder treats it as a single movie.
	folders, err := library.Scan(movie)
	require.NoError(t, err)

	require.Len(t, folders, 1)
	assert.Equal(t, "Brave (2012)", folders[0].Name)
	assert.Equal(t, "Brave (2012).m4v", folders[0].VideoFile)
	assert.Equal(t, movie, folders[0].Path)
}

func TestScan_SingleMovieFolderIgnoresSubdirs(t *testing.T) {
	parent := t.TempDir()
	movie := filepath.Join(parent, "Brave (2012)")
	writeFile(t, filepath.Join(movie, "Brave (2012).m4v"))
	// An extras subfolder with its own video must not become a second movie.
	writeFile(t, filepath.Join(movie, "extras", "deleted-scene.mkv"))

	folders, err := library.Scan(movie)
	require.NoError(t, err)

	require.Len(t, folders, 1)
	assert.Equal(t, "Brave (2012)", folders[0].Name)
	assert.Equal(t, "Brave (2012).m4v", folders[0].VideoFile)
}
