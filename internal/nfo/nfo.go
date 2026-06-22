// Package nfo marshals a canonical Movie model into Kodi NFO XML bytes.
//
// This is the walking-skeleton minimum — title and year only. The NFO grows to
// MediaElch-style richness (cast, ratings, art catalog, stream details) in
// later slices; see CONTEXT.md and the PRD.
package nfo

import (
	"bytes"
	"encoding/xml"
	"strconv"
)

// xmlHeader matches the MediaElch/Kodi-style declaration (standalone="yes").
const xmlHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"

// Movie is the canonical movie model marshaled into an NFO. It is modeled on a
// MediaElch/Kodi-21 NFO for richness; Kodi-owned runtime fields (playcount,
// lastplayed, resume, userrating) are deliberately absent so the tool never
// fights Kodi over playback state.
type Movie struct {
	Title         string
	OriginalTitle string
	SortTitle     string
	Year          int
	Premiered     string // ISO date, e.g. "1999-03-30"
	Runtime       int    // minutes
	Plot          string
	Tagline       string
	MPAA          string // certification, e.g. "R"
	Genres        []string
	Countries     []string
	Studios       []string
	Set           string // collection name
	Directors     []string
	Writers       []string
	Ratings       []Rating
	Actors        []Actor
	Trailer       string
	UniqueIDs     []UniqueID
	StreamDetails *StreamDetails
	// Posters is the full set of poster candidate URLs from every provider,
	// embedded as <thumb aspect="poster"> so Kodi's offline "Choose art" UI can
	// switch posters without re-scraping. It is the full catalog, independent of
	// which single poster is downloaded to disk.
	Posters []string
	// Fanarts is the full set of fanart/backdrop candidate URLs from every
	// provider, embedded under <fanart>. Like Posters it is the full catalog,
	// independent of the single fanart written to disk.
	Fanarts []string
}

// Rating is one scored rating, e.g. TMDB's. Exactly one should be Default.
type Rating struct {
	Name    string
	Max     int
	Default bool
	Value   float64
	Votes   int
}

// Actor is one cast member. Thumb is a URL (cast images are referenced, not
// downloaded). Order is the billing position, starting at zero.
type Actor struct {
	Name  string
	Role  string
	Order int
	Thumb string
}

// UniqueID is a provider identity written as <uniqueid type="..."> in the NFO.
// Exactly one should be marked Default; its value is also mirrored into the
// legacy <id> element for older Kodi skins.
type UniqueID struct {
	Type    string
	Default bool
	Value   string
}

// StreamDetails holds the technical media facts read from the video file. It
// maps to the NFO's <fileinfo><streamdetails> block.
type StreamDetails struct {
	Video *VideoStream
	Audio []AudioStream
}

// VideoStream describes the single video track.
type VideoStream struct {
	Codec             string
	Aspect            float64
	Width             int
	Height            int
	DurationInSeconds int
	ScanType          string
}

// AudioStream describes one audio track.
type AudioStream struct {
	Codec    string
	Language string
	Channels int
}

// movieXML is the on-disk <movie> element. Element order is fixed so output is
// deterministic and golden-file testable.
type movieXML struct {
	XMLName       xml.Name      `xml:"movie"`
	Title         string        `xml:"title"`
	OriginalTitle string        `xml:"originaltitle,omitempty"`
	SortTitle     string        `xml:"sorttitle,omitempty"`
	Year          int           `xml:"year"`
	Ratings       *ratingsXML   `xml:"ratings,omitempty"`
	Plot          string        `xml:"plot,omitempty"`
	Tagline       string        `xml:"tagline,omitempty"`
	Runtime       int           `xml:"runtime,omitempty"`
	MPAA          string        `xml:"mpaa,omitempty"`
	Premiered     string        `xml:"premiered,omitempty"`
	Genres        []string      `xml:"genre"`
	Countries     []string      `xml:"country"`
	Studios       []string      `xml:"studio"`
	Set           *setXML       `xml:"set,omitempty"`
	Credits       []string      `xml:"credits"`
	Directors     []string      `xml:"director"`
	Actors        []actorXML    `xml:"actor"`
	Trailer       string        `xml:"trailer,omitempty"`
	Posters       []thumbXML    `xml:"thumb"`
	Fanart        *fanartXML    `xml:"fanart,omitempty"`
	UniqueIDs     []uniqueIDXML `xml:"uniqueid"`
	ID            string        `xml:"id,omitempty"`
	FileInfo      *fileInfoXML  `xml:"fileinfo,omitempty"`
}

