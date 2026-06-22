package probe

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
	"strings"
)

// Matroska/EBML element IDs the probe reads (written with their length-descriptor
// marker bits, the canonical form Matroska IDs are referenced in).
const (
	idSegment        = 0x18538067
	idInfo           = 0x1549A966
	idTimestampScale = 0x2AD7B1
	idDuration       = 0x4489
	idTracks         = 0x1654AE6B
	idTrackEntry     = 0xAE
	idTrackType      = 0x83
	idCodecID        = 0x86
	idLanguage       = 0x22B59C
	idVideo          = 0xE0
	idPixelWidth     = 0xB0
	idPixelHeight    = 0xBA
	idFlagInterlaced = 0x9A
	idAudio          = 0xE1
	idChannels       = 0x9F
)

// defaultTimestampScale is Matroska's default when the Info block omits one:
// one timestamp tick equals 1,000,000 ns (1 ms).
const defaultTimestampScale = 1_000_000

// trackInfo accumulates the fields of one Matroska TrackEntry before it is
// classified (by TrackType) into a VideoStream or AudioStream.
type trackInfo struct {
	trackType  uint64
	codecID    string
	language   string
	width      int
	height     int
	interlaced bool
	channels   int
}

// MKVProber probes Matroska (.mkv) files with a pure-Go EBML reader.
type MKVProber struct{}

// Probe reads path and returns its Stream Details.
func (MKVProber) Probe(path string) (StreamDetails, error) {
	f, err := os.Open(path)
	if err != nil {
		return StreamDetails{}, err
	}
	defer func() { _ = f.Close() }()

	size, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return StreamDetails{}, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return StreamDetails{}, err
	}

	p := &ebmlReader{r: f, pos: 0, fileSize: size}

	// Walk top-level elements (EBML header, Segment) and parse the Segment.
	var sd StreamDetails
	timestampScale := uint64(defaultTimestampScale)
	durationTicks := 0.0
	var found bool
	err = p.walk(size, func(id uint32, bodyStart, bodySize int64) error {
		if id != idSegment {
			return nil
		}
		found = true
		return p.walk(bodyStart+bodySize, func(id uint32, bodyStart, bodySize int64) error {
			switch id {
			case idInfo:
				return p.walk(bodyStart+bodySize, func(id uint32, bodyStart, bodySize int64) error {
					switch id {
					case idTimestampScale:
						v, err := p.readUint(bodySize)
						if err == nil && v != 0 {
							timestampScale = v
						}
						return err
					case idDuration:
						v, err := p.readFloat(bodySize)
						durationTicks = v
						return err
					}
					return nil
				})
			case idTracks:
				tracks, err := p.parseTracks(bodyStart + bodySize)
				if err != nil {
					return err
				}
				applyTracks(&sd, tracks)
			}
			return nil
		})
	})
	if err != nil {
		return StreamDetails{}, err
	}
	if !found {
		return StreamDetails{}, ErrUnsupportedContainer
	}

	if sd.Video.Codec != "" {
		sd.Video.DurationInSeconds = durationSecondsFromTicks(durationTicks, timestampScale)
	}
	return sd, nil
}

// parseTracks reads each TrackEntry up to end and returns the per-track facts.
func (p *ebmlReader) parseTracks(end int64) ([]trackInfo, error) {
	var tracks []trackInfo
	err := p.walk(end, func(id uint32, bodyStart, bodySize int64) error {
		if id != idTrackEntry {
			return nil
		}
		t, err := p.parseTrackEntry(bodyStart + bodySize)
		if err != nil {
			return err
		}
		tracks = append(tracks, t)
		return nil
	})
	return tracks, err
}

// parseTrackEntry reads one TrackEntry's children up to end.
func (p *ebmlReader) parseTrackEntry(end int64) (trackInfo, error) {
	var t trackInfo
	err := p.walk(end, func(id uint32, bodyStart, bodySize int64) error {
		switch id {
		case idTrackType:
			v, err := p.readUint(bodySize)
			t.trackType = v
			return err
		case idCodecID:
			s, err := p.readString(bodySize)
			t.codecID = s
			return err
		case idLanguage:
			s, err := p.readString(bodySize)
			t.language = s
			return err
		case idVideo:
			return p.walk(bodyStart+bodySize, func(id uint32, _, bodySize int64) error {
				switch id {
				case idPixelWidth:
					v, err := p.readUint(bodySize)
					t.width = int(v)
					return err
				case idPixelHeight:
					v, err := p.readUint(bodySize)
					t.height = int(v)
					return err
				case idFlagInterlaced:
					v, err := p.readUint(bodySize)
					t.interlaced = v == 1 // 1=interlaced, 2=progressive, 0=undetermined
					return err
				}
				return nil
			})
		case idAudio:
			return p.walk(bodyStart+bodySize, func(id uint32, _, bodySize int64) error {
				if id == idChannels {
					v, err := p.readUint(bodySize)
					t.channels = int(v)
					return err
				}
				return nil
			})
		}
		return nil
	})
	return t, err
}

