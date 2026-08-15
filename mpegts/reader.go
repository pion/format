// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package mpegts

import (
	"errors"
	"io"
)

var (
	errNoUsableProgram  = errors.New("mpegts: no program with a supported video stream found")
	errInvalidMaxAUSize = errors.New("mpegts: max access unit size must be positive")
)

const (
	defaultMaxAccessUnitSize = 4 << 20 // 4 MiB
	timestampWrap            = int64(1) << 33
)

// AccessUnit is one coded video frame extracted from the transport stream.
type AccessUnit struct {
	Track *Track
	// PTS and DTS are in 90 kHz ticks, unwrapped from the 33-bit wire
	// representation into monotonically increasing values. DTS equals PTS
	// when the stream carries no separate DTS.
	PTS, DTS int64
	// RandomAccess reports the random_access_indicator of the packet that
	// started this access unit (set by muxers on IDR/IRAP pictures).
	RandomAccess bool
	// Data is the access unit in Annex-B format. It is only valid until
	// the next call to NextAccessUnit; copy it to retain it.
	Data []byte
}

// ReaderOption configures a Reader.
type ReaderOption func(*Reader) error

// WithMaxAccessUnitSize bounds the memory used to reassemble one access
// unit. Larger units are dropped and reading resynchronizes. Default 4 MiB.
func WithMaxAccessUnitSize(n int) ReaderOption {
	return func(r *Reader) error {
		if n <= 0 {
			return errInvalidMaxAUSize
		}
		r.maxAUSize = n

		return nil
	}
}

// Reader demuxes an MPEG-2 Transport Stream from an io.Reader. It selects
// the first program advertised in the PAT and exposes its supported video
// streams as tracks.
type Reader struct {
	src       io.Reader
	pkt       [packetSize]byte
	maxAUSize int
	pmtPID    uint16
	tracks    []*Track
	streams   map[uint16]*esStream
	order     []*esStream
	flushIdx  int
	eof       bool
}

type esStream struct {
	track      *Track
	buf        []byte // access unit being accumulated
	out        []byte // buffer of the last returned access unit (reused)
	collecting bool
	hasCC      bool
	cc         uint8
	pts, dts   int64
	rai        bool
	lastTS     int64 // reference for 33-bit unwrapping
}

// NewReader creates a Reader and consumes packets until the PAT and PMT
// have been parsed, so Tracks is available immediately after it returns.
func NewReader(src io.Reader, opts ...ReaderOption) (*Reader, error) {
	reader := &Reader{
		src:       src,
		maxAUSize: defaultMaxAccessUnitSize,
		streams:   map[uint16]*esStream{},
	}
	for _, opt := range opts {
		if err := opt(reader); err != nil {
			return nil, err
		}
	}
	if err := reader.readTables(); err != nil {
		return nil, err
	}

	return reader, nil
}

// Tracks returns the supported video tracks of the selected program.
func (r *Reader) Tracks() []*Track {
	return r.tracks
}

func (r *Reader) readPacket() error {
	_, err := io.ReadFull(r.src, r.pkt[:])
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return io.EOF
	}

	return err
}

//nolint:cyclop // Packet filtering keeps malformed or unrelated table packets skippable.
func (r *Reader) readTables() error {
	havePAT := false
	var patAsm, pmtAsm sectionAssembler
	for {
		if err := r.readPacket(); err != nil {
			if errors.Is(err, io.EOF) {
				return errNoUsableProgram
			}

			return err
		}
		hdr, err := parsePacketHeader(r.pkt[:])
		if err != nil || hdr.transportError {
			continue
		}
		payload, _, perr := packetPayload(r.pkt[:], hdr)
		if perr != nil || payload == nil {
			continue
		}
		switch {
		case hdr.pid == pidPAT && !havePAT:
			if section := patAsm.feed(hdr.payloadUnitStart, payload); section != nil {
				havePAT = r.readPAT(section)
			}
		case havePAT && hdr.pid == r.pmtPID:
			section := pmtAsm.feed(hdr.payloadUnitStart, payload)
			if section == nil {
				continue
			}
			complete, err := r.readPMT(section)
			if err != nil {
				return err
			}
			if complete {
				return nil
			}
		}
	}
}

func (r *Reader) readPAT(sectionBytes []byte) bool {
	section, err := parseSectionBytes(sectionBytes)
	if err != nil {
		return false
	}
	programs, err := parsePAT(section)
	if err != nil || len(programs) == 0 {
		return false
	}
	r.pmtPID = programs[0].pmtPID

	return true
}

