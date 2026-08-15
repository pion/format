// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package mpegts

// Codec identifies the codec of an elementary stream.
type Codec int

// Supported codecs.
const (
	CodecUnknown Codec = iota
	CodecH264
	CodecH265
)

// String returns the codec name.
func (c Codec) String() string {
	switch c {
	case CodecH264:
		return "H264"
	case CodecH265:
		return "H265"
	default:
		return "unknown"
	}
}

// Track describes one elementary stream inside a transport stream.
type Track struct {
	PID   uint16
	Codec Codec
}

// Stream type assignments from spec Table 2-34.
const (
	streamTypeH264 = 0x1B
	streamTypeH265 = 0x24
)

func codecFromStreamType(st uint8) Codec {
	switch st {
	case streamTypeH264:
		return CodecH264
	case streamTypeH265:
		return CodecH265
	default:
		return CodecUnknown
	}
}

//nolint:unused // Used by the MPEG-TS writer when converting a Track to a PMT entry.
func streamTypeFromCodec(c Codec) uint8 {
	switch c {
	case CodecH264:
		return streamTypeH264
	case CodecH265:
		return streamTypeH265
	default:
		return 0
	}
}
