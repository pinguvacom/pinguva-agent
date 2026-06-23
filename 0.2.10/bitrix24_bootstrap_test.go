package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestEnableBitrix24DiagnosticsPreservesWebhookAndProfiles(t *testing.T) {
	config := &bitrix24LocalConfig{
		BaseURL:    "https://crm.example.test",
		WebhookURL: "https://crm.example.test/rest/1/webhook-secret-placeholder",
		Profiles:   []string{"basic", "crm_contacts"},
	}
	if !enableBitrix24Diagnostics(config) {
		t.Fatal("expected diagnostics to be added to an older local config")
	}
	if config.WebhookURL != "https://crm.example.test/rest/1/webhook-secret-placeholder" {
		t.Fatalf("bootstrap changed the local webhook: %q", config.WebhookURL)
	}
	if !reflect.DeepEqual(config.Profiles, []string{"basic", "crm_contacts"}) {
		t.Fatalf("bootstrap changed REST profiles: %#v", config.Profiles)
	}
	if config.Diagnostics == nil || !config.Diagnostics.Enabled || config.Diagnostics.WindowMinutes != bitrix24DiagnosticsDefaultWindowMinutes {
		t.Fatalf("expected enabled default diagnostics, got %#v", config.Diagnostics)
	}
	if !reflect.DeepEqual(config.Diagnostics.AccessLogPaths, defaultBitrix24AccessLogPaths()) {
		t.Fatalf("expected bounded standard access-log paths, got %#v", config.Diagnostics.AccessLogPaths)
	}
}

func TestEnableBitrix24DiagnosticsPreservesExistingCustomerSetting(t *testing.T) {
	diagnostics := &bitrix24DiagnosticsLocalConfig{Enabled: false, AccessLogPaths: []string{"/srv/bitrix/access.log"}, WindowMinutes: 30}
	config := &bitrix24LocalConfig{WebhookURL: "https://crm.example.test/rest/1/webhook-secret-placeholder", Profiles: []string{"basic"}, Diagnostics: diagnostics}
	if enableBitrix24Diagnostics(config) {
		t.Fatal("bootstrap must not override an explicit diagnostics setting")
	}
	if config.Diagnostics != diagnostics {
		t.Fatal("bootstrap replaced the existing diagnostics configuration")
	}
}

func TestBitrix24DiagnosticsSystemdUnitHasNoSecretAndIsRestricted(t *testing.T) {
	unit := bitrix24DiagnosticsServiceUnit
	for _, expected := range []string{
		"ExecStart=/usr/bin/pinguva-agent bitrix24 diagnostics",
		"NoNewPrivileges=true",
		"PrivateTmp=true",
		"ProtectSystem=full",
		"ReadWritePaths=/var/lib/pinguva-agent",
	} {
		if !strings.Contains(unit, expected) {
			t.Fatalf("missing systemd hardening directive %q", expected)
		}
	}
	if strings.Contains(strings.ToLower(unit), "webhook") || strings.Contains(unit, "rest/") {
		t.Fatalf("systemd unit must not contain a Bitrix24 secret: %q", unit)
	}
}
