// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT
package mpegts

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePacketHeader(t *testing.T) {
	// PUSI=1, PID=0x0100, payload only, CC=5.
	pkt := []byte{0x47, 0x41, 0x00, 0x15}
	hdr, err := parsePacketHeader(pkt)
	require.NoError(t, err)
	assert.True(t, hdr.payloadUnitStart)
	assert.Equal(t, uint16(0x0100), hdr.pid)
	assert.True(t, hdr.hasPayload)
	assert.False(t, hdr.hasAdaptationField)
	assert.Equal(t, uint8(5), hdr.continuityCounter)
}

func TestPacketHeaderRoundTrip(t *testing.T) {
	in := packetHeader{
		payloadUnitStart:   true,
		pid:                0x1FFE,
		hasAdaptationField: true,
		hasPayload:         true,
		continuityCounter:  0x0F,
	}
	var buf [4]byte
	in.marshalTo(buf[:])
	out, err := parsePacketHeader(buf[:])
	require.NoError(t, err)
	assert.Equal(t, in, out)
}

func TestParsePacketHeaderErrors(t *testing.T) {
	_, err := parsePacketHeader([]byte{0x47, 0x00})
	assert.ErrorIs(t, err, errShortPacket)
	_, err = parsePacketHeader([]byte{0x48, 0x00, 0x00, 0x10})
	assert.ErrorIs(t, err, errInvalidSyncByte)
	_, err = parsePacketHeader([]byte{0x47, 0x00, 0x00, 0x00})
	assert.ErrorIs(t, err, errInvalidAFC)
}
