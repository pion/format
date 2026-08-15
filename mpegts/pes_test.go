// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package mpegts

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimestampRoundTrip(t *testing.T) {
	for _, ts := range []int64{0, 1, 90000, maxTimestamp, maxTimestamp - 1, 1 << 32} {
		buf := appendTimestamp(nil, 0b0010, ts)
		require.Len(t, buf, 5)
		assert.Equal(t, ts, parseTimestamp(buf), "ts=%d", ts)
	}
}

func TestPESHeaderPTSOnly(t *testing.T) {
	raw := appendPESHeader(nil, streamIDVideo, 90000, 90000)
	hdr, err := parsePESHeader(raw)
	require.NoError(t, err)
	assert.Equal(t, byte(streamIDVideo), hdr.streamID)
	assert.True(t, hdr.hasPTS)
	assert.False(t, hdr.hasDTS)
	assert.Equal(t, int64(90000), hdr.pts)
	assert.Equal(t, len(raw), hdr.headerLength)
}

func TestPESHeaderPTSAndDTS(t *testing.T) {
	raw := appendPESHeader(nil, streamIDVideo, 96000, 90000)
	hdr, err := parsePESHeader(raw)
	require.NoError(t, err)
	assert.True(t, hdr.hasPTS)
	assert.True(t, hdr.hasDTS)
	assert.Equal(t, int64(96000), hdr.pts)
	assert.Equal(t, int64(90000), hdr.dts)
}

func TestPESHeaderErrors(t *testing.T) {
	_, err := parsePESHeader([]byte{0, 0, 1})
	assert.ErrorIs(t, err, errPESTruncated)
	_, err = parsePESHeader([]byte{0, 0, 2, 0xE0, 0, 0, 0x84, 0x80, 0})
	assert.ErrorIs(t, err, errPESStartCode)
	_, err = parsePESHeader([]byte{0, 0, 1, 0xE0, 0, 0, 0x00, 0x80, 0})
	assert.ErrorIs(t, err, errPESMarkerBits)
}