func (r *Reader) readPMT(sectionBytes []byte) (bool, error) {
	section, err := parseSectionBytes(sectionBytes)
	if err != nil {
		return false, nil //nolint:nilerr // malformed PMT: wait for the next repetition
	}
	table, ok := parsePMTSection(section)
	if !ok {
		return false, nil
	}
	for _, elementaryStream := range table.streams {
		codec := codecFromStreamType(elementaryStream.streamType)
		if codec == CodecUnknown {
			continue
		}
		track := &Track{PID: elementaryStream.pid, Codec: codec}
		stream := &esStream{track: track, lastTS: -1}
		r.tracks = append(r.tracks, track)
		r.streams[elementaryStream.pid] = stream
		r.order = append(r.order, stream)
	}
	if len(r.tracks) == 0 {
		return true, errNoUsableProgram
	}

	return true, nil
}

func parsePMTSection(section psiSection) (pmt, bool) {
	table, err := parsePMT(section)

	return table, err == nil
}

func packetPayload(pkt []byte, hdr packetHeader) ([]byte, adaptationField, error) {
	off := 4
	var af adaptationField
	if hdr.hasAdaptationField {
		parsed, n, err := parseAdaptationField(pkt[4:])
		if err != nil {
			return nil, af, err
		}
		af = parsed
		off += n
	}
	if !hdr.hasPayload || off >= len(pkt) {
		return nil, af, nil
	}

	return pkt[off:], af, nil
}

// NextAccessUnit returns the next complete access unit in stream order.
// It returns io.EOF once the input and all buffered data are exhausted.
func (r *Reader) NextAccessUnit() (*AccessUnit, error) {
	for {
		if r.eof {
			for r.flushIdx < len(r.order) {
				stream := r.order[r.flushIdx]
				r.flushIdx++
				if au := stream.finish(); au != nil {
					return au, nil
				}
			}

			return nil, io.EOF
		}
		if err := r.readPacket(); err != nil {
			if errors.Is(err, io.EOF) {
				r.eof = true

				continue
			}

			return nil, err
		}
		if au := r.handlePacket(); au != nil {
			return au, nil
		}
	}
}

func (r *Reader) handlePacket() *AccessUnit {
	hdr, err := parsePacketHeader(r.pkt[:])
	if err != nil || hdr.transportError || hdr.scramblingControl != 0 {
		return nil // skip unusable packets, stay aligned on the 188-byte grid
	}
	stream, ok := r.streams[hdr.pid]
	if !ok {
		return nil // PSI repetitions, other programs, null packets
	}
	payload, af, err := packetPayload(r.pkt[:], hdr)
	if err != nil || payload == nil {
		return nil
	}

	return r.handleES(stream, hdr, af.randomAccess, payload)
}

func (r *Reader) handleES(stream *esStream, hdr packetHeader, rai bool, payload []byte) *AccessUnit {
	if stream.hasCC {
		if hdr.continuityCounter == stream.cc && !hdr.payloadUnitStart {
			return nil // duplicate packet
		}
		if hdr.continuityCounter != (stream.cc+1)&0x0F {
			// Packet loss: the partial access unit is unusable.
			stream.buf = stream.buf[:0]
			stream.collecting = false
		}
	}
	stream.cc, stream.hasCC = hdr.continuityCounter, true

	if hdr.payloadUnitStart {
		completed := stream.finish()
		stream.startUnit(rai, payload)

		return completed
	}
	if !stream.collecting {
		return nil
	}
	stream.buf = append(stream.buf, payload...)
	if len(stream.buf) > r.maxAUSize {
		stream.buf = stream.buf[:0]
		stream.collecting = false
	}

	return nil
}

func (s *esStream) startUnit(rai bool, payload []byte) {
	hdr, err := parsePESHeader(payload)
	if err != nil || !hdr.hasPTS {
		s.collecting = false // resync at the next payload unit start

		return
	}
	dts33 := hdr.pts
	if hdr.hasDTS {
		dts33 = hdr.dts
	}
	dts := extendTimestamp(s.lastTS, dts33)
	s.lastTS = dts
	s.pts = extendTimestamp(dts, hdr.pts)
	s.dts = dts
	s.rai = rai
	s.buf = append(s.buf[:0], payload[hdr.headerLength:]...)
	s.collecting = true
}

func (s *esStream) finish() *AccessUnit {
	if !s.collecting || len(s.buf) == 0 {
		s.collecting = false

		return nil
	}
	s.buf, s.out = s.out[:0], s.buf
	s.collecting = false

	return &AccessUnit{Track: s.track, PTS: s.pts, DTS: s.dts, RandomAccess: s.rai, Data: s.out}
}

// extendTimestamp maps a 33-bit timestamp onto the unwrapped timeline
// closest to the reference value.
func extendTimestamp(reference, ts33 int64) int64 {
	if reference < 0 {
		return ts33
	}
	candidate := reference - reference%timestampWrap + ts33
	switch {
	case candidate < reference-timestampWrap/2:
		candidate += timestampWrap
	case candidate > reference+timestampWrap/2:
		candidate -= timestampWrap
	}

	return candidate
}
