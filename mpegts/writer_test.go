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

func TestWriterReaderRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	writer, err := NewWriter(&buf, WithH264Track(256))
	require.NoError(t, err)

	aus := [][]byte{
		bytes.Repeat([]byte{0x11}, 700), // multi-packet
		{0x00, 0x00, 0x00, 0x01, 0x41, 0x9A},
		bytes.Repeat([]byte{0x22}, 183), // near-exact packet fit
	}
	pts := []int64{90000, 93600, 97200}
	for i, au := range aus {
		require.NoError(t, writer.WriteH264(256, pts[i], pts[i], au))
	}
	require.NoError(t, writer.Close())
	assert.Zero(t, buf.Len()%packetSize, "output must be 188-byte aligned")

	reader, err := NewReader(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	require.Len(t, reader.Tracks(), 1)
	assert.Equal(t, CodecH264, reader.Tracks()[0].Codec)

	for i, au := range aus {
		got, readErr := reader.NextAccessUnit()
		require.NoError(t, readErr, "au %d", i)
		// The writer prepends an access unit delimiter to frames lacking one.
		want := append(append([]byte{}, audH264...), au...)
		assert.Equal(t, want, got.Data, "au %d", i)
		assert.Equal(t, pts[i], got.PTS, "au %d", i)
		assert.Equal(t, pts[i], got.DTS, "au %d", i)
	}
	_, err = reader.NextAccessUnit()
	assert.ErrorIs(t, err, io.EOF)
}

func TestWriterBFrameTimestamps(t *testing.T) {
	var buf bytes.Buffer
	writer, err := NewWriter(&buf, WithH265Track(257))
	require.NoError(t, err)
	require.NoError(t, writer.WriteH265(257, 97200, 90000, []byte{0x01, 0x02}))

	reader, err := NewReader(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	au, err := reader.NextAccessUnit()
	require.NoError(t, err)
	assert.Equal(t, int64(97200), au.PTS)
	assert.Equal(t, int64(90000), au.DTS)
	assert.Equal(t, CodecH265, au.Track.Codec)
}

func TestWriterOptionValidation(t *testing.T) {
	_, err := NewWriter(&bytes.Buffer{})
	assert.ErrorIs(t, err, errNoTracks)
	_, err = NewWriter(&bytes.Buffer{}, WithH264Track(256), WithH265Track(256))
	assert.ErrorIs(t, err, errPIDInUse)
	_, err = NewWriter(&bytes.Buffer{}, WithH264Track(0x10))
	assert.ErrorIs(t, err, errInvalidPID)
	var w *Writer
	w, err = NewWriter(&bytes.Buffer{}, WithH264Track(256))
	require.NoError(t, err)
	assert.ErrorIs(t, w.WriteH265(256, 0, 0, []byte{1}), errCodecMismatch)
	assert.ErrorIs(t, w.WriteH264(999, 0, 0, []byte{1}), errUnknownPID)
}

func TestWriterRejectsOversizedPMT(t *testing.T) {
	// Each track adds 5 bytes to the PMT; 34 tracks push the section past
	// the 184 bytes that fit in a single TS packet.
	opts := make([]WriterOption, 34)
	for i := range opts {
		opts[i] = WithH264Track(uint16(0x0100 + i)) //nolint:gosec
	}
	_, err := NewWriter(&bytes.Buffer{}, opts...)
	assert.ErrorIs(t, err, errPSITooLarge)
}
