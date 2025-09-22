// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build windows
// +build windows

package main

import (
	"syscall"
)

type Handle = syscall.Handle
