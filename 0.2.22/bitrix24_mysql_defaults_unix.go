//go:build aix || darwin || dragonfly || freebsd || illumos || ios || linux || netbsd || openbsd || solaris

package main

import (
	"os"
	"syscall"
)

// bitrix24MySQLDefaultsRootOwned is replaceable only by package tests. The
// production function rejects any defaults file not owned by root:root.
var bitrix24MySQLDefaultsRootOwned = func(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == 0 && stat.Gid == 0
}
