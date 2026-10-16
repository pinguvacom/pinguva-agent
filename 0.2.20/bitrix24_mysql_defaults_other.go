//go:build !(aix || darwin || dragonfly || freebsd || illumos || ios || linux || netbsd || openbsd || solaris)

package main

import "os"

// Bitrix24 local diagnostics run on Linux. Other targets must never trust a
// POSIX-only root defaults file because ownership cannot be verified there.
var bitrix24MySQLDefaultsRootOwned = func(os.FileInfo) bool {
	return false
}
