//go:build ignore

// Command gen_fixture writes the testdata MP4/M4V fixture used by the probe
// tests. It is pure Go (uses github.com/abema/go-mp4) so the fixture can be
// regenerated without ffmpeg or any external program — the same single-binary
// constraint the probe itself honors (see ADR-0002).
//
// Regenerate with:
//
//	go run internal/probe/gen_fixture.go
//
// The output is a tiny but structurally-complete MP4 with one H.264 video track
// (320x240) and one AAC stereo audio track tagged English. It carries no real
// media samples — just the box structure the probe reads.
package main

import (
	"fmt"
	"os"

	mp4 "github.com/abema/go-mp4"
)

const fixturePath = "internal/probe/testdata/sample.m4v"

// iso639 packs a 3-letter ISO-639-2/T code into the 5-bit-per-char form go-mp4
// marshals for the mdhd Language field.
func iso639(code string) [3]byte {
	var out [3]byte
	for i := range 3 {
		out[i] = code[i] - 0x60
	}
	return out
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gen_fixture:", err)
		os.Exit(1)
	}
}

func run() error {
	f, err := os.Create(fixturePath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := mp4.NewWriter(f)
	ctx := mp4.Context{}

	leaf := func(typ mp4.BoxType, payload mp4.IImmutableBox) error {
		if _, err := w.StartBox(&mp4.BoxInfo{Type: typ}); err != nil {
			return err
		}
		if _, err := mp4.Marshal(w, payload, ctx); err != nil {
			return err
		}
		_, err := w.EndBox()
		return err
	}
	open := func(typ mp4.BoxType) error {
		_, err := w.StartBox(&mp4.BoxInfo{Type: typ})
		return err
	}
	close := func() error {
		_, err := w.EndBox()
		return err
	}

	// ftyp
	ftyp := &mp4.Ftyp{
		MajorBrand:   [4]byte{'i', 's', 'o', 'm'},
		MinorVersion: 512,
		CompatibleBrands: []mp4.CompatibleBrandElem{
			{CompatibleBrand: [4]byte{'i', 's', 'o', 'm'}},
			{CompatibleBrand: [4]byte{'M', '4', 'V', ' '}},
			{CompatibleBrand: [4]byte{'m', 'p', '4', '2'}},
		},
	}
	if err := leaf(mp4.BoxTypeFtyp(), ftyp); err != nil {
		return err
	}

	// moov
	if err := open(mp4.BoxTypeMoov()); err != nil {
		return err
	}

	mvhd := &mp4.Mvhd{Timescale: 1000, DurationV0: 1000, Rate: 0x00010000, Volume: 0x0100, NextTrackID: 3}
	mvhd.Matrix = unityMatrix()
	if err := leaf(mp4.BoxTypeMvhd(), mvhd); err != nil {
		return err
	}

	// video track
	if err := writeTrack(w, ctx, open, close, leaf, trackSpec{
		trackID:   1,
		handler:   [4]byte{'v', 'i', 'd', 'e'},
		timescale: 24000,
		duration:  24000,
		language:  iso639("und"),
		width:     320,
		height:    240,
		video:     true,
	}); err != nil {
		return err
	}

	// audio track
	if err := writeTrack(w, ctx, open, close, leaf, trackSpec{
		trackID:   2,
		handler:   [4]byte{'s', 'o', 'u', 'n'},
		timescale: 44100,
		duration:  44100,
		language:  iso639("eng"),
		channels:  2,
		video:     false,
	}); err != nil {
		return err
	}

	if err := close(); err != nil { // moov
		return err
	}

	// mdat (placeholder media data; stco offsets point here)
	if _, err := w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeMdat()}); err != nil {
		return err
	}
	if _, err := w.Write(make([]byte, 16)); err != nil {
		return err
	}
	if _, err := w.EndBox(); err != nil {
		return err
	}

	return nil
}

type trackSpec struct {
	trackID            uint32
	handler            [4]byte
	timescale          uint32
	duration           uint32
	language           [3]byte
	width, height      uint16
	channels           uint16
	video              bool
}

