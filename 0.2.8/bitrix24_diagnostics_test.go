package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBitrix24MySQLDefaultsOptionUsesOnlySafeRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "my.cnf")
	if err := os.WriteFile(path, []byte("[client]\nuser=root\npassword=placeholder\n"), 0o600); err != nil {
		t.Fatalf("write defaults file: %v", err)
	}
	if got, want := bitrix24MySQLDefaultsOption(path), "--defaults-extra-file="+path; got != want {
		t.Fatalf("safe defaults option = %q, want %q", got, want)
	}
	if err := os.Chmod(path, 0o620); err != nil {
		t.Fatalf("chmod defaults file: %v", err)
	}
	if got := bitrix24MySQLDefaultsOption(path); got != "" {
		t.Fatalf("group-writable defaults file must be rejected, got %q", got)
	}
	link := filepath.Join(t.TempDir(), "my.cnf")
	if err := os.Symlink(path, link); err != nil {
		t.Fatalf("create defaults symlink: %v", err)
	}
	if got := bitrix24MySQLDefaultsOption(link); got != "" {
		t.Fatalf("defaults symlink must be rejected, got %q", got)
	}
}
