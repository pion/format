// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package mpegts

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testPSIPacket(t *testing.T, pid uint16, cc uint8, section []byte) []byte {
	t.Helper()
	pkt := make([]byte, packetSize)
	packetHeader{pid: pid, payloadUnitStart: true, hasPayload: true, continuityCounter: cc}.marshalTo(pkt)
	n := copy(pkt[4:], section)
	for i := 4 + n; i < packetSize; i++ {
		pkt[i] = 0xFF
	}

	return pkt
}

// testPSISectionPackets splits one marshaled section (pointer_field
// included) across as many TS packets as it needs.
func testPSISectionPackets(t *testing.T, pid uint16, section []byte) []byte {
	t.Helper()
	var out []byte
	first := true
	cc := uint8(0)
	for first || len(section) > 0 {
		pkt := make([]byte, packetSize)
		packetHeader{pid: pid, payloadUnitStart: first, hasPayload: true, continuityCounter: cc}.marshalTo(pkt)
		cc = (cc + 1) & 0x0F
		n := copy(pkt[4:], section)
		section = section[n:]
		for i := 4 + n; i < packetSize; i++ {
			pkt[i] = 0xFF
		}
		out = append(out, pkt...)
		first = false
	}

	return out
}

// testESPackets wraps one PES packet (header+payload) into TS packets.
func testESPackets(t *testing.T, pid uint16, cc *uint8, rai bool, pes []byte) []byte {
	t.Helper()
	var out []byte
	first := true
	for first || len(pes) > 0 {
		pkt := make([]byte, packetSize)
		hdr := packetHeader{pid: pid, payloadUnitStart: first, hasPayload: true, continuityCounter: *cc}
		*cc = (*cc + 1) & 0x0F
		af := adaptationField{randomAccess: rai && first}
		minAF := 0
		if af.randomAccess {
			minAF = af.encodedLen()
		}
		afTotal := minAF
		if len(pes) < packetSize-4-minAF {
			afTotal = packetSize - 4 - len(pes)
		}
		hdr.hasAdaptationField = afTotal > 0
		hdr.marshalTo(pkt)
		off := 4
		if afTotal > 0 {
			af.marshalTo(pkt[4:], afTotal)
			off += afTotal
		}
		n := copy(pkt[off:], pes)
		pes = pes[n:]
		out = append(out, pkt...)
		first = false
	}

	return out
}

func buildTestStream(t *testing.T, aus [][]byte, pts []int64, rai []bool) []byte {
	t.Helper()
	pat, err := marshalPAT(1, []patProgram{{programNumber: 1, pmtPID: 0x1000}})
	require.NoError(t, err)
	pmtRaw, err := marshalPMT(1, pmt{pcrPID: 256, streams: []pmtStream{{streamType: streamTypeH264, pid: 256}}})
	require.NoError(t, err)

	stream := testPSIPacket(t, pidPAT, 0, pat)
	stream = append(stream, testPSIPacket(t, 0x1000, 0, pmtRaw)...)
	var cc uint8
	for i, au := range aus {
		pes := appendPESHeader(nil, streamIDVideo, pts[i], pts[i])
		pes = append(pes, au...)
		stream = append(stream, testESPackets(t, 256, &cc, rai[i], pes)...)
	}

	return stream
}

func TestReaderTwoAccessUnits(t *testing.T) {
	au0 := bytes.Repeat([]byte{0xAA}, 400) // spans 3 TS packets
	au1 := []byte{0x00, 0x00, 0x00, 0x01, 0x65, 0x11, 0x22}
	stream := buildTestStream(t, [][]byte{au0, au1}, []int64{90000, 93600}, []bool{true, false})

	reader, err := NewReader(bytes.NewReader(stream))
	require.NoError(t, err)
	require.Len(t, reader.Tracks(), 1)
	assert.Equal(t, CodecH264, reader.Tracks()[0].Codec)

	got0, err := reader.NextAccessUnit()
	require.NoError(t, err)
	assert.Equal(t, au0, got0.Data)
	assert.Equal(t, int64(90000), got0.PTS)
	assert.Equal(t, int64(90000), got0.DTS)
	assert.True(t, got0.RandomAccess)

	got1, err := reader.NextAccessUnit()
	require.NoError(t, err)
	assert.Equal(t, au1, got1.Data)
	assert.Equal(t, int64(93600), got1.PTS)
	assert.False(t, got1.RandomAccess)

	_, err = reader.NextAccessUnit()
	assert.ErrorIs(t, err, io.EOF)
}

func TestReaderMultiPacketPMT(t *testing.T) {
	// 40 non-video streams plus one H.264 stream make the PMT exceed one
	// TS packet payload, so the Reader must reassemble it.
	streams := make([]pmtStream, 0, 41)
	for i := range 40 {
		streams = append(streams, pmtStream{streamType: 0x06, pid: uint16(0x0300 + i)}) //nolint:gosec
	}
	streams = append(streams, pmtStream{streamType: streamTypeH264, pid: 256})
	pmtRaw, err := marshalPMT(1, pmt{pcrPID: 256, streams: streams})
	require.NoError(t, err)
	require.Greater(t, len(pmtRaw), packetSize-4, "PMT must span multiple TS packets")

	pat, err := marshalPAT(1, []patProgram{{programNumber: 1, pmtPID: 0x1000}})
	require.NoError(t, err)
	stream := testPSIPacket(t, pidPAT, 0, pat)
	stream = append(stream, testPSISectionPackets(t, 0x1000, pmtRaw)...)
	var cc uint8
	au := []byte{0x00, 0x00, 0x00, 0x01, 0x65, 0x11}
	pes := appendPESHeader(nil, streamIDVideo, 90000, 90000)
	pes = append(pes, au...)
	stream = append(stream, testESPackets(t, 256, &cc, true, pes)...)

	reader, err := NewReader(bytes.NewReader(stream))
	require.NoError(t, err)
	require.Len(t, reader.Tracks(), 1)
	assert.Equal(t, uint16(256), reader.Tracks()[0].PID)

	got, err := reader.NextAccessUnit()
	require.NoError(t, err)
	assert.Equal(t, au, got.Data)
}

func TestReaderTimestampWrap(t *testing.T) {
	nearWrap := maxTimestamp - 1000
	stream := buildTestStream(t,
		[][]byte{{0x01}, {0x02}},
		[]int64{nearWrap, (nearWrap + 3600) & maxTimestamp}, // wraps past 2^33
		[]bool{false, false})
	reader, err := NewReader(bytes.NewReader(stream))
	require.NoError(t, err)
	first, err := reader.NextAccessUnit()
	require.NoError(t, err)
	second, err := reader.NextAccessUnit()
	require.NoError(t, err)
	assert.Equal(t, first.PTS+3600, second.PTS) // continuous despite the wrap
}

func TestExtendTimestamp(t *testing.T) {
	assert.Equal(t, int64(500), extendTimestamp(-1, 500))
	assert.Equal(t, timestampWrap+10, extendTimestamp(maxTimestamp-5, 10))
	assert.Equal(t, maxTimestamp-5, extendTimestamp(timestampWrap+10, maxTimestamp-5)) // small backward step
}
