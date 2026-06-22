// Package tmdb is a small client for The Movie Database (TMDB) v3 REST API. It
// searches for a movie by the folder's title and year, fetches full details
// (credits, external IDs, certification, videos), and maps the result into the
// canonical nfo.Movie model — see ADR-0003.
//
// TMDB is the required provider: it supplies identity, metadata, and the
// baseline that turns a minimal NFO into the rich, MediaElch-style one. The API
// key is read from the environment by the caller and must not be empty.
package tmdb

import (
	"errors"
	"net/http"
	"time"
)

// defaultBaseURL is the TMDB v3 API root.
const defaultBaseURL = "https://api.themoviedb.org/3"

// Sentinel errors. ErrNoAPIKey is fatal (the tool refuses to run without a
// key); ErrNoMatch and ErrAmbiguousMatch are per-folder signals the caller
// records in the report and never guesses past.
var (
	// ErrNoAPIKey means New was called without a TMDB API key.
	ErrNoAPIKey = errors.New("tmdb: TMDB_API_KEY is required")
	// ErrNoMatch means no confident title+year match was found.
	ErrNoMatch = errors.New("tmdb: no confident match")
	// ErrAmbiguousMatch means several equally-plausible matches were found.
	ErrAmbiguousMatch = errors.New("tmdb: ambiguous match")
)

// Client talks to the TMDB v3 REST API.
type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the API root (used in tests and behind proxies).
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = u } }

// WithHTTPClient overrides the HTTP client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// New builds a Client. It returns ErrNoAPIKey when apiKey is empty so a missing
// TMDB_API_KEY is a clear fatal error.
func New(apiKey string, opts ...Option) (*Client, error) {
	if apiKey == "" {
		return nil, ErrNoAPIKey
	}
	c := &Client{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}
