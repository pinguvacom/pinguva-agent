package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestCollectBitrix24AccessLogSummaryRedactsAndAggregates(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	logPath := filepath.Join(t.TempDir(), "access.log")
	stamp := now.Format("02/Jan/2006:15:04:05 -0700")
	lines := []string{
		`20.86.251.108 - - [` + stamp + `] "GET /api/accruedpoints?customer=42&token=secret HTTP/1.1" 200 123 "-" "test"`,
		`20.86.251.108 - - [` + stamp + `] "POST /api/contact/550e8400-e29b-41d4-a716-446655440000 HTTP/1.1" 503 123 "-" "test"`,
		`20.86.251.108 - - [` + stamp + `] "GET /assets/app.js HTTP/1.1" 200 123 "-" "test"`,
	}
	if err := os.WriteFile(logPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("write access log: %v", err)
	}

	summary := collectBitrix24AccessLogSummary([]string{logPath}, 15, now)
	if summary == nil || summary.Status != "ok" {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if summary.Requests != 2 || summary.Errors5xx != 1 || summary.UniqueSources != 1 {
		t.Fatalf("unexpected aggregate: %+v", summary)
	}
	if len(summary.TopEndpoints) != 2 {
		t.Fatalf("unexpected endpoints: %+v", summary.TopEndpoints)
	}
	for _, endpoint := range summary.TopEndpoints {
		if strings.Contains(endpoint.Path, "token") || strings.Contains(endpoint.Path, "customer") || strings.Contains(endpoint.Path, "550e8400") {
			t.Fatalf("sensitive route data leaked into summary: %+v", endpoint)
		}
	}
	if len(summary.TopSources) != 1 || summary.TopSources[0].Source != "20.86.251.xxx" {
		t.Fatalf("expected masked source, got %+v", summary.TopSources)
	}
}

func TestParseBitrix24QueryFingerprintsRejectsRawSQL(t *testing.T) {
	items := parseBitrix24QueryFingerprints([]string{
		"crm_contact_case_insensitive_lookup\t138\t93",
		"SELECT * FROM b_user WHERE email='private@example.com'\t9\t60",
	})
	if len(items) != 1 {
		t.Fatalf("expected only classified query group, got %+v", items)
	}
	if items[0].Kind != "crm_contact_case_insensitive_lookup" || items[0].Count != 138 || items[0].MaxDurationSec != 93 {
		t.Fatalf("unexpected fingerprint: %+v", items[0])
	}
}

func TestNormalizeBitrix24EndpointPathRedactsSensitiveSegments(t *testing.T) {
	value := normalizeBitrix24EndpointPath("/api/customer/user@example.com/7f0c0e6138a1a0ab6d9c0001")
	if value != "/api/customer/:value/:id" {
		t.Fatalf("unexpected normalized endpoint: %q", value)
	}
}
