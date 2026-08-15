// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package mpegts

import "errors"

var (
	errNotPMT       = errors.New("mpegts: section is not a PMT")
	errPMTTruncated = errors.New("mpegts: PMT truncated")
)

const tableIDPMT = 0x02

type pmtStream struct {
	streamType uint8
	pid        uint16
}

type pmt struct {
	pcrPID  uint16
	streams []pmtStream
}

func parsePMT(sec psiSection) (pmt, error) {
	var table pmt
	if sec.tableID != tableIDPMT {
		return table, errNotPMT
	}
	buf := sec.data
	if len(buf) < 4 {
		return table, errPMTTruncated
	}
	table.pcrPID = uint16(buf[0]&0x1F)<<8 | uint16(buf[1])
	infoLen := int(buf[2]&0x0F)<<8 | int(buf[3])
	if len(buf) < 4+infoLen {
		return table, errPMTTruncated
	}
	buf = buf[4+infoLen:] // skip program descriptors
	for len(buf) > 0 {
		if len(buf) < 5 {
			return table, errPMTTruncated
		}
		esInfoLen := int(buf[3]&0x0F)<<8 | int(buf[4])
		if len(buf) < 5+esInfoLen {
			return table, errPMTTruncated
		}
		table.streams = append(table.streams, pmtStream{
			streamType: buf[0],
			pid:        uint16(buf[1]&0x1F)<<8 | uint16(buf[2]),
		})
		buf = buf[5+esInfoLen:] // skip ES descriptors
	}

	return table, nil
}

//nolint:unparam // The program number is part of the PMT format and is intentionally configurable.
func marshalPMT(programNumber uint16, table pmt) ([]byte, error) {
	data := make([]byte, 0, 4+len(table.streams)*5)
	// The byte conversions intentionally serialize the high and low parts
	// of these bounded MPEG-TS PID fields.
	data = append(data,
		0xE0|byte(table.pcrPID>>8), byte(table.pcrPID), //nolint:gosec
		0xF0, 0x00, // program_info_length = 0
	)
	for _, s := range table.streams {
		data = append(data,
			s.streamType,
			0xE0|byte(s.pid>>8), byte(s.pid), //nolint:gosec
			0xF0, 0x00, // ES_info_length = 0
		)
	}

	return psiSection{tableID: tableIDPMT, tableIDExtension: programNumber, data: data}.marshal()
}
