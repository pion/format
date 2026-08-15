// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package mpegts

import "errors"

var (
	errAdaptationTruncated = errors.New("mpegts: adaptation field truncated")
	errAdaptationTooLong   = errors.New("mpegts: adaptation field longer than packet")
)

// adaptationField (spec 2.4.3.4). Only the fields required for video
// demuxing/muxing are represented; others are skipped on parse.
type adaptationField struct {
	discontinuity bool
	randomAccess  bool
	hasPCR        bool
	pcr           uint64 // 27 MHz units
}

// parseAdaptationField parses from the byte right after the 4-byte packet
// header and returns the number of bytes consumed (length byte included).
func parseAdaptationField(buf []byte) (adaptationField, int, error) {
	var af adaptationField
	if len(buf) < 1 {
		return af, 0, errAdaptationTruncated
	}
	length := int(buf[0])
	if 1+length > len(buf) {
		return af, 0, errAdaptationTooLong
	}
	if length == 0 { // single stuffing byte
		return af, 1, nil
	}
	flags := buf[1]
	af.discontinuity = flags&0x80 != 0
	af.randomAccess = flags&0x40 != 0
	if flags&0x10 != 0 { // PCR_flag
		if length < 7 {
			return af, 0, errAdaptationTruncated
		}
		af.hasPCR = true
		base := uint64(buf[2])<<25 | uint64(buf[3])<<17 |
			uint64(buf[4])<<9 | uint64(buf[5])<<1 | uint64(buf[6])>>7
		ext := uint64(buf[6]&0x01)<<8 | uint64(buf[7])
		af.pcr = base*300 + ext
	}

	return af, 1 + length, nil
}

// encodedLen is the minimum size of this adaptation field, length byte included.
func (af adaptationField) encodedLen() int {
	n := 2 // length + flags
	if af.hasPCR {
		n += 6
	}

	return n
}

// marshalTo writes the adaptation field into buf using exactly total bytes,
// filling the remainder with 0xFF stuffing. total must be 1 (pure one-byte
// stuffing, only valid when no fields are set) or >= encodedLen(), and buf
// must be at least total bytes long.
//
//nolint:gosec // Byte conversions deliberately slice the 33+9-bit PCR into bytes.
func (af adaptationField) marshalTo(buf []byte, total int) {
	buf[0] = byte(total - 1)
	if total == 1 {
		return
	}
	var flags byte
	if af.discontinuity {
		flags |= 0x80
	}
	if af.randomAccess {
		flags |= 0x40
	}
	off := 2
	if af.hasPCR {
		flags |= 0x10
		base, ext := af.pcr/300, af.pcr%300
		buf[2] = byte(base >> 25)
		buf[3] = byte(base >> 17)
		buf[4] = byte(base >> 9)
		buf[5] = byte(base >> 1)
		buf[6] = byte(base<<7) | 0x7E | byte(ext>>8)
		buf[7] = byte(ext)
		off = 8
	}
	buf[1] = flags
	for i := off; i < total; i++ {
		buf[i] = 0xFF
	}
}
