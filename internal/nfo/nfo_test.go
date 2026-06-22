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

func TestMarshal_UniqueIDsAndLegacyID(t *testing.T) {
	m := nfo.Movie{
		Title: "The Matrix",
		Year:  1999,
		UniqueIDs: []nfo.UniqueID{
			{Type: "tmdb", Default: true, Value: "603"},
			{Type: "imdb", Value: "tt0133093"},
		},
	}

	got, err := nfo.Marshal(m)
	require.NoError(t, err)

	body := string(got)
	assert.Contains(t, body, `<uniqueid type="tmdb" default="true">603</uniqueid>`)
	assert.Contains(t, body, `<uniqueid type="imdb">tt0133093</uniqueid>`)
	// Legacy <id> mirrors the default unique id for older skins.
	assert.Contains(t, body, "<id>603</id>")
}

func TestMarshal_WithStreamDetails(t *testing.T) {
	m := nfo.Movie{
		Title: "The Matrix",
		Year:  1999,
		StreamDetails: &nfo.StreamDetails{
			Video: &nfo.VideoStream{
				Codec:             "h264",
				Aspect:            1.78,
				Width:             1920,
				Height:            1080,
				DurationInSeconds: 8160,
				ScanType:          "progressive",
			},
			Audio: []nfo.AudioStream{
				{Codec: "aac", Language: "eng", Channels: 2},
				{Codec: "ac3", Language: "fra", Channels: 6},
			},
		},
	}

	got, err := nfo.Marshal(m)
	require.NoError(t, err)

	want, err := os.ReadFile("testdata/streamdetails.nfo")
	require.NoError(t, err)

	assert.Equal(t, string(want), string(got))
}
