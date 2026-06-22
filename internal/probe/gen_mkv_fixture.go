//go:build ignore

// Command gen_mkv_fixture writes the testdata MKV/Matroska fixture used by the
// probe tests. It hand-writes the EBML structure in pure Go — no ffmpeg, no
// third-party library — so the fixture can be regenerated without any external
// program, honoring the single-binary constraint the probe itself keeps (see
// ADR-0002).
//
// Regenerate with:
//
//	go run internal/probe/gen_mkv_fixture.go
//
// The output is a tiny but structurally-complete Matroska file: an EBML header,
// a Segment with an Info block (1s duration) and a Tracks block carrying one
// H.264 video track (320x240) and one AAC stereo audio track tagged English. It
// carries no Clusters/real media samples — just the elements the probe reads.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"
)

const fixturePath = "internal/probe/testdata/sample.mkv"

// Matroska/EBML element IDs (stored with their length-descriptor marker bits,
// the canonical way Matroska IDs are written and referenced).
const (
	idEBML               = 0x1A45DFA3
	idEBMLVersion        = 0x4286
	idEBMLReadVersion    = 0x42F7
	idEBMLMaxIDLength    = 0x42F2
	idEBMLMaxSizeLength  = 0x42F3
	idDocType            = 0x4282
	idDocTypeVersion     = 0x4287
	idDocTypeReadVersion = 0x4285

	idSegment        = 0x18538067
	idInfo           = 0x1549A966
	idTimestampScale = 0x2AD7B1
	idDuration       = 0x4489
	idTracks         = 0x1654AE6B
	idTrackEntry     = 0xAE
	idTrackNumber    = 0xD7
	idTrackType      = 0x83
	idCodecID        = 0x86
	idLanguage       = 0x22B59C
	idVideo          = 0xE0
	idPixelWidth     = 0xB0
	idPixelHeight    = 0xBA
	idAudio          = 0xE1
	idChannels       = 0x9F
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gen_mkv_fixture:", err)
		os.Exit(1)
	}
}

func run() error {
	header := master(idEBML,
		uintElem(idEBMLVersion, 1),
		uintElem(idEBMLReadVersion, 1),
		uintElem(idEBMLMaxIDLength, 4),
		uintElem(idEBMLMaxSizeLength, 8),
		strElem(idDocType, "matroska"),
		uintElem(idDocTypeVersion, 4),
		uintElem(idDocTypeReadVersion, 2),
	)

	info := master(idInfo,
		uintElem(idTimestampScale, 1000000),
		floatElem(idDuration, 1000.0), // 1000 * 1e6 ns = 1s
	)

	videoTrack := master(idTrackEntry,
		uintElem(idTrackNumber, 1),
		uintElem(idTrackType, 1), // video
		strElem(idCodecID, "V_MPEG4/ISO/AVC"),
		strElem(idLanguage, "und"),
		master(idVideo,
			uintElem(idPixelWidth, 320),
			uintElem(idPixelHeight, 240),
		),
	)

	audioTrack := master(idTrackEntry,
		uintElem(idTrackNumber, 2),
		uintElem(idTrackType, 2), // audio
		strElem(idCodecID, "A_AAC"),
		strElem(idLanguage, "eng"),
		master(idAudio,
			uintElem(idChannels, 2),
		),
	)

	tracks := master(idTracks, videoTrack, audioTrack)
	segment := master(idSegment, info, tracks)

	out := append(header, segment...)
	return os.WriteFile(fixturePath, out, 0o644)
}

// master wraps child elements in a master element with the given ID.
func master(id uint32, children ...[]byte) []byte {
	var body []byte
	for _, c := range children {
		body = append(body, c...)
	}
	return element(id, body)
}

// element frames a payload as ID + size (VINT) + payload.
func element(id uint32, payload []byte) []byte {
	out := encodeID(id)
	out = append(out, encodeSize(uint64(len(payload)))...)
	return append(out, payload...)
}

func uintElem(id uint32, v uint64) []byte { return element(id, uintBytes(v)) }
func strElem(id uint32, s string) []byte  { return element(id, []byte(s)) }

func floatElem(id uint32, v float64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], math.Float64bits(v))
	return element(id, buf[:])
}

// encodeID emits the minimal big-endian bytes of an EBML element ID. The IDs
// above already carry their marker bits, so minimal-length encoding round-trips.
func encodeID(id uint32) []byte {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], id)
	for i := range 4 {
		if buf[i] != 0 {
			return buf[i:]
		}
	}
	return buf[3:]
}

// encodeSize emits an EBML variable-length integer (VINT) for an element size,
// setting the length-descriptor marker bit.
func encodeSize(n uint64) []byte {
	length := 1
	for length < 8 && n >= (uint64(1)<<(7*length))-1 {
		length++
	}
	v := n | (uint64(1) << (7 * length)) // set the marker bit
	buf := make([]byte, length)
	for i := length - 1; i >= 0; i-- {
		buf[i] = byte(v)
		v >>= 8
	}
	return buf
}

// uintBytes emits the minimal big-endian bytes of a Matroska unsigned integer
// (at least one byte).
func uintBytes(v uint64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], v)
	trimmed := bytes.TrimLeft(buf[:], "\x00")
	if len(trimmed) == 0 {
		return []byte{0}
	}
	return trimmed
}