// applyTracks classifies parsed tracks into the StreamDetails. The first video
// track becomes the single VideoStream; audio tracks accumulate in order.
func applyTracks(sd *StreamDetails, tracks []trackInfo) {
	for _, t := range tracks {
		switch t.trackType {
		case 1: // video
			if sd.Video.Codec != "" {
				continue
			}
			v := VideoStream{
				Codec:    matroskaVideoCodec(t.codecID),
				Width:    t.width,
				Height:   t.height,
				ScanType: "progressive",
			}
			if t.interlaced {
				v.ScanType = "interlaced"
			}
			if t.height != 0 {
				v.Aspect = float64(t.width) / float64(t.height)
			}
			sd.Video = v
		case 2: // audio
			sd.Audio = append(sd.Audio, AudioStream{
				Codec:    matroskaAudioCodec(t.codecID),
				Language: t.language,
				Channels: t.channels,
			})
		}
	}
}

// matroskaVideoCodec maps a Matroska CodecID to the short codec name written in
// the NFO. Unknown codecs fall back to a cleaned, lower-cased form.
func matroskaVideoCodec(codecID string) string {
	switch {
	case strings.HasPrefix(codecID, "V_MPEG4/ISO/AVC"):
		return "h264"
	case strings.Contains(codecID, "HEVC"):
		return "hevc"
	case strings.Contains(codecID, "AV1"):
		return "av1"
	case strings.Contains(codecID, "VP9"):
		return "vp9"
	case strings.Contains(codecID, "VP8"):
		return "vp8"
	case strings.HasPrefix(codecID, "V_MPEG4"):
		return "mpeg4"
	case strings.HasPrefix(codecID, "V_MPEG2"):
		return "mpeg2video"
	}
	return fallbackCodec(codecID)
}

// matroskaAudioCodec maps a Matroska audio CodecID to the short codec name.
func matroskaAudioCodec(codecID string) string {
	switch {
	case strings.HasPrefix(codecID, "A_AAC"):
		return "aac"
	case strings.HasPrefix(codecID, "A_AC3"):
		return "ac3"
	case strings.HasPrefix(codecID, "A_EAC3"):
		return "eac3"
	case strings.HasPrefix(codecID, "A_DTS"):
		return "dts"
	case codecID == "A_MPEG/L3":
		return "mp3"
	case strings.HasPrefix(codecID, "A_TRUEHD"):
		return "truehd"
	case strings.HasPrefix(codecID, "A_FLAC"):
		return "flac"
	case strings.HasPrefix(codecID, "A_OPUS"):
		return "opus"
	case strings.HasPrefix(codecID, "A_VORBIS"):
		return "vorbis"
	case strings.HasPrefix(codecID, "A_PCM"):
		return "pcm"
	}
	return fallbackCodec(codecID)
}

// fallbackCodec turns an unrecognized "X_FOO/BAR" CodecID into "foo" — the
// leading track-type tag dropped, lower-cased, first path segment only.
func fallbackCodec(codecID string) string {
	s := codecID
	if _, rest, ok := strings.Cut(s, "_"); ok {
		s = rest
	}
	s, _, _ = strings.Cut(s, "/")
	return strings.ToLower(s)
}

// durationSecondsFromTicks converts a Matroska Duration (in timestamp ticks) to
// whole seconds using the timestamp scale (nanoseconds per tick).
func durationSecondsFromTicks(ticks float64, timestampScale uint64) int {
	if ticks <= 0 {
		return 0
	}
	return int(ticks*float64(timestampScale)/1e9 + 0.5)
}

// ebmlReader is a minimal forward EBML element reader over a seekable source. It
// reads only the fields the probe needs and seeks past everything else (notably
// the large Cluster data), so it never loads media samples into memory.
type ebmlReader struct {
	r        io.ReadSeeker
	pos      int64
	fileSize int64
}

