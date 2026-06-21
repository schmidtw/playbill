// Package nameparse extracts a movie's title and year from its folder name.
//
// A Movie Folder is named "Title (Year)". The folder name is the ground-truth
// source of the title and year — see CONTEXT.md and ADR-0001.
package nameparse

import (
	"regexp"
	"strconv"
	"strings"
)

// yearGroup matches a parenthesized four-digit year, e.g. "(1999)".
var yearGroup = regexp.MustCompile(`\((\d{4})\)`)

// Parse extracts the title and year from a "Title (Year)" folder name.
//
// The year is taken from the rightmost "(YYYY)" group, so a title that itself
// contains a year (e.g. "2012 (2009)") is handled correctly. The title is
// everything before that group, with surrounding whitespace trimmed. ok is
// false when no year group is present or the title is empty.
func Parse(folder string) (title string, year int, ok bool) {
	matches := yearGroup.FindAllStringSubmatchIndex(folder, -1)
	if len(matches) == 0 {
		return "", 0, false
	}

	last := matches[len(matches)-1]
	yearStr := folder[last[2]:last[3]]
	y, err := strconv.Atoi(yearStr)
	if err != nil {
		return "", 0, false
	}

	title = strings.TrimSpace(folder[:last[0]])
	if title == "" {
		return "", 0, false
	}

	return title, y, true
}
