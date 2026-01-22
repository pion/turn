// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build windows
// +build windows

package turn

import (
	"syscall"
)

type Handle = syscall.Handle
