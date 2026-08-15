// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package mpegts

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCRC32MPEGKnownVector(t *testing.T) {
	// Standard check value for CRC-32/MPEG-2.
	assert.Equal(t, uint32(0x0376E6E7), crc32MPEG([]byte("123456789")))
}

func TestPSISectionRoundTrip(t *testing.T) {
	in := psiSection{tableID: 0x02, tableIDExtension: 1, version: 3, data: []byte{0xE1, 0x00, 0xF0, 0x00}}
	raw, err := in.marshal()
	require.NoError(t, err)
	out, err := parsePSISection(raw)
	require.NoError(t, err)
	assert.Equal(t, in.tableID, out.tableID)
	assert.Equal(t, in.tableIDExtension, out.tableIDExtension)
	assert.Equal(t, in.version, out.version)
	assert.Equal(t, in.data, out.data)
}

func TestPSISectionBadCRC(t *testing.T) {
	raw, err := psiSection{tableID: 0, data: []byte{1, 2, 3, 4}}.marshal()
	require.NoError(t, err)
	raw[len(raw)-1] ^= 0xFF
	_, err = parsePSISection(raw)
	assert.ErrorIs(t, err, errSectionCRC)
}
