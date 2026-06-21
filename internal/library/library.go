// Package library walks a movie root and yields the Movie Folders it finds.
//
// A Movie Folder is an immediate subdirectory of the root that contains a video
// file (see CONTEXT.md). Walking is thin I/O only: it does not parse names or
// decide what to enrich — that happens downstream.
package library

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// videoExts is the set of recognized video file extensions (lowercase).
var videoExts = map[string]bool{
	".mkv":  true,
	".mp4":  true,
	".m4v":  true,
	".avi":  true,
	".mov":  true,
	".wmv":  true,
	".mpg":  true,
	".mpeg": true,
	".ts":   true,
	".m2ts": true,
	".flv":  true,
	".webm": true,
}

// MovieFolder is a directory holding one movie's video file.
type MovieFolder struct {
	// Path is the path to the folder.
	Path string
	// Name is the folder's base name, e.g. "The Matrix (1999)".
	Name string
	// VideoFile is the base name of the video file found in the folder.
	VideoFile string
}

// Scan returns the Movie Folders directly under root, sorted by name. A folder
// qualifies when it contains at least one recognized video file; the first such
// file (alphabetically) is recorded as the VideoFile.
func Scan(root string) ([]MovieFolder, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var folders []MovieFolder
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dir := filepath.Join(root, entry.Name())
		video, err := firstVideo(dir)
		if err != nil {
			return nil, err
		}
		if video == "" {
			continue
		}

		folders = append(folders, MovieFolder{
			Path:      dir,
			Name:      entry.Name(),
			VideoFile: video,
		})
	}

	sort.Slice(folders, func(i, j int) bool {
		return folders[i].Name < folders[j].Name
	})

	return folders, nil
}

// firstVideo returns the alphabetically-first video file name in dir, or "" if
// none is present.
func firstVideo(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}

	var videos []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if videoExts[ext] {
			videos = append(videos, entry.Name())
		}
	}

	if len(videos) == 0 {
		return "", nil
	}

	sort.Strings(videos)
	return videos[0], nil
}
