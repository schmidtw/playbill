package tmdb_test

import (
	"testing"

	"github.com/schmidtw/playbill/internal/tmdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_EmptyKeyIsError(t *testing.T) {
	_, err := tmdb.New("")
	assert.ErrorIs(t, err, tmdb.ErrNoAPIKey)
}

func TestNew_WithKeySucceeds(t *testing.T) {
	c, err := tmdb.New("deadbeef")
	require.NoError(t, err)
	assert.NotNil(t, c)
}
