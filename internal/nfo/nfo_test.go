package nfo_test

import (
	"os"
	"testing"

	"github.com/schmidtw/playbill/internal/nfo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshal_Minimal(t *testing.T) {
	m := nfo.Movie{
		Title: "The Matrix",
		Year:  1999,
	}

	got, err := nfo.Marshal(m)
	require.NoError(t, err)

	want, err := os.ReadFile("testdata/minimal.nfo")
	require.NoError(t, err)

	assert.Equal(t, string(want), string(got))
}
