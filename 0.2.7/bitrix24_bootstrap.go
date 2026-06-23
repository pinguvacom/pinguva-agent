package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

const bitrix24DiagnosticsServiceUnit = `[Unit]
Description=Pinguva Bitrix24 local diagnostics
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
User=root
Group=root
ExecStart=/usr/bin/pinguva-agent bitrix24 diagnostics --config-path /etc/pinguva-agent/bitrix24.json --output-path /var/lib/pinguva-agent/bitrix24-diagnostics.json
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ReadWritePaths=/var/lib/pinguva-agent
`

const bitrix24DiagnosticsTimerUnit = `[Unit]
Description=Run Pinguva Bitrix24 local diagnostics every minute

[Timer]
OnBootSec=90s
OnUnitActiveSec=60s
Persistent=true
Unit=pinguva-bitrix24-diagnostics.service

[Install]
WantedBy=timers.target
`

// bootstrap upgrades a pre-existing local Bitrix24 setup without asking for or
// changing the webhook. It runs only from an administrator's local command.
func runBitrix24Bootstrap(args []string, logger interface{ Printf(string, ...any) }) error {
	fs := flag.NewFlagSet("bitrix24 bootstrap", flag.ContinueOnError)
	configPathFlag := fs.String("config-path", defaultBitrix24ConfigPath(), "Local Bitrix24 config path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if runtime.GOOS != "linux" {
		return errors.New("Bitrix24 local diagnostics are available only on Linux")
	}
	if os.Geteuid() != 0 {
		return errors.New("run Bitrix24 bootstrap through sudo")
	}
	config, err := loadBitrix24Config(*configPathFlag)
	if err != nil {
		return err
	}
	if config == nil {
		if logger != nil {
			logger.Printf("Bitrix24 integration is not configured; diagnostics bootstrap skipped")
		}
		return nil
	}
	if enableBitrix24Diagnostics(config) {
		if err := saveBitrix24Config(*configPathFlag, *config); err != nil {
			return err
		}
	}
	if err := installBitrix24DiagnosticsTimer(); err != nil {
		return err
	}
	if logger != nil {
		logger.Printf("Bitrix24 diagnostics are enabled without changing the local webhook or REST profiles")
	}
	return nil
}

// enableBitrix24Diagnostics adds diagnostics only when an older local config
// did not have this section. Existing settings stay under the customer's control.
func enableBitrix24Diagnostics(config *bitrix24LocalConfig) bool {
	if config == nil || config.Diagnostics != nil {
		return false
	}
	config.Diagnostics = newBitrix24DiagnosticsLocalConfig(true, "")
	return true
}

func installBitrix24DiagnosticsTimer() error {
	for path, body := range map[string]string{
		"/etc/systemd/system/pinguva-bitrix24-diagnostics.service": bitrix24DiagnosticsServiceUnit,
		"/etc/systemd/system/pinguva-bitrix24-diagnostics.timer":   bitrix24DiagnosticsTimerUnit,
	} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return err
		}
	}
	for _, args := range [][]string{
		{"daemon-reload"},
		{"enable", "--now", "pinguva-bitrix24-diagnostics.timer"},
		{"start", "pinguva-bitrix24-diagnostics.service"},
	} {
		output, err := exec.Command("systemctl", args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("systemctl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}
