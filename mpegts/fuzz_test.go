// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package mpegts

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func FuzzReader(f *testing.F) {
	pat, err := marshalPAT(1, []patProgram{{programNumber: 1, pmtPID: 0x1000}})
	require.NoError(f, err)
	pmtRaw, err := marshalPMT(1, pmt{pcrPID: 256, streams: []pmtStream{{streamType: streamTypeH264, pid: 256}}})
	require.NoError(f, err)
	seed := make([]byte, 0, 3*packetSize)
	pkt := make([]byte, packetSize)
	packetHeader{pid: pidPAT, payloadUnitStart: true, hasPayload: true}.marshalTo(pkt)
	copy(pkt[4:], pat)
	seed = append(seed, pkt...)
	packetHeader{pid: 0x1000, payloadUnitStart: true, hasPayload: true}.marshalTo(pkt)
	copy(pkt[4:], pmtRaw)
	seed = append(seed, pkt...)
	f.Add(seed)
	f.Add(bytes.Repeat([]byte{0x47}, packetSize*4))

	f.Fuzz(func(_ *testing.T, data []byte) {
		reader, err := NewReader(bytes.NewReader(data))
		if err != nil {
			return
		}
		for {
			if _, err := reader.NextAccessUnit(); err != nil {
				if !errors.Is(err, io.EOF) {
					return
				}

				return
			}
		}
	})
}
