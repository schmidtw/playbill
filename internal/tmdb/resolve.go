package tmdb

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/schmidtw/playbill/internal/nfo"
)

// imageBaseURL is the TMDB image CDN root for original-size images. Cast thumbs
// are referenced as full URLs in the NFO, never downloaded.
const imageBaseURL = "https://image.tmdb.org/t/p/original"

// ResolveRequest asks for the canonical movie behind a Movie Folder. Title and
// Year come from the folder name. KnownID, when non-empty, is a TMDB id from a
// prior run's NFO: it short-circuits the search so a previous (possibly
// hand-corrected) match is trusted.
type ResolveRequest struct {
	Title   string
	Year    int
	KnownID string
}

// Resolve turns a request into the rich, canonical nfo.Movie. With a KnownID it
// fetches details directly; otherwise it searches by title+year first. It
// returns ErrNoMatch or ErrAmbiguousMatch when no confident match exists so the
// caller can skip and report rather than guess.
func (c *Client) Resolve(req ResolveRequest) (nfo.Movie, error) {
	id := req.KnownID
	if id == "" {
		matched, err := c.search(req.Title, req.Year)
		if err != nil {
			return nfo.Movie{}, err
		}
		id = strconv.Itoa(matched)
	}

	details, err := c.movie(id)
	if err != nil {
		return nfo.Movie{}, err
	}
	return mapMovie(details), nil
}

// search queries TMDB for title+year and returns the single confident match's
// id. See the package errors for the no-match and ambiguous-match cases.
func (c *Client) search(title string, year int) (int, error) {
	q := url.Values{}
	q.Set("query", title)
	if year > 0 {
		q.Set("year", strconv.Itoa(year))
	}
	q.Set("include_adult", "false")

	var resp searchResponse
	if err := c.get("/search/movie", q, &resp); err != nil {
		return 0, err
	}
	return chooseMatch(resp.Results, title, year)
}

// chooseMatch applies the confident-match policy: among results in the right
// year, prefer an exact (normalized) title match; a lone candidate is trusted,
// but several non-exact (or several exact) candidates are ambiguous.
func chooseMatch(results []searchResult, title string, year int) (int, error) {
	var candidates []searchResult
	for _, r := range results {
		if releaseYear(r.ReleaseDate) == year {
			candidates = append(candidates, r)
		}
	}
	if len(candidates) == 0 {
		return 0, ErrNoMatch
	}

	want := normalize(title)
	var exact []searchResult
	for _, r := range candidates {
		if normalize(r.Title) == want || normalize(r.OriginalTitle) == want {
			exact = append(exact, r)
		}
	}
	switch {
	case len(exact) == 1:
		return exact[0].ID, nil
	case len(exact) > 1:
		return 0, ErrAmbiguousMatch
	case len(candidates) == 1:
		return candidates[0].ID, nil
	default:
		return 0, ErrAmbiguousMatch
	}
}

// movie fetches full details for a TMDB id, asking for the credits, external
// IDs, certification, and videos in one request.
func (c *Client) movie(id string) (movieResponse, error) {
	q := url.Values{}
	q.Set("append_to_response", "credits,external_ids,release_dates,videos")

	var resp movieResponse
	if err := c.get("/movie/"+id, q, &resp); err != nil {
		return movieResponse{}, err
	}
	return resp, nil
}

// get performs a GET against the API, decodes a JSON body into out, and turns a
// non-2xx status into an error.
func (c *Client) get(path string, q url.Values, out any) error {
	q.Set("api_key", c.apiKey)
	endpoint := c.baseURL + path + "?" + q.Encode()

	resp, err := c.http.Get(endpoint)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("tmdb: %s: %s: %s", path, resp.Status, strings.TrimSpace(string(body)))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("tmdb: %s: decode: %w", path, err)
	}
	return nil
}

// normalize lowercases and strips non-alphanumeric runes so title comparison is
// punctuation- and spacing-insensitive.
func normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// releaseYear extracts the four-digit year from an ISO release date, or 0.
func releaseYear(date string) int {
	if len(date) < 4 {
		return 0
	}
	y, err := strconv.Atoi(date[:4])
	if err != nil {
		return 0
	}
	return y
}
