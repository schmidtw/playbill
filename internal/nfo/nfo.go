// Package nfo marshals a canonical Movie model into Kodi NFO XML bytes.
//
// This is the walking-skeleton minimum — title and year only. The NFO grows to
// MediaElch-style richness (cast, ratings, art catalog, stream details) in
// later slices; see CONTEXT.md and the PRD.
package nfo

import (
	"bytes"
	"encoding/xml"
)

// xmlHeader matches the MediaElch/Kodi-style declaration (standalone="yes").
const xmlHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"

// Movie is the canonical movie model marshaled into an NFO.
type Movie struct {
	Title string
	Year  int
}

// movieXML is the on-disk <movie> element. Element order is fixed so output is
// deterministic and golden-file testable.
type movieXML struct {
	XMLName xml.Name `xml:"movie"`
	Title   string   `xml:"title"`
	Year    int      `xml:"year"`
}

// Marshal renders the movie as Kodi NFO XML, indented two spaces and terminated
// with a trailing newline.
func Marshal(m Movie) ([]byte, error) {
	doc := movieXML{
		Title: m.Title,
		Year:  m.Year,
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