var errInvalidEBML = errors.New("probe: invalid EBML")

// walk iterates sibling elements from the current position up to end, invoking
// fn with each element's ID and body extent. After fn returns, the reader is
// positioned at the next sibling regardless of how much of the body fn consumed.
func (p *ebmlReader) walk(end int64, fn func(id uint32, bodyStart, bodySize int64) error) error {
	for p.pos < end {
		id, err := p.readID()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		size, unknown, err := p.readSize()
		if err != nil {
			return err
		}

		bodyStart := p.pos
		bodySize := int64(size)
		if unknown || bodyStart+bodySize > end {
			// Unknown or oversized length: treat the body as running to the
			// parent's end so a truncated or streaming element can't overrun.
			bodySize = end - bodyStart
		}

		if err := fn(id, bodyStart, bodySize); err != nil {
			return err
		}

		if err := p.seekTo(bodyStart + bodySize); err != nil {
			return err
		}
	}
	return nil
}

// readID reads an EBML element ID, returning it with its marker bits intact.
func (p *ebmlReader) readID() (uint32, error) {
	var first [1]byte
	if _, err := io.ReadFull(p.r, first[:]); err != nil {
		return 0, err
	}
	p.pos++

	b0 := first[0]
	length := 1
	for mask := byte(0x80); mask != 0 && b0&mask == 0; mask >>= 1 {
		length++
	}
	if length > 4 {
		return 0, errInvalidEBML
	}

	id := uint32(b0)
	for range length - 1 {
		var b [1]byte
		if _, err := io.ReadFull(p.r, b[:]); err != nil {
			return 0, err
		}
		p.pos++
		id = id<<8 | uint32(b[0])
	}
	return id, nil
}

// readSize reads an EBML VINT data size, stripping the length-descriptor marker
// bit. The bool reports an "unknown size" sentinel (all data bits set).
func (p *ebmlReader) readSize() (uint64, bool, error) {
	var first [1]byte
	if _, err := io.ReadFull(p.r, first[:]); err != nil {
		return 0, false, err
	}
	p.pos++

	b0 := first[0]
	length := 1
	mask := byte(0x80)
	for mask != 0 && b0&mask == 0 {
		mask >>= 1
		length++
	}
	if mask == 0 || length > 8 {
		return 0, false, errInvalidEBML
	}

	value := uint64(b0 &^ mask)
	for range length - 1 {
		var b [1]byte
		if _, err := io.ReadFull(p.r, b[:]); err != nil {
			return 0, false, err
		}
		p.pos++
		value = value<<8 | uint64(b[0])
	}

	unknown := value == (uint64(1)<<(7*length))-1
	return value, unknown, nil
}

// readUint reads a big-endian Matroska unsigned integer of n bytes.
func (p *ebmlReader) readUint(n int64) (uint64, error) {
	if n <= 0 || n > 8 {
		return 0, errInvalidEBML
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(p.r, buf); err != nil {
		return 0, err
	}
	p.pos += n
	var v uint64
	for _, b := range buf {
		v = v<<8 | uint64(b)
	}
	return v, nil
}

// readFloat reads a 4- or 8-byte big-endian IEEE-754 Matroska float.
func (p *ebmlReader) readFloat(n int64) (float64, error) {
	buf := make([]byte, n)
	if _, err := io.ReadFull(p.r, buf); err != nil {
		return 0, err
	}
	p.pos += n
	switch n {
	case 4:
		return float64(math.Float32frombits(binary.BigEndian.Uint32(buf))), nil
	case 8:
		return math.Float64frombits(binary.BigEndian.Uint64(buf)), nil
	default:
		return 0, errInvalidEBML
	}
}

// readString reads an n-byte Matroska string, trimming trailing NUL padding.
func (p *ebmlReader) readString(n int64) (string, error) {
	buf := make([]byte, n)
	if _, err := io.ReadFull(p.r, buf); err != nil {
		return "", err
	}
	p.pos += n
	return strings.TrimRight(string(buf), "\x00"), nil
}

// seekTo repositions the reader to an absolute offset.
func (p *ebmlReader) seekTo(off int64) error {
	if off == p.pos {
		return nil
	}
	if _, err := p.r.Seek(off, io.SeekStart); err != nil {
		return err
	}
	p.pos = off
	return nil
}
