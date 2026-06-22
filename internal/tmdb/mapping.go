package tmdb

import (
	"strconv"

	"github.com/schmidtw/playbill/internal/nfo"
)

// searchResponse is the /search/movie payload.
type searchResponse struct {
	Results []searchResult `json:"results"`
}

type searchResult struct {
	ID            int    `json:"id"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	ReleaseDate   string `json:"release_date"`
}

// movieResponse is the /movie/{id} payload with appended sub-resources.
type movieResponse struct {
	ID                  int          `json:"id"`
	IMDBID              string       `json:"imdb_id"`
	Title               string       `json:"title"`
	OriginalTitle       string       `json:"original_title"`
	Overview            string       `json:"overview"`
	Tagline             string       `json:"tagline"`
	ReleaseDate         string       `json:"release_date"`
	Runtime             int          `json:"runtime"`
	VoteAverage         float64      `json:"vote_average"`
	VoteCount           int          `json:"vote_count"`
	Genres              []namedItem  `json:"genres"`
	ProductionCompanies []namedItem  `json:"production_companies"`
	ProductionCountries []namedItem  `json:"production_countries"`
	BelongsToCollection *namedItem   `json:"belongs_to_collection"`
	Credits             credits      `json:"credits"`
	ExternalIDs         externalIDs  `json:"external_ids"`
	ReleaseDates        releaseDates `json:"release_dates"`
	Videos              videos       `json:"videos"`
}

type namedItem struct {
	Name string `json:"name"`
}

type credits struct {
	Cast []castMember `json:"cast"`
	Crew []crewMember `json:"crew"`
}

type castMember struct {
	Name        string `json:"name"`
	Character   string `json:"character"`
	Order       int    `json:"order"`
	ProfilePath string `json:"profile_path"`
}

type crewMember struct {
	Name       string `json:"name"`
	Job        string `json:"job"`
	Department string `json:"department"`
}

type externalIDs struct {
	IMDBID string `json:"imdb_id"`
}

type releaseDates struct {
	Results []countryReleaseDates `json:"results"`
}

type countryReleaseDates struct {
	Country  string             `json:"iso_3166_1"`
	Releases []countryCertEntry `json:"release_dates"`
}

type countryCertEntry struct {
	Certification string `json:"certification"`
	Type          int    `json:"type"`
}

type videos struct {
	Results []video `json:"results"`
}

type video struct {
	Site string `json:"site"`
	Type string `json:"type"`
	Key  string `json:"key"`
}

// mapMovie turns a TMDB details payload into the canonical nfo.Movie.
func mapMovie(d movieResponse) nfo.Movie {
	m := nfo.Movie{
		Title:         d.Title,
		OriginalTitle: d.OriginalTitle,
		Year:          releaseYear(d.ReleaseDate),
		Premiered:     d.ReleaseDate,
		Runtime:       d.Runtime,
		Plot:          d.Overview,
		Tagline:       d.Tagline,
		MPAA:          certification(d.ReleaseDates),
		Genres:        names(d.Genres),
		Countries:     names(d.ProductionCountries),
		Studios:       names(d.ProductionCompanies),
		Trailer:       trailer(d.Videos),
		Ratings:       ratings(d),
		Actors:        cast(d.Credits.Cast),
		UniqueIDs:     uniqueIDs(d),
	}
	if d.BelongsToCollection != nil {
		m.Set = d.BelongsToCollection.Name
	}
	m.Directors, m.Writers = crewCredits(d.Credits.Crew)
	return m
}

// names extracts the Name field from a list of named items.
func names(items []namedItem) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Name)
	}
	return out
}

// ratings builds the canonical rating list from the TMDB vote fields, or nil
// when the movie is unrated.
func ratings(d movieResponse) []nfo.Rating {
	if d.VoteCount == 0 {
		return nil
	}
	return []nfo.Rating{{
		Name:    "themoviedb",
		Max:     10,
		Default: true,
		Value:   d.VoteAverage,
		Votes:   d.VoteCount,
	}}
}

// cast maps TMDB cast members to canonical actors with full thumb URLs.
func cast(members []castMember) []nfo.Actor {
	if len(members) == 0 {
		return nil
	}
	out := make([]nfo.Actor, 0, len(members))
	for _, c := range members {
		out = append(out, nfo.Actor{
			Name:  c.Name,
			Role:  c.Character,
			Order: c.Order,
			Thumb: imageURL(c.ProfilePath),
		})
	}
	return out
}

// crewCredits splits the crew into directors and writers, preserving order and
// de-duplicating repeated names within each role.
func crewCredits(crew []crewMember) (directors, writers []string) {
	seenDir := map[string]bool{}
	seenWri := map[string]bool{}
	for _, c := range crew {
		switch {
		case c.Job == "Director" && !seenDir[c.Name]:
			seenDir[c.Name] = true
			directors = append(directors, c.Name)
		case isWriter(c) && !seenWri[c.Name]:
			seenWri[c.Name] = true
			writers = append(writers, c.Name)
		}
	}
	return directors, writers
}

// isWriter reports whether a crew member is a writing credit.
func isWriter(c crewMember) bool {
	if c.Department == "Writing" {
		return true
	}
	return c.Job == "Writer" || c.Job == "Screenplay" || c.Job == "Story"
}

// uniqueIDs builds the provider id list: TMDB (default) plus IMDB when known.
func uniqueIDs(d movieResponse) []nfo.UniqueID {
	ids := []nfo.UniqueID{
		{Type: "tmdb", Default: true, Value: strconv.Itoa(d.ID)},
	}
	if imdb := imdbID(d); imdb != "" {
		ids = append(ids, nfo.UniqueID{Type: "imdb", Value: imdb})
	}
	return ids
}

// imdbID prefers the top-level imdb_id, falling back to external_ids.
func imdbID(d movieResponse) string {
	if d.IMDBID != "" {
		return d.IMDBID
	}
	return d.ExternalIDs.IMDBID
}

// certification returns the US theatrical certification (MPAA), preferring a
// type-3 (theatrical) entry but accepting any non-empty US certification.
func certification(rd releaseDates) string {
	for _, country := range rd.Results {
		if country.Country != "US" {
			continue
		}
		var fallback string
		for _, rel := range country.Releases {
			if rel.Certification == "" {
				continue
			}
			if rel.Type == 3 {
				return rel.Certification
			}
			if fallback == "" {
				fallback = rel.Certification
			}
		}
		return fallback
	}
	return ""
}

// trailer returns a Kodi YouTube-plugin URL for the best YouTube trailer (a
// "Trailer"-typed video preferred over any other YouTube video), or "".
func trailer(v videos) string {
	var fallback string
	for _, vid := range v.Results {
		if vid.Site != "YouTube" || vid.Key == "" {
			continue
		}
		if vid.Type == "Trailer" {
			return youTubePluginURL(vid.Key)
		}
		if fallback == "" {
			fallback = vid.Key
		}
	}
	if fallback == "" {
		return ""
	}
	return youTubePluginURL(fallback)
}

// youTubePluginURL builds the Kodi youtube-plugin play URL MediaElch uses.
func youTubePluginURL(key string) string {
	return "plugin://plugin.video.youtube/?action=play_video&videoid=" + key
}

// imageURL turns a TMDB image path into a full CDN URL, or "" when absent.
func imageURL(path string) string {
	if path == "" {
		return ""
	}
	return imageBaseURL + path
}
