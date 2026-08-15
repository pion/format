// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package mpegts

import (
	"errors"
	"io"
)

var (
	errNoTracks        = errors.New("mpegts: writer needs at least one track")
	errPIDInUse        = errors.New("mpegts: PID already in use")
	errInvalidPID      = errors.New("mpegts: elementary stream PID out of range")
	errUnknownPID      = errors.New("mpegts: no track with this PID")
	errCodecMismatch   = errors.New("mpegts: track codec does not match write call")
	errEmptyAccessUnit = errors.New("mpegts: empty access unit")
	errPSITooLarge     = errors.New("mpegts: PAT/PMT section does not fit in one TS packet (too many tracks)")
)

const (
	writerPMTPID     = 0x1000
	psiIntervalTicks = 9000  // repeat PAT/PMT at least every 100 ms
	pcrDelayTicks    = 63000 // PCR leads DTS by 700 ms, clamped at zero
	minElementaryPID = 0x0020
	maxElementaryPID = 0x1FFE
)

// WriterOption configures a Writer.
type WriterOption func(*Writer) error

// WithH264Track adds an H.264 elementary stream on the given PID.
func WithH264Track(pid uint16) WriterOption { return withTrack(CodecH264, pid) }

// WithH265Track adds an H.265 elementary stream on the given PID.
func WithH265Track(pid uint16) WriterOption { return withTrack(CodecH265, pid) }

func withTrack(codec Codec, pid uint16) WriterOption {
	return func(w *Writer) error {
		if pid < minElementaryPID || pid > maxElementaryPID || pid == writerPMTPID {
			return errInvalidPID
		}
		if _, taken := w.byPID[pid]; taken {
			return errPIDInUse
		}
		track := &Track{PID: pid, Codec: codec}
		w.tracks = append(w.tracks, track)
		w.byPID[pid] = track

		return nil
	}
}

// Writer muxes video access units into a single-program MPEG-2 Transport
// Stream. The PCR is carried on the first configured track's PID.
type Writer struct {
	dst        io.Writer
	pkt        [packetSize]byte
	tracks     []*Track
	byPID      map[uint16]*Track
	cc         map[uint16]uint8
	pcrPID     uint16
	psiWritten bool
	lastPSIDTS int64
	pesScratch []byte
	segs       [][]byte
	patRaw     []byte
	pmtRaw     []byte
}

// NewWriter creates a Writer emitting to dst. At least one track option
// is required.
func NewWriter(dst io.Writer, opts ...WriterOption) (*Writer, error) {
	writer := &Writer{dst: dst, byPID: map[uint16]*Track{}, cc: map[uint16]uint8{}}
	for _, opt := range opts {
		if err := opt(writer); err != nil {
			return nil, err
		}
	}
	if len(writer.tracks) == 0 {
		return nil, errNoTracks
	}
	writer.pcrPID = writer.tracks[0].PID

	var err error
	if writer.patRaw, err = marshalPAT(1, []patProgram{{programNumber: 1, pmtPID: writerPMTPID}}); err != nil {
		return nil, err
	}
	streams := make([]pmtStream, len(writer.tracks))
	for i, track := range writer.tracks {
		streams[i] = pmtStream{streamType: streamTypeFromCodec(track.Codec), pid: track.PID}
	}
	if writer.pmtRaw, err = marshalPMT(1, pmt{pcrPID: writer.pcrPID, streams: streams}); err != nil {
		return nil, err
	}
	// writeSection emits each table as a single TS packet, so the marshaled
	// section (pointer_field included) must fit in one packet payload.
	if len(writer.patRaw) > packetSize-4 || len(writer.pmtRaw) > packetSize-4 {
		return nil, errPSITooLarge
	}

	return writer, nil
}

// WriteH264 writes one H.264 access unit in Annex-B format. An access unit
// delimiter NAL is prepended when absent. pts and dts are 90 kHz ticks;
// pass dts equal to pts when the stream has no B-frames.
func (w *Writer) WriteH264(pid uint16, pts, dts int64, accessUnit []byte) error {
	aud := []byte(nil)
	if len(accessUnit) > 0 && !h264StartsWithAUD(accessUnit) {
		aud = audH264
	}

	return w.writeAccessUnit(pid, CodecH264, pts, dts, aud, accessUnit, h264IsRandomAccess(accessUnit))
}

