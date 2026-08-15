// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package mpegts

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPATRoundTrip(t *testing.T) {
	raw, err := marshalPAT(1, []patProgram{{programNumber: 1, pmtPID: 0x1000}})
	require.NoError(t, err)
	sec, err := parsePSISection(raw)
	require.NoError(t, err)
	progs, err := parsePAT(sec)
	require.NoError(t, err)
	require.Len(t, progs, 1)
	assert.Equal(t, uint16(1), progs[0].programNumber)
	assert.Equal(t, uint16(0x1000), progs[0].pmtPID)
}

func TestPMTRoundTrip(t *testing.T) {
	in := pmt{pcrPID: 256, streams: []pmtStream{
		{streamType: streamTypeH264, pid: 256},
		{streamType: streamTypeH265, pid: 257},
	}}
	raw, err := marshalPMT(1, in)
	require.NoError(t, err)
	sec, err := parsePSISection(raw)
	require.NoError(t, err)
	out, err := parsePMT(sec)
	require.NoError(t, err)
	assert.Equal(t, in, out)
	assert.Equal(t, CodecH264, codecFromStreamType(out.streams[0].streamType))
	assert.Equal(t, CodecH265, codecFromStreamType(out.streams[1].streamType))
}