func writeTrack(
	w *mp4.Writer,
	ctx mp4.Context,
	open func(mp4.BoxType) error,
	close func() error,
	leaf func(mp4.BoxType, mp4.IImmutableBox) error,
	spec trackSpec,
) error {
	if err := open(mp4.BoxTypeTrak()); err != nil {
		return err
	}

	tkhd := &mp4.Tkhd{TrackID: spec.trackID, DurationV0: 1000}
	tkhd.SetFlags(0x000007) // enabled, in movie, in preview
	tkhd.Matrix = unityMatrix()
	if spec.video {
		tkhd.Width = uint32(spec.width) << 16
		tkhd.Height = uint32(spec.height) << 16
	} else {
		tkhd.Volume = 0x0100
	}
	if err := leaf(mp4.BoxTypeTkhd(), tkhd); err != nil {
		return err
	}

	// mdia
	if err := open(mp4.BoxTypeMdia()); err != nil {
		return err
	}
	mdhd := &mp4.Mdhd{Timescale: spec.timescale, DurationV0: spec.duration, Language: spec.language, PreDefined: 0}
	if err := leaf(mp4.BoxTypeMdhd(), mdhd); err != nil {
		return err
	}
	hdlr := &mp4.Hdlr{HandlerType: spec.handler, Name: "playbill"}
	if err := leaf(mp4.BoxTypeHdlr(), hdlr); err != nil {
		return err
	}

	// minf > stbl
	if err := open(mp4.BoxTypeMinf()); err != nil {
		return err
	}
	if err := open(mp4.BoxTypeStbl()); err != nil {
		return err
	}

	// stsd + sample entry
	if err := open(mp4.BoxTypeStsd()); err != nil {
		return err
	}
	if _, err := mp4.Marshal(w, &mp4.Stsd{EntryCount: 1}, ctx); err != nil {
		return err
	}
	if spec.video {
		if err := writeAvc1(w, ctx, open, close, leaf, spec); err != nil {
			return err
		}
	} else {
		if err := writeMp4a(w, ctx, open, close, spec); err != nil {
			return err
		}
	}
	if err := close(); err != nil { // stsd
		return err
	}

	// minimal sample tables
	if err := leaf(mp4.BoxTypeStts(), &mp4.Stts{EntryCount: 1, Entries: []mp4.SttsEntry{{SampleCount: 1, SampleDelta: spec.timescale}}}); err != nil {
		return err
	}
	if err := leaf(mp4.BoxTypeStsc(), &mp4.Stsc{EntryCount: 1, Entries: []mp4.StscEntry{{FirstChunk: 1, SamplesPerChunk: 1, SampleDescriptionIndex: 1}}}); err != nil {
		return err
	}
	if err := leaf(mp4.BoxTypeStsz(), &mp4.Stsz{SampleSize: 0, SampleCount: 1, EntrySize: []uint32{16}}); err != nil {
		return err
	}
	if err := leaf(mp4.BoxTypeStco(), &mp4.Stco{EntryCount: 1, ChunkOffset: []uint32{0}}); err != nil {
		return err
	}

	if err := close(); err != nil { // stbl
		return err
	}
	if err := close(); err != nil { // minf
		return err
	}
	if err := close(); err != nil { // mdia
		return err
	}
	return close() // trak
}

func writeAvc1(
	w *mp4.Writer,
	ctx mp4.Context,
	open func(mp4.BoxType) error,
	close func() error,
	leaf func(mp4.BoxType, mp4.IImmutableBox) error,
	spec trackSpec,
) error {
	if err := open(mp4.BoxTypeAvc1()); err != nil {
		return err
	}
	vse := &mp4.VisualSampleEntry{
		Width:           spec.width,
		Height:          spec.height,
		Horizresolution: 0x00480000,
		Vertresolution:  0x00480000,
		FrameCount:      1,
		Depth:           0x0018,
		PreDefined3:     -1,
	}
	vse.DataReferenceIndex = 1
	vse.SetType(mp4.BoxTypeAvc1())
	if _, err := mp4.Marshal(w, vse, ctx); err != nil {
		return err
	}
	avcC := &mp4.AVCDecoderConfiguration{
		ConfigurationVersion: 1,
		Profile:              0x42, // baseline
		ProfileCompatibility: 0,
		Level:                0x0d,
		Reserved:             0x3f,
		LengthSizeMinusOne:   3,
		Reserved2:            0x07,
	}
	avcC.SetType(mp4.BoxTypeAvcC())
	if err := leaf(mp4.BoxTypeAvcC(), avcC); err != nil {
		return err
	}
	return close() // avc1
}

func writeMp4a(
	w *mp4.Writer,
	ctx mp4.Context,
	open func(mp4.BoxType) error,
	close func() error,
	spec trackSpec,
) error {
	if err := open(mp4.BoxTypeMp4a()); err != nil {
		return err
	}
	ase := &mp4.AudioSampleEntry{
		ChannelCount: spec.channels,
		SampleSize:   16,
		SampleRate:   uint32(spec.timescale) << 16,
	}
	ase.DataReferenceIndex = 1
	ase.SetType(mp4.BoxTypeMp4a())
	if _, err := mp4.Marshal(w, ase, ctx); err != nil {
		return err
	}
	// Minimal esds — empty descriptor list is enough for the probe to classify
	// the track as AAC (channel count comes from the sample entry).
	esds := &mp4.Esds{}
	if _, err := w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeEsds()}); err != nil {
		return err
	}
	if _, err := mp4.Marshal(w, esds, ctx); err != nil {
		return err
	}
	if _, err := w.EndBox(); err != nil { // esds
		return err
	}
	return close() // mp4a
}

func unityMatrix() [9]int32 {
	return [9]int32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000}
}
