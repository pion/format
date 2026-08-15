// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package mpegts

import "errors"

var (
	errSectionTruncated = errors.New("mpegts: PSI section truncated")
	errSectionCRC       = errors.New("mpegts: PSI section CRC mismatch")
	errSectionMultipart = errors.New("mpegts: multi-section PSI tables are not supported")
	errSectionTooLong   = errors.New("mpegts: PSI section too long")
)

// psiSection is the generic PSI envelope (spec 2.4.4): PAT and PMT both use
// the "long" section syntax. Only single-section tables are supported.
type psiSection struct {
	tableID          uint8
	tableIDExtension uint16 // transport_stream_id (PAT) / program_number (PMT)
	version          uint8
	data             []byte // body between last_section_number and the CRC
}

// parsePSISection parses a section from a TS packet payload that has the
// payload_unit_start_indicator set (so it begins with a pointer_field).
func parsePSISection(payload []byte) (psiSection, error) {
	if len(payload) < 1 {
		return psiSection{}, errSectionTruncated
	}
	skip := 1 + int(payload[0]) // pointer_field
	if len(payload) < skip {
		return psiSection{}, errSectionTruncated
	}

	return parseSectionBytes(payload[skip:])
}

// parseSectionBytes parses a complete section starting at table_id.
func parseSectionBytes(buf []byte) (psiSection, error) {
	var sec psiSection
	if len(buf) < 8 {
		return sec, errSectionTruncated
	}
	sec.tableID = buf[0]
	length := int(buf[1]&0x0F)<<8 | int(buf[2]) // bytes after this field, CRC included
	if length < 9 || 3+length > len(buf) {
		return sec, errSectionTruncated
	}
	sec.tableIDExtension = uint16(buf[3])<<8 | uint16(buf[4])
	sec.version = buf[5] >> 1 & 0x1F
	if buf[6] != 0 || buf[7] != 0 { // section_number / last_section_number
		return sec, errSectionMultipart
	}
	end := 3 + length
	body, crcBytes := buf[:end-4], buf[end-4:end]
	want := uint32(crcBytes[0])<<24 | uint32(crcBytes[1])<<16 |
		uint32(crcBytes[2])<<8 | uint32(crcBytes[3])
	if crc32MPEG(body) != want {
		return sec, errSectionCRC
	}
	sec.data = buf[8 : end-4]

	return sec, nil
}

// sectionAssembler reassembles one PSI section that may span several TS
// packets on the same PID. Memory is bounded by the 12-bit section_length.
type sectionAssembler struct {
	buf    []byte
	active bool
}

// feed consumes one packet payload and returns the complete section bytes
// (starting at table_id) once the announced section_length has arrived,
// nil while the section is still incomplete.
func (a *sectionAssembler) feed(unitStart bool, payload []byte) []byte {
	if unitStart {
		a.active = false
		skip := 1 + int(payload[0]) // pointer_field
		if skip > len(payload) {
			return nil
		}
		a.buf = append(a.buf[:0], payload[skip:]...)
		a.active = true
	} else {
		if !a.active {
			return nil
		}
		a.buf = append(a.buf, payload...)
	}
	if len(a.buf) < 3 {
		return nil
	}
	total := 3 + (int(a.buf[1]&0x0F)<<8 | int(a.buf[2]))
	if len(a.buf) < total {
		return nil
	}
	a.active = false

	return a.buf[:total]
}

// marshal serializes the section, pointer_field included, ready to be placed
// at the start of a TS packet payload.
//
//nolint:gosec // Byte conversions serialize bounded values (length <= 0x3FD) and the CRC.
func (s psiSection) marshal() ([]byte, error) {
	length := 5 + len(s.data) + 4 // fixed part after length field + body + CRC
	if length > 0x3FD {
		return nil, errSectionTooLong
	}
	out := make([]byte, 0, 4+4+len(s.data)+4)
	out = append(out, 0x00) // pointer_field
	out = append(out,
		s.tableID,
		0xB0|byte(length>>8), // syntax=1, '0', reserved '11'
		byte(length),
		byte(s.tableIDExtension>>8),
		byte(s.tableIDExtension),
		0xC1|s.version<<1, // reserved '11', version, current_next=1
		0x00,              // section_number
		0x00,              // last_section_number
	)
	out = append(out, s.data...)
	crc := crc32MPEG(out[1:]) // CRC covers table_id .. end of body
	out = append(out, byte(crc>>24), byte(crc>>16), byte(crc>>8), byte(crc))

	return out, nil
}
