// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package mpegts

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readFixture(t *testing.T, name string) *Reader {
	t.Helper()
	file, err := os.Open(filepath.Join("testdata", name)) //nolint:gosec // fixed fixture names in tests
	if errors.Is(err, os.ErrNotExist) {
		t.Skipf("fixture %s not generated; see testdata/README.md", name)
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = file.Close() })
	reader, err := NewReader(file)
	require.NoError(t, err)

	return reader
}

func drainFixture(t *testing.T, reader *Reader) []AccessUnit {
	t.Helper()
	var aus []AccessUnit
	for {
		au, err := reader.NextAccessUnit()
		if errors.Is(err, io.EOF) {
			return aus
		}
		require.NoError(t, err)
		copied := *au
		copied.Data = append([]byte(nil), au.Data...)
		aus = append(aus, copied)
	}
}

func TestFFmpegH264Fixture(t *testing.T) {
	reader := readFixture(t, "h264.ts")
	require.Len(t, reader.Tracks(), 1)
	assert.Equal(t, CodecH264, reader.Tracks()[0].Codec)
	aus := drainFixture(t, reader)
	assert.Len(t, aus, 25)
	assert.True(t, aus[0].RandomAccess)
	assert.True(t, h264IsRandomAccess(aus[0].Data))
}

func TestFFmpegH265Fixture(t *testing.T) {
	reader := readFixture(t, "h265.ts")
	require.Len(t, reader.Tracks(), 1)
	assert.Equal(t, CodecH265, reader.Tracks()[0].Codec)
	aus := drainFixture(t, reader)
	assert.Len(t, aus, 25)
	assert.True(t, h265IsRandomAccess(aus[0].Data))
}

func TestFFmpegBFrameFixture(t *testing.T) {
	reader := readFixture(t, "h264_bframes.ts")
	aus := drainFixture(t, reader)
	require.NotEmpty(t, aus)
	sawReorder := false
	for i := 1; i < len(aus); i++ {
		assert.GreaterOrEqual(t, aus[i].DTS, aus[i-1].DTS, "DTS must be monotonic")
		if aus[i].PTS < aus[i-1].PTS {
			sawReorder = true // B-frames present: presentation order differs from decode order
		}
	}
	assert.True(t, sawReorder, "expected PTS reordering from B-frames")
}
