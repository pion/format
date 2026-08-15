// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package mpegts implements MPEG-2 Transport Stream readers and writers.
package mpegts

const (
	packetSize = 188
	syncByte   = 0x47
	pidPAT     = 0x0000
	pidNull    = 0x1FFF
)
