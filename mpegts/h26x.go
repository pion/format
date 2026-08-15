// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package mpegts

import "bytes"

//nolint:gochecknoglobals
var (
	nalStartCode3 = []byte{0x00, 0x00, 0x01}
	// Access unit delimiters as emitted by common muxers.
	audH264 = []byte{0x00, 0x00, 0x00, 0x01, 0x09, 0xF0}
	audH265 = []byte{0x00, 0x00, 0x00, 0x01, 0x46, 0x01, 0x50}
)

const (
	h264NALTypeIDR = 5
	h264NALTypeAUD = 9
	h265NALTypeAUD = 35
)

// forEachNAL iterates over the NAL units of one complete Annex-B access
// unit. It is not a streaming parser: it must not be fed partial chunks.
// A buffer without any start code yields no NAL units. fn returns false
// to stop early.
func forEachNAL(annexB []byte, fn func(nal []byte) bool) {
	idx := bytes.Index(annexB, nalStartCode3)
	for idx != -1 {
		start := idx + 3
		rel := bytes.Index(annexB[start:], nalStartCode3)
		if rel == -1 {
			if start < len(annexB) {
				fn(annexB[start:])
			}

			return
		}
		end := start + rel
		if end > start && annexB[end-1] == 0x00 { // 4-byte start code
			end--
		}
		if end > start && !fn(annexB[start:end]) {
			return
		}
		idx = start + rel
	}
}

func h264NALType(nal []byte) uint8 { return nal[0] & 0x1F }
func h265NALType(nal []byte) uint8 { return nal[0] >> 1 & 0x3F }

func h264StartsWithAUD(au []byte) bool { return firstNALType(au, h264NALType) == h264NALTypeAUD }
func h265StartsWithAUD(au []byte) bool { return firstNALType(au, h265NALType) == h265NALTypeAUD }

func firstNALType(au []byte, typeOf func([]byte) uint8) uint8 {
	result := uint8(0xFF)
	forEachNAL(au, func(nal []byte) bool {
		result = typeOf(nal)

		return false
	})

	return result
}

// h264IsRandomAccess reports whether the access unit contains an IDR
// slice, i.e. whether it is a valid random access point.
func h264IsRandomAccess(au []byte) bool {
	found := false
	forEachNAL(au, func(nal []byte) bool {
		if h264NALType(nal) == h264NALTypeIDR {
			found = true

			return false
		}

		return true
	})

	return found
}

// h265IsRandomAccess reports whether the access unit contains an IRAP
// slice (NAL types 16-23: BLA, IDR and CRA pictures). IRAP pictures are
// valid random access points but not necessarily full decoder refreshes:
// CRA and BLA pictures may have undecodable leading pictures.
func h265IsRandomAccess(au []byte) bool {
	found := false
	forEachNAL(au, func(nal []byte) bool {
		if t := h265NALType(nal); t >= 16 && t <= 23 {
			found = true

			return false
		}

		return true
	})

	return found
}
