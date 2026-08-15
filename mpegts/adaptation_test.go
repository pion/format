// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package mpegts

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPCRRoundTrip(t *testing.T) {
	in := adaptationField{randomAccess: true, hasPCR: true, pcr: 27_000_000*3 + 123} // 3s + 123 ticks
	buf := make([]byte, 20)
	in.marshalTo(buf, 20)
	out, n, err := parseAdaptationField(buf)
	require.NoError(t, err)
	assert.Equal(t, 20, n)
	assert.Equal(t, in, out)
	for _, b := range buf[8:] { // stuffing after the 8 PCR-field bytes
		assert.Equal(t, byte(0xFF), b)
	}
}

func TestAdaptationFieldOneByteStuffing(t *testing.T) {
	var af adaptationField
	buf := make([]byte, 1)
	af.marshalTo(buf, 1)
	assert.Equal(t, byte(0), buf[0])
	out, n, err := parseAdaptationField(buf)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, adaptationField{}, out)
}

func TestAdaptationFieldEncodedLen(t *testing.T) {
	assert.Equal(t, 2, adaptationField{randomAccess: true}.encodedLen())
	assert.Equal(t, 8, adaptationField{hasPCR: true}.encodedLen())
}

func TestAdaptationFieldErrors(t *testing.T) {
	_, _, err := parseAdaptationField([]byte{})
	assert.ErrorIs(t, err, errAdaptationTruncated)
	_, _, err = parseAdaptationField([]byte{10, 0x00})
	assert.ErrorIs(t, err, errAdaptationTooLong)
	_, _, err = parseAdaptationField([]byte{3, 0x10, 0x00, 0x00}) // PCR flag but no room
	assert.ErrorIs(t, err, errAdaptationTruncated)
}
