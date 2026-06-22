// Package fanart is a small client for the Fanart.tv v3 movies API. It supplies
// the extended artwork types (banner, clearlogo, discart, landscape, clearart)
// that complement TMDB's baseline set — see ADR-0003 and the PRD.
//
// Fanart.tv is the optional provider: the tool runs without a key (those art
// types are simply skipped) and, for art types both providers offer, Fanart.tv
// is preferred over TMDB. The API key is read from the environment by the caller
// and must not be empty.
package fanart

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/schmidtw/playbill/internal/artselect"
	"golang.org/x/time/rate"
)

// defaultBaseURL is the Fanart.tv v3 API root.
const defaultBaseURL = "https://webservice.fanart.tv/v3"

// Fanart.tv rate limit. Personal API keys are generous; the client averages 5/s
// with a small burst so the bounded worker pool stays a good API citizen
// (user story 32).
const (
	defaultRate  = rate.Limit(5)
	defaultBurst = 5
)

// ErrNoAPIKey means New was called without a Fanart.tv API key.
var ErrNoAPIKey = errors.New("fanart: FANARTTV_API_KEY is required")

// Client talks to the Fanart.tv v3 movies API.
type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
	limiter *rate.Limiter
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the API root (used in tests and behind proxies).
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = u } }

// WithHTTPClient overrides the HTTP client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithRateLimiter overrides the request rate limiter. A nil limiter disables
// throttling; the default keeps the client a polite API citizen.
func WithRateLimiter(l *rate.Limiter) Option { return func(c *Client) { c.limiter = l } }

// New builds a Client. It returns ErrNoAPIKey when apiKey is empty so a missing
// FANARTTV_API_KEY is caught before a run, letting the caller degrade gracefully
// rather than issue keyless requests.
func New(apiKey string, opts ...Option) (*Client, error) {
	if apiKey == "" {
		return nil, ErrNoAPIKey
	}
	c := &Client{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
		limiter: rate.NewLimiter(defaultRate, defaultBurst),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// artImage is one image in a Fanart.tv art group. Likes is the community score
// (a string in the API) used as the popularity tiebreak; Lang is the ISO-639-1
// code, empty for a language-neutral image.
type artImage struct {
	URL   string `json:"url"`
	Lang  string `json:"lang"`
	Likes string `json:"likes"`
}

// movieArt is the Fanart.tv /movies payload: one slice per art type it knows.
// hdmovielogo/movielogo both map to clearlogo and hdmovieclearart/movieclearart
// both map to clearart; artselect then picks the best within each kind.
type movieArt struct {
	Poster     []artImage `json:"movieposter"`
	Background []artImage `json:"moviebackground"`
	Banner     []artImage `json:"moviebanner"`
	HDLogo     []artImage `json:"hdmovielogo"`
	Logo       []artImage `json:"movielogo"`
	Disc       []artImage `json:"moviedisc"`
	Thumb      []artImage `json:"moviethumb"`
	HDClearArt []artImage `json:"hdmovieclearart"`
	ClearArt   []artImage `json:"movieclearart"`
}

// Images fetches the extended artwork candidates for a movie id (a TMDB or IMDb
// id) and maps them to the canonical artselect.Image model, every candidate
// tagged as the Fanart.tv provider. A movie Fanart.tv has never seen (HTTP 404)
// yields no candidates and no error, so an unknown movie degrades gracefully
// rather than failing the folder.
func (c *Client) Images(id string) ([]artselect.Image, error) {
	q := url.Values{}
	q.Set("api_key", c.apiKey)

	var art movieArt
	found, err := c.get("/movies/"+id, q, &art)
	if err != nil || !found {
		return nil, err
	}

	var out []artselect.Image
	out = appendArt(out, art.Poster, artselect.Poster)
	out = appendArt(out, art.Background, artselect.Fanart)
	out = appendArt(out, art.Banner, artselect.Banner)
	out = appendArt(out, art.HDLogo, artselect.Clearlogo)
	out = appendArt(out, art.Logo, artselect.Clearlogo)
	out = appendArt(out, art.Disc, artselect.Discart)
	out = appendArt(out, art.Thumb, artselect.Landscape)
	out = appendArt(out, art.HDClearArt, artselect.Clearart)
	out = appendArt(out, art.ClearArt, artselect.Clearart)
	return out, nil
}

// appendArt maps a Fanart.tv art group of one kind onto candidates, skipping any
// entry with no URL and parsing the (string) likes count into the popularity
// score.
func appendArt(dst []artselect.Image, entries []artImage, kind artselect.Kind) []artselect.Image {
	for _, e := range entries {
		if e.URL == "" {
			continue
		}
		likes, _ := strconv.ParseFloat(e.Likes, 64)
		dst = append(dst, artselect.Image{
			Kind:       kind,
			Provider:   artselect.ProviderFanart,
			URL:        e.URL,
			Language:   e.Lang,
			Popularity: likes,
		})
	}
	return dst
}

// get issues a rate-limited GET against path with query q and decodes a JSON
// 200 body into out. It reports found=false (with no error) on a 404 so the
// caller can treat a movie Fanart.tv lacks as "no art" rather than a failure.
func (c *Client) get(path string, q url.Values, out any) (bool, error) {
	if c.limiter != nil {
		if err := c.limiter.Wait(context.Background()); err != nil {
			return false, err
		}
	}

	req, err := http.NewRequest(http.MethodGet, c.baseURL+path+"?"+q.Encode(), nil)
	if err != nil {
		return false, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return false, fmt.Errorf("fanart: %s: %s: %s", path, resp.Status, body)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return false, fmt.Errorf("fanart: decode %s: %w", path, err)
	}
	return true, nil
}