type ratingsXML struct {
	Ratings []ratingXML `xml:"rating"`
}

type ratingXML struct {
	Name    string `xml:"name,attr"`
	Max     int    `xml:"max,attr"`
	Default string `xml:"default,attr,omitempty"`
	Value   string `xml:"value"`
	Votes   int    `xml:"votes"`
}

type setXML struct {
	Name string `xml:"name"`
}

type actorXML struct {
	Name  string `xml:"name"`
	Role  string `xml:"role"`
	Order int    `xml:"order"`
	Thumb string `xml:"thumb,omitempty"`
}

// thumbXML is a single artwork URL. As a top-level <thumb aspect="poster"> it
// is one poster candidate; nested inside <fanart> the aspect is omitted and it
// is one fanart candidate.
type thumbXML struct {
	Aspect string `xml:"aspect,attr,omitempty"`
	URL    string `xml:",chardata"`
}

// fanartXML is the <fanart> element holding every backdrop candidate.
type fanartXML struct {
	Thumbs []thumbXML `xml:"thumb"`
}

type uniqueIDXML struct {
	Type    string `xml:"type,attr"`
	Default string `xml:"default,attr,omitempty"`
	Value   string `xml:",chardata"`
}

type fileInfoXML struct {
	StreamDetails streamDetailsXML `xml:"streamdetails"`
}

type streamDetailsXML struct {
	Video *videoXML  `xml:"video"`
	Audio []audioXML `xml:"audio"`
}

type videoXML struct {
	Codec             string `xml:"codec"`
	Aspect            string `xml:"aspect"`
	Width             int    `xml:"width"`
	Height            int    `xml:"height"`
	DurationInSeconds int    `xml:"durationinseconds"`
	ScanType          string `xml:"scantype"`
}

type audioXML struct {
	Codec    string `xml:"codec"`
	Language string `xml:"language"`
	Channels int    `xml:"channels"`
}

// Marshal renders the movie as Kodi NFO XML, indented two spaces and terminated
// with a trailing newline.
func Marshal(m Movie) ([]byte, error) {
	doc := movieXML{
		Title:         m.Title,
		OriginalTitle: m.OriginalTitle,
		SortTitle:     m.SortTitle,
		Year:          m.Year,
		Ratings:       ratings(m.Ratings),
		Plot:          m.Plot,
		Tagline:       m.Tagline,
		Runtime:       m.Runtime,
		MPAA:          m.MPAA,
		Premiered:     m.Premiered,
		Genres:        m.Genres,
		Countries:     m.Countries,
		Studios:       m.Studios,
		Set:           set(m.Set),
		Credits:       m.Writers,
		Directors:     m.Directors,
		Actors:        actors(m.Actors),
		Trailer:       m.Trailer,
		Posters:       posters(m.Posters),
		Fanart:        fanartList(m.Fanarts),
		UniqueIDs:     uniqueIDs(m.UniqueIDs),
		ID:            legacyID(m.UniqueIDs),
		FileInfo:      fileInfo(m.StreamDetails),
	}

	body, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	buf.WriteString(xmlHeader)
	buf.Write(body)
	buf.WriteByte('\n')

	return buf.Bytes(), nil
}

