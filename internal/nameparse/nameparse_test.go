package nameparse_test

import (
	"testing"

	"github.com/schmidtw/playbill/internal/nameparse"
	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name      string
		folder    string
		wantTitle string
		wantYear  int
		wantOK    bool
	}{
		{
			name:      "simple title and year",
			folder:    "The Matrix (1999)",
			wantTitle: "The Matrix",
			wantYear:  1999,
			wantOK:    true,
		},
		{
			name:      "year in the title uses rightmost group",
			folder:    "2012 (2009)",
			wantTitle: "2012",
			wantYear:  2009,
			wantOK:    true,
		},
		{
			name:      "extra tokens after the year group",
			folder:    "The Matrix (1999) [1080p]",
			wantTitle: "The Matrix",
			wantYear:  1999,
			wantOK:    true,
		},
		{
			name:      "unicode title",
			folder:    "Amélie (2001)",
			wantTitle: "Amélie",
			wantYear:  2001,
			wantOK:    true,
		},
		{
			name:      "title with parenthetical aside before year",
			folder:    "Movie (Special Edition) (2010)",
			wantTitle: "Movie (Special Edition)",
			wantYear:  2010,
			wantOK:    true,
		},
		{
			name:   "no year",
			folder: "Some Random Folder",
			wantOK: false,
		},
		{
			name:   "year group but empty title",
			folder: "(1999)",
			wantOK: false,
		},
		{
			name:   "non-four-digit parenthetical is not a year",
			folder: "Movie (123)",
			wantOK: false,
		},
		{
			name:   "empty string",
			folder: "",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title, year, ok := nameparse.Parse(tt.folder)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantTitle, title)
			assert.Equal(t, tt.wantYear, year)
		})
	}
}
