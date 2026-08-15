// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package mpegts

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForEachNAL(t *testing.T) {
	annexB := []byte{
		0, 0, 0, 1, 0x67, 0xAA,
		0, 0, 1, 0x68, 0xBB,
		0, 0, 0, 1, 0x65, 0xCC,
	}
	var types []uint8
	forEachNAL(annexB, func(nal []byte) bool {
		types = append(types, h264NALType(nal))

		return true
	})
	assert.Equal(t, []uint8{7, 8, 5}, types)

	// No start code: not a valid access unit, yields no NAL units.
	types = nil
	forEachNAL([]byte{0x65, 0x01}, func(nal []byte) bool {
		types = append(types, h264NALType(nal))

		return true
	})
	assert.Empty(t, types)
}

func TestRandomAccessDetection(t *testing.T) {
	assert.True(t, h264IsRandomAccess([]byte{0, 0, 0, 1, 0x65, 0x88}))
	assert.False(t, h264IsRandomAccess([]byte{0, 0, 0, 1, 0x41, 0x9A}))
	assert.True(t, h265IsRandomAccess([]byte{0, 0, 0, 1, 0x26, 0x01, 0xAF}))  // type 19 IDR_W_RADL
	assert.False(t, h265IsRandomAccess([]byte{0, 0, 0, 1, 0x02, 0x01, 0xD0})) // type 1
}

func TestWriterInsertsAUDAndRAI(t *testing.T) {
	var buf bytes.Buffer
	writer, err := NewWriter(&buf, WithH264Track(256))
	require.NoError(t, err)
	idr := []byte{0x00, 0x00, 0x00, 0x01, 0x65, 0x88, 0x84}
	require.NoError(t, writer.WriteH264(256, 90000, 90000, idr))

	reader, err := NewReader(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	au, err := reader.NextAccessUnit()
	require.NoError(t, err)
	assert.True(t, au.RandomAccess)
	assert.Equal(t, append(append([]byte{}, audH264...), idr...), au.Data)

	// A frame already starting with an AUD is passed through untouched.
	buf.Reset()
	writer, err = NewWriter(&buf, WithH264Track(256))
	require.NoError(t, err)
	withAUD := append(append([]byte{}, audH264...), idr...)
	require.NoError(t, writer.WriteH264(256, 90000, 90000, withAUD))
	reader, err = NewReader(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	au, err = reader.NextAccessUnit()
	require.NoError(t, err)
	assert.Equal(t, withAUD, au.Data)
}
