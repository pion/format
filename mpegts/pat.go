// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package mpegts

import "errors"

var errNotPAT = errors.New("mpegts: section is not a PAT")

const tableIDPAT = 0x00

type patProgram struct {
	programNumber uint16
	pmtPID        uint16
}

func parsePAT(sec psiSection) ([]patProgram, error) {
	if sec.tableID != tableIDPAT {
		return nil, errNotPAT
	}
	var programs []patProgram
	for i := 0; i+4 <= len(sec.data); i += 4 {
		num := uint16(sec.data[i])<<8 | uint16(sec.data[i+1])
		pid := uint16(sec.data[i+2]&0x1F)<<8 | uint16(sec.data[i+3])
		if num == 0 { // network information PID, not a program
			continue
		}
		programs = append(programs, patProgram{programNumber: num, pmtPID: pid})
	}

	return programs, nil
}

//nolint:unparam // The transport stream ID is part of the PAT format and is intentionally configurable.
func marshalPAT(transportStreamID uint16, programs []patProgram) ([]byte, error) {
	data := make([]byte, 0, len(programs)*4)
	for _, p := range programs {
		// The byte conversions intentionally serialize the high and low parts
		// of these bounded MPEG-TS fields.
		data = append(data,
			byte(p.programNumber>>8), byte(p.programNumber), //nolint:gosec
			0xE0|byte(p.pmtPID>>8), byte(p.pmtPID), //nolint:gosec
		)
	}

	return psiSection{tableID: tableIDPAT, tableIDExtension: transportStreamID, data: data}.marshal()
}
