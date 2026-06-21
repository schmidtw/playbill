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