// ratings maps the canonical ratings to their XML form, or nil when there are
// none (so <ratings> is omitted entirely). Values use the minimal decimal
// representation so output is deterministic.
func ratings(rs []Rating) *ratingsXML {
	if len(rs) == 0 {
		return nil
	}
	out := &ratingsXML{Ratings: make([]ratingXML, 0, len(rs))}
	for _, r := range rs {
		x := ratingXML{
			Name:  r.Name,
			Max:   r.Max,
			Value: strconv.FormatFloat(r.Value, 'f', -1, 64),
			Votes: r.Votes,
		}
		if r.Default {
			x.Default = "true"
		}
		out.Ratings = append(out.Ratings, x)
	}
	return out
}

// posters maps the poster catalog URLs to top-level <thumb aspect="poster">
// elements. An empty catalog yields no elements.
func posters(urls []string) []thumbXML {
	out := make([]thumbXML, 0, len(urls))
	for _, u := range urls {
		out = append(out, thumbXML{Aspect: "poster", URL: u})
	}
	return out
}

// fanartList maps the fanart catalog URLs to a <fanart> element of nested
// <thumb> entries, or nil when empty so <fanart> is omitted entirely.
func fanartList(urls []string) *fanartXML {
	if len(urls) == 0 {
		return nil
	}
	out := &fanartXML{Thumbs: make([]thumbXML, 0, len(urls))}
	for _, u := range urls {
		out.Thumbs = append(out.Thumbs, thumbXML{URL: u})
	}
	return out
}

// set maps a collection name to its XML form, or nil when empty.
func set(name string) *setXML {
	if name == "" {
		return nil
	}
	return &setXML{Name: name}
}

// actors maps the canonical cast to its XML form.
func actors(as []Actor) []actorXML {
	out := make([]actorXML, 0, len(as))
	for _, a := range as {
		out = append(out, actorXML(a))
	}
	return out
}

// uniqueIDs maps the canonical unique IDs to their XML form, rendering the
// default="true" attribute only for the default entry.
func uniqueIDs(ids []UniqueID) []uniqueIDXML {
	out := make([]uniqueIDXML, 0, len(ids))
	for _, id := range ids {
		x := uniqueIDXML{Type: id.Type, Value: id.Value}
		if id.Default {
			x.Default = "true"
		}
		out = append(out, x)
	}
	return out
}

// legacyID returns the value of the default unique ID, mirrored into the legacy
// <id> element. It is empty when no unique ID is marked default.
func legacyID(ids []UniqueID) string {
	for _, id := range ids {
		if id.Default {
			return id.Value
		}
	}
	return ""
}

// TMDBID extracts the <uniqueid type="tmdb"> value from existing NFO bytes. It
// returns ok=false when the data is not parseable NFO, has no tmdb unique id, or
// the tmdb value is empty. Callers use this to trust a prior (possibly
// hand-corrected) TMDB match and short-circuit a fresh search.
func TMDBID(data []byte) (id string, ok bool) {
	var doc struct {
		UniqueIDs []uniqueIDXML `xml:"uniqueid"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return "", false
	}
	for _, u := range doc.UniqueIDs {
		if u.Type == "tmdb" && u.Value != "" {
			return u.Value, true
		}
	}
	return "", false
}

// fileInfo maps the canonical StreamDetails to its XML form, or nil when there
// are none (so <fileinfo> is omitted entirely).
func fileInfo(sd *StreamDetails) *fileInfoXML {
	if sd == nil {
		return nil
	}

	out := &fileInfoXML{}
	if sd.Video != nil {
		out.StreamDetails.Video = &videoXML{
			Codec:             sd.Video.Codec,
			Aspect:            strconv.FormatFloat(sd.Video.Aspect, 'f', 2, 64),
			Width:             sd.Video.Width,
			Height:            sd.Video.Height,
			DurationInSeconds: sd.Video.DurationInSeconds,
			ScanType:          sd.Video.ScanType,
		}
	}
	for _, a := range sd.Audio {
		out.StreamDetails.Audio = append(out.StreamDetails.Audio, audioXML(a))
	}
	return out
}