// WriteH265 writes one H.265 access unit in Annex-B format. An access unit
// delimiter NAL is prepended when absent.
func (w *Writer) WriteH265(pid uint16, pts, dts int64, accessUnit []byte) error {
	aud := []byte(nil)
	if len(accessUnit) > 0 && !h265StartsWithAUD(accessUnit) {
		aud = audH265
	}

	return w.writeAccessUnit(pid, CodecH265, pts, dts, aud, accessUnit, h265IsRandomAccess(accessUnit))
}

func (w *Writer) writeAccessUnit(
	pid uint16, codec Codec, pts, dts int64, aud, accessUnit []byte, randomAccess bool,
) error {
	track, ok := w.byPID[pid]
	if !ok {
		return errUnknownPID
	}
	if track.Codec != codec {
		return errCodecMismatch
	}
	if len(accessUnit) == 0 {
		return errEmptyAccessUnit
	}
	if err := w.maybeWritePSI(dts); err != nil {
		return err
	}

	w.pesScratch = appendPESHeader(w.pesScratch[:0], streamIDVideo, pts, dts)
	w.segs = w.segs[:0]
	w.segs = append(w.segs, w.pesScratch)
	if aud != nil {
		w.segs = append(w.segs, aud)
	}
	w.segs = append(w.segs, accessUnit)

	pcrValue := uint64(0)
	withPCR := pid == w.pcrPID
	if withPCR {
		base := max(dts-pcrDelayTicks, 0)
		pcrValue = uint64(base) * 300 //nolint:gosec
	}

	return w.writePES(pid, randomAccess, withPCR, pcrValue, w.segs)
}

func (w *Writer) maybeWritePSI(dts int64) error {
	if w.psiWritten && dts >= w.lastPSIDTS && dts-w.lastPSIDTS < psiIntervalTicks {
		return nil
	}
	if err := w.writeSection(pidPAT, w.patRaw); err != nil {
		return err
	}
	if err := w.writeSection(writerPMTPID, w.pmtRaw); err != nil {
		return err
	}
	w.psiWritten = true
	w.lastPSIDTS = dts

	return nil
}

func (w *Writer) writeSection(pid uint16, section []byte) error {
	pkt := w.pkt[:]
	packetHeader{
		pid: pid, payloadUnitStart: true, hasPayload: true, continuityCounter: w.nextCC(pid),
	}.marshalTo(pkt)
	n := copy(pkt[4:], section)
	for i := 4 + n; i < packetSize; i++ {
		pkt[i] = 0xFF
	}
	_, err := w.dst.Write(pkt)

	return err
}

func (w *Writer) nextCC(pid uint16) uint8 {
	counter := w.cc[pid]
	w.cc[pid] = (counter + 1) & 0x0F

	return counter
}

// writePES slices the concatenation of segs into 188-byte packets.
//
//nolint:cyclop // Branches directly model packet boundaries, adaptation fields, and segmented payload copies.
func (w *Writer) writePES(pid uint16, rai, withPCR bool, pcr uint64, segs [][]byte) error {
	remaining := 0
	for _, seg := range segs {
		remaining += len(seg)
	}
	segIdx, segOff := 0, 0
	first := true
	for first || remaining > 0 {
		pkt := w.pkt[:]
		hdr := packetHeader{
			pid: pid, payloadUnitStart: first, hasPayload: true, continuityCounter: w.nextCC(pid),
		}
		af := adaptationField{randomAccess: rai && first, hasPCR: withPCR && first, pcr: pcr}
		minAF := 0
		if af.randomAccess || af.hasPCR {
			minAF = af.encodedLen()
		}
		afTotal := minAF
		if remaining < packetSize-4-minAF { // final packet: stuff the difference
			afTotal = packetSize - 4 - remaining
		}
		hdr.hasAdaptationField = afTotal > 0
		hdr.marshalTo(pkt)
		off := 4
		if afTotal > 0 {
			af.marshalTo(pkt[4:], afTotal)
			off += afTotal
		}
		for off < packetSize && segIdx < len(segs) {
			n := copy(pkt[off:], segs[segIdx][segOff:])
			off += n
			segOff += n
			remaining -= n
			if segOff == len(segs[segIdx]) {
				segIdx++
				segOff = 0
			}
		}
		if _, err := w.dst.Write(pkt); err != nil {
			return err
		}
		first = false
	}

	return nil
}

// Close closes the underlying writer when it implements io.Closer. A
// transport stream needs no trailer.
func (w *Writer) Close() error {
	if closer, ok := w.dst.(io.Closer); ok {
		return closer.Close()
	}

	return nil
}
