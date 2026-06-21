package probe_test

import (
	"testing"

	"github.com/schmidtw/playbill/internal/probe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMP4Prober_Probe(t *testing.T) {
	got, err := probe.MP4Prober{}.Probe("testdata/sample.m4v")
	require.NoError(t, err)

	assert.Equal(t, "h264", got.Video.Codec)
	assert.Equal(t, 320, got.Video.Width)
	assert.Equal(t, 240, got.Video.Height)
	assert.InDelta(t, 1.333, got.Video.Aspect, 0.01)
	assert.Equal(t, "progressive", got.Video.ScanType)
	assert.Equal(t, 1, got.Video.DurationInSeconds)

	require.Len(t, got.Audio, 1)
	assert.Equal(t, "aac", got.Audio[0].Codec)
	assert.Equal(t, "eng", got.Audio[0].Language)
	assert.Equal(t, 2, got.Audio[0].Channels)
}

func TestProbe_DispatchesByExtension(t *testing.T) {
	got, err := probe.Probe("testdata/sample.m4v")
	require.NoError(t, err)
	assert.Equal(t, "h264", got.Video.Codec)
}

func TestProbe_UnsupportedContainer(t *testing.T) {
	_, err := probe.Probe("/some/movie.avi")
	assert.ErrorIs(t, err, probe.ErrUnsupportedContainer)
}

func TestMP4Prober_MissingFile(t *testing.T) {
	_, err := probe.MP4Prober{}.Probe("testdata/does-not-exist.m4v")
	require.Error(t, err)
	assert.NotErrorIs(t, err, probe.ErrUnsupportedContainer)
}
