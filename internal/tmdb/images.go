package tmdb

import (
	"net/url"

	"github.com/schmidtw/playbill/internal/artselect"
)

// imagesResponse is the /movie/{id}/images payload. TMDB groups images by the
// art types it offers: posters, backdrops (fanart), and logos (clearlogo).
type imagesResponse struct {
	Posters   []imageEntry `json:"posters"`
	Backdrops []imageEntry `json:"backdrops"`
	Logos     []imageEntry `json:"logos"`
}

// imageEntry is one image in an images payload. Language is the ISO-639-1 code,
// null for a language-neutral image; VoteAverage is the community score used as
// the popularity tiebreak.
type imageEntry struct {
	FilePath    string  `json:"file_path"`
	Language    string  `json:"iso_639_1"`
	VoteAverage float64 `json:"vote_average"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
}

// Images fetches the baseline artwork candidates for a TMDB id and maps them to
// the canonical artselect.Image model. TMDB posters become Poster candidates,
// backdrops become Fanart, and logos become Clearlogo; every candidate is
// tagged as the TMDB provider.
func (c *Client) Images(id string) ([]artselect.Image, error) {
	var resp imagesResponse
	if err := c.get("/movie/"+id+"/images", url.Values{}, &resp); err != nil {
		return nil, err
	}

	var out []artselect.Image
	out = appendImages(out, resp.Posters, artselect.Poster)
	out = appendImages(out, resp.Backdrops, artselect.Fanart)
	out = appendImages(out, resp.Logos, artselect.Clearlogo)
	return out, nil
}

// appendImages maps a group of TMDB image entries of one kind onto candidates,
// skipping any with no file path.
func appendImages(dst []artselect.Image, entries []imageEntry, kind artselect.Kind) []artselect.Image {
	for _, e := range entries {
		if e.FilePath == "" {
			continue
		}
		dst = append(dst, artselect.Image{
			Kind:       kind,
			Provider:   artselect.ProviderTMDB,
			URL:        imageURL(e.FilePath),
			Language:   e.Language,
			Popularity: e.VoteAverage,
			Width:      e.Width,
			Height:     e.Height,
		})
	}
	return dst
}
