// Package artselect chooses the single best Artwork image for each art type
// from a set of candidates, and is pure: candidates plus a language preference
// in, chosen images out, no I/O.
//
// The selection policy is deterministic so a re-run picks the same image: within
// an art type, candidates are ranked by preferred language, then provider
// precedence (Fanart.tv over TMDB), then popularity, then resolution, with the
// URL as a final stable tiebreak. See the PRD "Selection & output specifics".
package artselect

import "sort"

// Kind is an art type, used both to group candidates and to name the file on
// disk (e.g. "Title (Year)-poster.jpg").
type Kind string

// The art types playbill understands. TMDB supplies Poster, Fanart, and
// Clearlogo; the remaining types come from Fanart.tv.
const (
	Poster    Kind = "poster"
	Fanart    Kind = "fanart"
	Banner    Kind = "banner"
	Clearlogo Kind = "clearlogo"
	Discart   Kind = "discart"
	Landscape Kind = "landscape"
)

// Provider names the source of a candidate image. For art types both providers
// offer, Fanart.tv is preferred over TMDB.
type Provider string

// The image providers, in order of precedence.
const (
	ProviderFanart Provider = "fanart"
	ProviderTMDB   Provider = "tmdb"
)

// Image is one candidate artwork image. URL is the absolute download URL;
// Language is its ISO-639-1 code ("" for a language-neutral/textless image);
// Popularity is the provider's score; Width and Height are the pixel size.
type Image struct {
	Kind       Kind
	Provider   Provider
	URL        string
	Language   string
	Popularity float64
	Width      int
	Height     int
}

// kindOrder is the canonical output ordering of art types, matching the PRD's
// default art set, so Select's result is deterministic regardless of input or
// map iteration order.
var kindOrder = map[Kind]int{
	Poster:    0,
	Fanart:    1,
	Banner:    2,
	Clearlogo: 3,
	Discart:   4,
	Landscape: 5,
}

// Select picks the single best Image for each Kind present in candidates and
// returns the winners sorted by the canonical art-type order. The selection is
// fully deterministic: a tie that survives every policy key breaks on the lower
// URL, so the result does not depend on the input order.
func Select(candidates []Image, preferredLang string) []Image {
	best := map[Kind]Image{}
	for _, img := range candidates {
		cur, ok := best[img.Kind]
		if !ok || better(img, cur, preferredLang) {
			best[img.Kind] = img
		}
	}

	out := make([]Image, 0, len(best))
	for _, img := range best {
		out = append(out, img)
	}
	sort.Slice(out, func(i, j int) bool {
		if ki, kj := kindOrder[out[i].Kind], kindOrder[out[j].Kind]; ki != kj {
			return ki < kj
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

// better reports whether candidate a is a strictly better choice than b under
// the selection policy: preferred language, then provider precedence (Fanart.tv
// over TMDB), then popularity, then resolution. Provider precedence outranks
// popularity because the two providers' scores are not comparable.
func better(a, b Image, preferredLang string) bool {
	if ra, rb := langRank(a.Language, preferredLang), langRank(b.Language, preferredLang); ra != rb {
		return ra < rb
	}
	if pa, pb := providerRank(a.Provider), providerRank(b.Provider); pa != pb {
		return pa < pb
	}
	if a.Popularity != b.Popularity {
		return a.Popularity > b.Popularity
	}
	if ra, rb := resolution(a), resolution(b); ra != rb {
		return ra > rb
	}
	return a.URL < b.URL
}

// providerRank orders sources by precedence: Fanart.tv before TMDB before any
// unknown provider. Lower is better.
func providerRank(p Provider) int {
	switch p {
	case ProviderFanart:
		return 0
	case ProviderTMDB:
		return 1
	default:
		return 2
	}
}

// resolution is a candidate's pixel area, used as the resolution tiebreak.
func resolution(img Image) int { return img.Width * img.Height }

// langRank scores a candidate's language against the preference: the preferred
// language is best, a language-neutral image next, any other language last.
// Lower is better.
func langRank(lang, preferred string) int {
	switch lang {
	case preferred:
		return 0
	case "":
		return 1
	default:
		return 2
	}
}
