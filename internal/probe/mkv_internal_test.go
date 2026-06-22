package probe

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These white-box tests build tiny in-memory Matroska byte streams and drive
// them through MKVProber, varying codecs and edge cases that committing a binary
// fixture per case would not. They assert externally observable StreamDetails,
// not internal call sequences.

func TestMatroskaVideoCodec(t *testing.T) {
	cases := map[string]string{
		"V_MPEG4/ISO/AVC":  "h264",
		"V_MPEGH/ISO/HEVC": "hevc",
		"V_AV1":            "av1",
		"V_VP9":            "vp9",
		"V_VP8":            "vp8",
		"V_MPEG4/ISO/ASP":  "mpeg4",
		"V_MPEG2":          "mpeg2video",
		"V_THEORA":         "theora",
		"V_REAL/RV40":      "real",
	}
	for in, want := range cases {
		assert.Equalf(t, want, matroskaVideoCodec(in), "video codec %q", in)
	}
}

func TestMatroskaAudioCodec(t *testing.T) {
	cases := map[string]string{
		"A_AAC":         "aac",
		"A_AAC/MPEG4":   "aac",
		"A_AC3":         "ac3",
		"A_EAC3":        "eac3",
		"A_DTS":         "dts",
		"A_DTS/EXPRESS": "dts",
		"A_MPEG/L3":     "mp3",
		"A_TRUEHD":      "truehd",
		"A_FLAC":        "flac",
		"A_OPUS":        "opus",
		"A_VORBIS":      "vorbis",
		"A_PCM/INT/LIT": "pcm",
		"A_REAL/COOK":   "real",
	}
	for in, want := range cases {
		assert.Equalf(t, want, matroskaAudioCodec(in), "audio codec %q", in)
	}
}

// TestMKVProber_HEVCInterlacedMultiAudio covers an HEVC video track flagged
// interlaced, a 32-bit float duration, two audio tracks (one defaulting its
// language when omitted), and a second video track that must be ignored.
func TestMKVProber_HEVCInterlacedMultiAudio(t *testing.T) {
	video := tMaster(idTrackEntry,
		tUint(idTrackType, 1),
		tStr(idCodecID, "V_MPEGH/ISO/HEVC"),
		tStr(idLanguage, "und"),
		tMaster(idVideo,
			tUint(idPixelWidth, 1920),
			tUint(idPixelHeight, 1080),
			tUint(idFlagInterlaced, 1),
		),
	)
	video2 := tMaster(idTrackEntry, // second video track — ignored
		tUint(idTrackType, 1),
		tStr(idCodecID, "V_VP9"),
		tMaster(idVideo, tUint(idPixelWidth, 640), tUint(idPixelHeight, 480)),
	)
	audioEng := tMaster(idTrackEntry,
		tUint(idTrackType, 2),
		tStr(idCodecID, "A_AC3"),
		tStr(idLanguage, "eng"),
		tMaster(idAudio, tUint(idChannels, 6)),
	)
	audioNoLang := tMaster(idTrackEntry, // language element omitted
		tUint(idTrackType, 2),
		tStr(idCodecID, "A_DTS"),
		tMaster(idAudio, tUint(idChannels, 2)),
	)

	info := tMaster(idInfo,
		tUint(idTimestampScale, 1000000),
		tFloat32(idDuration, 7200000.0), // 7,200,000 ms ticks → 7200s
	)
	tracks := tMaster(idTracks, video, video2, audioEng, audioNoLang)
	data := withHeader(tMaster(idSegment, info, tracks))

	sd := probeBytes(t, data)

	assert.Equal(t, "hevc", sd.Video.Codec)
	assert.Equal(t, 1920, sd.Video.Width)
	assert.Equal(t, 1080, sd.Video.Height)
	assert.Equal(t, "interlaced", sd.Video.ScanType)
	assert.Equal(t, 7200, sd.Video.DurationInSeconds)

	require.Len(t, sd.Audio, 2)
	assert.Equal(t, "ac3", sd.Audio[0].Codec)
	assert.Equal(t, "eng", sd.Audio[0].Language)
	assert.Equal(t, 6, sd.Audio[0].Channels)
	assert.Equal(t, "dts", sd.Audio[1].Codec)
	assert.Equal(t, "", sd.Audio[1].Language)
	assert.Equal(t, 2, sd.Audio[1].Channels)
}

// TestMKVProber_DefaultTimestampScaleNoDuration covers the Info block omitting
// both TimestampScale and Duration: duration is reported as zero, not an error.
func TestMKVProber_DefaultTimestampScaleNoDuration(t *testing.T) {
	video := tMaster(idTrackEntry,
		tUint(idTrackType, 1),
		tStr(idCodecID, "V_MPEG4/ISO/AVC"),
		tMaster(idVideo, tUint(idPixelWidth, 720), tUint(idPixelHeight, 480)),
	)
	data := withHeader(tMaster(idSegment,
		tMaster(idInfo), // empty Info
		tMaster(idTracks, video),
	))

	sd := probeBytes(t, data)

	assert.Equal(t, "h264", sd.Video.Codec)
	assert.Equal(t, 0, sd.Video.DurationInSeconds)
}

// TestMKVProber_NoSegment covers a well-formed EBML file with no Segment: the
// container is reported as unsupported rather than producing bogus details.
func TestMKVProber_NoSegment(t *testing.T) {
	data := withHeader(nil)
	_, err := probeBytes2(t, data)
	assert.ErrorIs(t, err, ErrUnsupportedContainer)
}

func probeBytes(t *testing.T, data []byte) StreamDetails {
	t.Helper()
	sd, err := probeBytes2(t, data)
	require.NoError(t, err)
	return sd
}

func probeBytes2(t *testing.T, data []byte) (StreamDetails, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "in.mkv")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return MKVProber{}.Probe(path)
}

// --- minimal EBML encoder used only by these tests ---

// test-only element IDs not needed by the production reader.
const (
	idEBMLHeader  = 0x1A45DFA3
	idTestDocType = 0x4282
)

// withHeader prepends a tiny EBML header (a non-Segment top-level element the
// reader must skip) to the given Segment bytes.
func withHeader(segment []byte) []byte {
	header := tMaster(idEBMLHeader, tStr(idTestDocType, "matroska"))
	return append(header, segment...)
}

func tMaster(id uint32, children ...[]byte) []byte {
	var body []byte
	for _, c := range children {
		body = append(body, c...)
	}
	return tEl(id, body)
}

func tEl(id uint32, payload []byte) []byte {
	out := encID(id)
	out = append(out, encSize(uint64(len(payload)))...)
	return append(out, payload...)
}

func tUint(id uint32, v uint64) []byte { return tEl(id, encUint(v)) }
func tStr(id uint32, s string) []byte  { return tEl(id, []byte(s)) }

func tFloat32(id uint32, v float32) []byte {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], math.Float32bits(v))
	return tEl(id, buf[:])
}

func encID(id uint32) []byte {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], id)
	for i := range 4 {
		if buf[i] != 0 {
			return buf[i:]
		}
	}
	return buf[3:]
}

func encSize(n uint64) []byte {
	length := 1
	for length < 8 && n >= (uint64(1)<<(7*length))-1 {
		length++
	}
	v := n | (uint64(1) << (7 * length))
	buf := make([]byte, length)
	for i := length - 1; i >= 0; i-- {
		buf[i] = byte(v)
		v >>= 8
	}
	return buf
}

func encUint(v uint64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], v)
	trimmed := bytes.TrimLeft(buf[:], "\x00")
	if len(trimmed) == 0 {
		return []byte{0}
	}
	return trimmed
}
