// Package probe reads technical Stream Details (video codec/resolution/scan
// type, audio tracks/languages/channels, duration) directly from a movie's
// video file, with no external program — see ADR-0002.
//
// Probing is pure Go. v1 understands MP4/M4V and MKV/Matroska containers; any
// other container returns ErrUnsupportedContainer so the caller can skip it and
// report it rather than failing the run.
package probe

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	mp4 "github.com/abema/go-mp4"
)

// ErrUnsupportedContainer signals that the file's container is not one this
// package can probe. Callers should skip stream details and report it, not
// abort the run.
var ErrUnsupportedContainer = errors.New("probe: unsupported container")

// StreamDetails is the technical media facts written into the NFO's
// <fileinfo><streamdetails>.
type StreamDetails struct {
	Video VideoStream
	Audio []AudioStream
}

// VideoStream describes the single video track.
type VideoStream struct {
	Codec             string
	Width             int
	Height            int
	Aspect            float64
	ScanType          string
	DurationInSeconds int
}

// AudioStream describes one audio track.
type AudioStream struct {
	Codec    string
	Language string
	Channels int
}

// Prober probes a single video file for its Stream Details.
type Prober interface {
	Probe(path string) (StreamDetails, error)
}

// Probe selects a Prober by file extension and probes path. It returns
// ErrUnsupportedContainer for containers no Prober handles.
func Probe(path string) (StreamDetails, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".m4v":
		return MP4Prober{}.Probe(path)
	case ".mkv":
		return MKVProber{}.Probe(path)
	default:
		return StreamDetails{}, ErrUnsupportedContainer
	}
}

// MP4Prober probes MP4/M4V files using a pure-Go parser.
type MP4Prober struct{}

// Probe reads path and returns its Stream Details.
func (MP4Prober) Probe(path string) (StreamDetails, error) {
	f, err := os.Open(path)
	if err != nil {
		return StreamDetails{}, err
	}
	defer func() { _ = f.Close() }()

	info, err := mp4.Probe(f)
	if err != nil {
		return StreamDetails{}, err
	}

	langByTrackID, err := languagesByTrackID(f)
	if err != nil {
		return StreamDetails{}, err
	}

	var sd StreamDetails
	for _, tr := range info.Tracks {
		switch tr.Codec {
		case mp4.CodecAVC1:
			sd.Video = videoStream(tr)
		case mp4.CodecMP4A:
			sd.Audio = append(sd.Audio, audioStream(tr, langByTrackID))
		}
	}

	return sd, nil
}

// videoStream builds the VideoStream for an H.264 track. MP4 carries no reliable
// interlace flag in the boxes we read, so scan type is reported as progressive.
func videoStream(tr *mp4.Track) VideoStream {
	v := VideoStream{
		Codec:             "h264",
		ScanType:          "progressive",
		DurationInSeconds: durationSeconds(tr.Duration, tr.Timescale),
	}
	if tr.AVC != nil {
		v.Width = int(tr.AVC.Width)
		v.Height = int(tr.AVC.Height)
		if tr.AVC.Height != 0 {
			v.Aspect = float64(tr.AVC.Width) / float64(tr.AVC.Height)
		}
	}
	return v
}

func audioStream(tr *mp4.Track, langByTrackID map[uint32]string) AudioStream {
	a := AudioStream{
		Codec:    "aac",
		Language: langByTrackID[tr.TrackID],
	}
	if tr.MP4A != nil {
		a.Channels = int(tr.MP4A.ChannelCount)
	}
	return a
}

func durationSeconds(duration uint64, timescale uint32) int {
	if timescale == 0 {
		return 0
	}
	return int(duration / uint64(timescale))
}

// languagesByTrackID walks each trak to pair its track ID (tkhd) with its media
// language (mdhd). The mdhd language is a packed ISO-639-2/T code; each byte is
// a 5-bit value offset by 0x60 from its ASCII letter.
func languagesByTrackID(f *os.File) (map[uint32]string, error) {
	traks, err := mp4.ExtractBox(f, nil, mp4.BoxPath{mp4.BoxTypeMoov(), mp4.BoxTypeTrak()})
	if err != nil {
		return nil, err
	}

	langs := make(map[uint32]string, len(traks))
	for _, trak := range traks {
		tkhds, err := mp4.ExtractBoxWithPayload(f, trak, mp4.BoxPath{mp4.BoxTypeTkhd()})
		if err != nil {
			return nil, err
		}
		mdhds, err := mp4.ExtractBoxWithPayload(f, trak, mp4.BoxPath{mp4.BoxTypeMdia(), mp4.BoxTypeMdhd()})
		if err != nil {
			return nil, err
		}
		if len(tkhds) == 0 || len(mdhds) == 0 {
			continue
		}
		trackID := tkhds[0].Payload.(*mp4.Tkhd).TrackID
		langs[trackID] = decodeLanguage(mdhds[0].Payload.(*mp4.Mdhd).Language)
	}

	return langs, nil
}

func decodeLanguage(packed [3]byte) string {
	var b [3]byte
	for i, v := range packed {
		b[i] = v + 0x60
	}
	return string(b[:])
}
