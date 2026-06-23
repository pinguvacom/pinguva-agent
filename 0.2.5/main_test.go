package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeBitrix24WebhookURL(t *testing.T) {
	value, err := normalizeBitrix24WebhookURL("https://crm.example.kz/rest/1/webhook-code-placeholder/user.current")
	if err != nil {
		t.Fatalf("normalize webhook: %v", err)
	}
	if value != "https://crm.example.kz/rest/1/webhook-code-placeholder" {
		t.Fatalf("unexpected webhook URL: %s", value)
	}
	if _, err := normalizeBitrix24WebhookURL("http://crm.example.kz/rest/1/webhook-code-placeholder"); err == nil {
		t.Fatalf("expected HTTPS validation error")
	}
}

func TestValidateAgentConfigRejectsEmptyServer(t *testing.T) {
	if err := validateAgentConfig(agentConfig{ServerURL: ""}); err == nil {
		t.Fatalf("expected missing server validation error")
	}
}

func TestSafeBitrix24ErrorRedactsWebhookSecret(t *testing.T) {
	config := &bitrix24LocalConfig{WebhookURL: "https://crm.example.kz/rest/1/webhook-code-placeholder"}
	message := safeBitrix24Error("request failed https://crm.example.kz/rest/1/webhook-code-placeholder/user.current webhook-code-placeholder", config)
	if strings.Contains(message, "webhook-code-placeholder") {
		t.Fatalf("secret leaked in error: %s", message)
	}
	if !strings.Contains(message, "[redacted]") {
		t.Fatalf("expected redacted marker: %s", message)
	}
}

func TestCheckBitrix24Webhook(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/rest/1/webhook-code-placeholder/user.current" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"ID": "1"}})
	}))
	t.Cleanup(server.Close)

	status := checkBitrix24Webhook(server.Client(), bitrix24LocalConfig{
		WebhookURL: server.URL + "/rest/1/webhook-code-placeholder",
		Profiles:   []string{"basic"},
	})
	if status == nil || !status.Configured || status.Status != "ok" {
		t.Fatalf("unexpected status: %+v", status)
	}
	if status.ResponseMS < 0 {
		t.Fatalf("unexpected response time: %d", status.ResponseMS)
	}
	if status.Summary == nil || status.Summary.MethodCount != 1 || status.Summary.OKCount != 1 {
		t.Fatalf("unexpected summary: %+v", status.Summary)
	}
}

func TestCheckBitrix24WebhookProfilesSummary(t *testing.T) {
	seen := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path]++
		switch r.URL.Path {
		case "/rest/1/webhook-code-placeholder/user.current", "/rest/1/webhook-code-placeholder/scope":
			_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"ok": true}})
		case "/rest/1/webhook-code-placeholder/crm.item.list":
			_ = json.NewEncoder(w).Encode(map[string]any{"result": []any{map[string]any{"ID": "1"}}, "total": 7})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	status := checkBitrix24Webhook(server.Client(), bitrix24LocalConfig{
		WebhookURL: server.URL + "/rest/1/webhook-code-placeholder",
		Profiles:   []string{"basic", "scope", "crm_deals"},
	})
	if status == nil || status.Status != "ok" || status.Summary == nil {
		t.Fatalf("unexpected status: %+v", status)
	}
	if status.Summary.MethodCount != 3 || status.Summary.OKCount != 3 || status.Summary.ErrorCount != 0 {
		t.Fatalf("unexpected summary: %+v", status.Summary)
	}
	if len(status.Methods) != 3 {
		t.Fatalf("unexpected methods: %+v", status.Methods)
	}
	if status.Methods[2].ResultCount != 7 {
		t.Fatalf("expected safe result count, got %+v", status.Methods[2])
	}
	if seen["/rest/1/webhook-code-placeholder/user.current"] != 1 || seen["/rest/1/webhook-code-placeholder/scope"] != 1 || seen["/rest/1/webhook-code-placeholder/crm.item.list"] != 1 {
		t.Fatalf("unexpected calls: %+v", seen)
	}
}

func TestCheckBitrix24MethodDiscoveryChecksUserGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/rest/1/webhook-code-placeholder/method.get" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var payload struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload.Name != "user.get" {
			t.Fatalf("expected method.get target user.get, got %q", payload.Name)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": map[string]any{"isExisting": true, "isAvailable": true},
		})
	}))
	t.Cleanup(server.Close)

	status := checkBitrix24Webhook(server.Client(), bitrix24LocalConfig{
		WebhookURL: server.URL + "/rest/1/webhook-code-placeholder",
		Profiles:   []string{"method_discovery"},
	})
	if status == nil || status.Status != "ok" || len(status.Methods) != 1 {
		t.Fatalf("unexpected status: %+v", status)
	}
	if status.Methods[0].Method != "method.get" || status.Methods[0].Name != "Проверка доступности user.get" {
		t.Fatalf("unexpected method status: %+v", status.Methods[0])
	}
}
