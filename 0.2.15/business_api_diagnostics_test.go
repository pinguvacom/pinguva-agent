package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNormalizeAgentBusinessAPIPathRedactsDynamicSegments(t *testing.T) {
	got := normalizeAgentBusinessAPIPath("https://api.example.test/api/orders/550e8400-e29b-41d4-a716-446655440000/status?token=secret")
	if got != "/api/orders/:value/status" {
		t.Fatalf("unexpected path: %q", got)
	}
	got = normalizeAgentBusinessAPIPath("/api/loyalty/services/consumers/:uuid/points/increase")
	if got != "/api/loyalty/services/consumers/:value/points/increase" {
		t.Fatalf("unexpected placeholder path: %q", got)
	}
}

func TestParseBusinessAPIAccessLogLineWithOptionalLatency(t *testing.T) {
	line := `203.0.113.10 - - [23/Jun/2026:22:55:00 +0500] "POST /api/orders/123/status?token=secret HTTP/1.1" 201 123 "-" "agent" 0.245`
	got, ok := parseBusinessAPIAccessLogLine(line)
	if !ok {
		t.Fatal("expected log line to parse")
	}
	if got.Source != "203.0.113.10" || got.Method != "POST" || got.Path != "/api/orders/:value/status" || got.Status != 201 {
		t.Fatalf("unexpected parsed line: %+v", got)
	}
	if !got.HasLatency || got.LatencyMS != 245 {
		t.Fatalf("expected 245ms latency, got %+v", got)
	}
}

func TestCollectBusinessAPIAccessLogBucketCountsOnlyConfiguredRoutes(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "access.log")
	stamp := "23/Jun/2026:22:55:00 +0500"
	body := `203.0.113.10 - - [` + stamp + `] "POST /api/orders/123/status HTTP/1.1" 201 123 "-" "agent" 0.200
` +
		`203.0.113.10 - - [` + stamp + `] "POST /api/orders/456/status HTTP/1.1" 503 123 "-" "agent" 0.900
` +
		`203.0.113.10 - - [` + stamp + `] "GET /health HTTP/1.1" 200 12 "-" "agent" 0.010
`
	if err := os.WriteFile(logPath, []byte(body), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}
	old := defaultBusinessAPIAccessLogPaths
	defaultBusinessAPIAccessLogPaths = func() []string { return []string{logPath} }
	defer func() { defaultBusinessAPIAccessLogPaths = old }()
	from := time.Date(2026, 6, 23, 17, 54, 0, 0, time.UTC)
	until := from.Add(2 * time.Minute)
	bucket := collectBusinessAPIAccessLogBucket([]agentBusinessAPIEndpointWatch{{ID: "r1", Method: "POST", Path: "/api/orders/:order_id/status"}}, from, until)
	if bucket == nil || bucket.Status != "ok" || len(bucket.Routes) != 1 {
		t.Fatalf("unexpected bucket: %+v", bucket)
	}
	route := bucket.Routes[0]
	if route.Requests != 2 || route.Successes != 1 || route.ServerErrors != 1 || !route.LatencyAvailable || route.AverageLatencyMS != 550 || route.MaxLatencyMS != 900 {
		t.Fatalf("unexpected route metrics: %+v", route)
	}
	if len(bucket.Sources) != 1 || bucket.Sources[0].Source != "203.0.113.xxx" {
		t.Fatalf("expected masked source, got %+v", bucket.Sources)
	}
}

func TestCollectBusinessAPIAccessLogBucketReadsCustomLogAndAnyMethod(t *testing.T) {
	tmp := t.TempDir()
	logDir := filepath.Join(tmp, "log")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	logPath := filepath.Join(logDir, "api.access.log")
	stamp := "23/Jun/2026:22:55:00 +0500"
	body := `203.0.113.10 - - [` + stamp + `] "POST /api/orders/123 HTTP/1.1" 201 123 "-" "agent" 0.200
` +
		`203.0.113.10 - - [` + stamp + `] "DELETE /api/orders/123 HTTP/1.1" 503 123 "-" "agent" 0.900
`
	if err := os.WriteFile(logPath, []byte(body), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}
	old := defaultBusinessAPIAccessLogPaths
	defaultBusinessAPIAccessLogPaths = func() []string { return nil }
	defer func() { defaultBusinessAPIAccessLogPaths = old }()
	from := time.Date(2026, 6, 23, 17, 54, 0, 0, time.UTC)
	bucket := collectBusinessAPIAccessLogBucket([]agentBusinessAPIEndpointWatch{{ID: "any", Method: "ANY", Path: "/api/orders/:id", LogPath: logPath}}, from, from.Add(2*time.Minute))
	if bucket == nil || bucket.Status != "ok" || len(bucket.Routes) != 1 {
		t.Fatalf("unexpected custom log bucket: %+v", bucket)
	}
	route := bucket.Routes[0]
	if route.Requests != 2 || route.Successes != 1 || route.ServerErrors != 1 || route.AverageLatencyMS != 550 || route.MaxLatencyMS != 900 {
		t.Fatalf("unexpected any-method metrics: %+v", route)
	}
}

func TestBusinessAPIAccessLogPathSafety(t *testing.T) {
	if !isSafeBusinessAPIAccessLogPath("/ilab/log/request_orders.txt") {
		t.Fatal("expected application log path to be accepted")
	}
	if isSafeBusinessAPIAccessLogPath("/etc/passwd") {
		t.Fatal("did not expect arbitrary system file to be accepted")
	}
	if got := normalizeAgentBusinessAPILogPath("/var/log/../etc/passwd"); got != "" {
		t.Fatalf("expected traversal path to be rejected, got %q", got)
	}
}
