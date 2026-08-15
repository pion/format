// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package mpegts

import "errors"

var (
	errShortPacket     = errors.New("mpegts: packet shorter than 188 bytes")
	errInvalidSyncByte = errors.New("mpegts: invalid sync byte")
	errInvalidAFC      = errors.New("mpegts: adaptation field control 00 is reserved")
)

type packetHeader struct {
	transportError     bool
	payloadUnitStart   bool
	transportPriority  bool
	pid                uint16
	scramblingControl  uint8
	hasAdaptationField bool
	hasPayload         bool
	continuityCounter  uint8
}

func parsePacketHeader(pkt []byte) (packetHeader, error) {
	var hdr packetHeader
	if len(pkt) < 4 {
		return hdr, errShortPacket
	}
	if pkt[0] != syncByte {
		return hdr, errInvalidSyncByte
	}
	hdr.transportError = pkt[1]&0x80 != 0
	hdr.payloadUnitStart = pkt[1]&0x40 != 0
	hdr.transportPriority = pkt[1]&0x20 != 0
	hdr.pid = uint16(pkt[1]&0x1F)<<8 | uint16(pkt[2])
	hdr.scramblingControl = pkt[3] >> 6
	afc := (pkt[3] >> 4) & 0x03
	if afc == 0 {
		return hdr, errInvalidAFC
	}
	hdr.hasAdaptationField = afc&0x02 != 0
	hdr.hasPayload = afc&0x01 != 0
	hdr.continuityCounter = pkt[3] & 0x0F

	return hdr, nil
}

// marshalTo writes the 4-byte header into pkt (len(pkt) >= 4).
func (h packetHeader) marshalTo(pkt []byte) {
	pkt[0] = syncByte
	pkt[1] = byte(h.pid>>8) & 0x1F
	if h.transportError {
		pkt[1] |= 0x80
	}
	if h.payloadUnitStart {
		pkt[1] |= 0x40
	}
	if h.transportPriority {
		pkt[1] |= 0x20
	}
	pkt[2] = byte(h.pid) //nolint:gosec // PID low byte is intentionally truncated.
	var afc byte
	if h.hasAdaptationField {
		afc |= 0x02
	}
	if h.hasPayload {
		afc |= 0x01
	}
	pkt[3] = h.scramblingControl<<6 | afc<<4 | h.continuityCounter&0x0F
}
