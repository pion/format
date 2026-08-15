// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package mpegts

// CRC-32/MPEG-2: poly 0x04C11DB7, init 0xFFFFFFFF, no reflection, no xor-out.
// hash/crc32 only implements the reflected variant, so we build our own table.
//
//nolint:gochecknoglobals
var crc32Table = func() [256]uint32 {
	var table [256]uint32
	for i := range table {
		crc := uint32(i) << 24 //nolint:gosec
		for range 8 {
			if crc&0x80000000 != 0 {
				crc = crc<<1 ^ 0x04C11DB7
			} else {
				crc <<= 1
			}
		}
		table[i] = crc
	}

	return table
}()

func crc32MPEG(data []byte) uint32 {
	crc := uint32(0xFFFFFFFF)
	for _, b := range data {
		crc = crc<<8 ^ crc32Table[byte(crc>>24)^b]
	}

	return crc
}
