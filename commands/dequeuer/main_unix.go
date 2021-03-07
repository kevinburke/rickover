//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris
// +build aix darwin dragonfly freebsd linux netbsd openbsd solaris

package main

import "golang.org/x/sys/unix"

var sigint = unix.SIGINT
var sigterm = unix.SIGTERM
