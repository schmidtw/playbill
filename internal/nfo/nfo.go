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

// Movie is the canonical movie model marshaled into an NFO.
type Movie struct {
	Title         string
	Year          int
	StreamDetails *StreamDetails
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
	XMLName  xml.Name     `xml:"movie"`
	Title    string       `xml:"title"`
	Year     int          `xml:"year"`
	FileInfo *fileInfoXML `xml:"fileinfo,omitempty"`
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
		Title:    m.Title,
		Year:     m.Year,
		FileInfo: fileInfo(m.StreamDetails),
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
