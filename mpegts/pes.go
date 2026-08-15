// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package mpegts

import "errors"

var (
	errPESTruncated  = errors.New("mpegts: PES header truncated")
	errPESStartCode  = errors.New("mpegts: invalid PES start code")
	errPESMarkerBits = errors.New("mpegts: invalid PES marker bits")
)

const (
	streamIDVideo = 0xE0
	maxTimestamp  = int64(1)<<33 - 1
)

type pesHeader struct {
	streamID     byte
	packetLength int
	headerLength int // total bytes before the elementary stream payload
	hasPTS       bool
	hasDTS       bool
	pts          int64 // 33-bit, 90 kHz
	dts          int64
}

//nolint:cyclop // Validation branches mirror the PES header format.
func parsePESHeader(buf []byte) (pesHeader, error) {
	var hdr pesHeader
	if len(buf) < 9 {
		return hdr, errPESTruncated
	}
	if buf[0] != 0 || buf[1] != 0 || buf[2] != 1 {
		return hdr, errPESStartCode
	}
	hdr.streamID = buf[3]
	hdr.packetLength = int(buf[4])<<8 | int(buf[5])
	if buf[6]&0xC0 != 0x80 {
		return hdr, errPESMarkerBits
	}
	ptsDTSFlags := buf[7] >> 6
	hdr.headerLength = 9 + int(buf[8])
	if len(buf) < hdr.headerLength {
		return hdr, errPESTruncated
	}
	switch ptsDTSFlags {
	case 0b10:
		if int(buf[8]) < 5 {
			return hdr, errPESTruncated
		}
		hdr.hasPTS = true
		hdr.pts = parseTimestamp(buf[9:14])
	case 0b11:
		if int(buf[8]) < 10 {
			return hdr, errPESTruncated
		}
		hdr.hasPTS, hdr.hasDTS = true, true
		hdr.pts = parseTimestamp(buf[9:14])
		hdr.dts = parseTimestamp(buf[14:19])
	}

	return hdr, nil
}

// appendPESHeader appends a video PES header with unbounded packet length
// (PES_packet_length = 0, legal for video) and data alignment set.
// The DTS is written only when it differs from the PTS.
//
//nolint:unparam // The stream ID remains configurable for distinct video stream IDs.
func appendPESHeader(dst []byte, streamID byte, pts, dts int64) []byte {
	pts &= maxTimestamp
	dts &= maxTimestamp
	dst = append(dst, 0x00, 0x00, 0x01, streamID, 0x00, 0x00)
	if pts == dts {
		dst = append(dst, 0x84, 0x80, 5) // alignment; PTS only
		dst = appendTimestamp(dst, 0b0010, pts)
	} else {
		dst = append(dst, 0x84, 0xC0, 10) // alignment; PTS+DTS
		dst = appendTimestamp(dst, 0b0011, pts)
		dst = appendTimestamp(dst, 0b0001, dts)
	}

	return dst
}

// 33-bit timestamp in 5 bytes with marker bits (spec 2.4.3.7).
//
//nolint:gosec // Byte conversions intentionally serialize slices of the bounded 33-bit timestamp.
func appendTimestamp(dst []byte, prefix byte, ts int64) []byte {
	return append(dst,
		prefix<<4|byte(ts>>29)&0x0E|0x01,
		byte(ts>>22),
		byte(ts>>14)&0xFE|0x01,
		byte(ts>>7),
		byte(ts<<1)|0x01,
	)
}

func parseTimestamp(buf []byte) int64 {
	return int64(buf[0]&0x0E)<<29 | int64(buf[1])<<22 |
		int64(buf[2]&0xFE)<<14 | int64(buf[3])<<7 | int64(buf[4])>>1
}
