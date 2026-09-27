// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package util provides auxiliary functions internally used in format packages.
package util //nolint:revive

import "github.com/pion/randutil"

// Use global random generator to properly seed by crypto grade random.
var globalMathRandomGenerator = randutil.NewMathRandomGenerator() // nolint:gochecknoglobals

// RandUint32 generates a mathematical random uint32.
func RandUint32() uint32 {
	return globalMathRandomGenerator.Uint32()
}
