//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris
// +build aix darwin dragonfly freebsd linux netbsd openbsd solaris

package db

import "golang.org/x/sys/windows"

var econnrefused = windows.ECONNREFUSED
