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
	line := `20.86.251.108 - - [23/Jun/2026:22:55:00 +0500] "POST /api/orders/123/status?token=secret HTTP/1.1" 201 123 "-" "agent" 0.245`
	got, ok := parseBusinessAPIAccessLogLine(line)
	if !ok {
		t.Fatal("expected log line to parse")
	}
	if got.Source != "20.86.251.108" || got.Method != "POST" || got.Path != "/api/orders/:value/status" || got.Status != 201 {
		t.Fatalf("unexpected parsed line: %+v", got)
	}
	if !got.HasLatency || got.LatencyMS != 245 {
		t.Fatalf("expected 245ms latency, got %+v", got)
	}
}

func TestParseBusinessAPIAccessLogLineWithStatusBeforeRequest(t *testing.T) {
	line := `10.90.90.34 - - [08/Oct/2026:21:40:58 +0500 - 0.146] 200 "POST /api/customer.segment HTTP/1.1" 272 "-" "GuzzleHttp/7" "-"`
	got, ok := parseBusinessAPIAccessLogLine(line)
	if !ok {
		t.Fatal("expected custom access log line to parse")
	}
	if got.Source != "10.90.90.34" || got.Method != "POST" || got.Path != "/api/customer.segment" || got.Status != 200 {
		t.Fatalf("unexpected parsed custom line: %+v", got)
	}
	if !got.HasLatency || got.LatencyMS != 146 {
		t.Fatalf("expected 146ms latency, got %+v", got)
	}
}

func TestParseBusinessAPIAccessLogLineWithoutRequestQuotes(t *testing.T) {
	line := `10.90.90.34 - - [09/Oct/2026:13:30:46 +0500] POST /api/customer.change HTTP/1.1 200 184 0.141`
	got, ok := parseBusinessAPIAccessLogLine(line)
	if !ok {
		t.Fatal("expected unquoted access log line to parse")
	}
	if got.Method != "POST" || got.Path != "/api/customer.change" || got.Status != 200 || got.LatencyMS != 141 {
		t.Fatalf("unexpected parsed unquoted line: %+v", got)
	}
}

func TestParseBusinessAPIJSONAccessLogLine(t *testing.T) {
	line := `{"remote_addr":"10.90.90.34","time_local":"2026-10-09T13:30:46+05:00","request_method":"POST","request_uri":"/api/customer.change","status":200,"request_time":0.141}`
	got, ok := parseBusinessAPIAccessLogLine(line)
	if !ok {
		t.Fatal("expected JSON access log line to parse")
	}
	if got.Source != "10.90.90.34" || got.Method != "POST" || got.Path != "/api/customer.change" || got.Status != 200 || got.LatencyMS != 141 {
		t.Fatalf("unexpected parsed JSON line: %+v", got)
	}
}

func TestParseBusinessAPIPHPLogBlock(t *testing.T) {
	lines := []string{
		"----------------------------------------------------- 13.10.2026 21:13:33",
		"uuid - a2f9134d-4bc7-4fca-81eb-9d2ffaf61389",
		"Код ответа - 200",
		"Ответ - Array",
		"(",
		"    [data] => Array",
		")",
		"Ошибка -",
		"headers - Array",
		"(",
		"    [url] => https://api.example.test/api/loyalty/services/consumers/00000000-0000-4000-8000-000000000001/tier",
		"    [http_code] => 200",
		"    [total_time] => 0.843471",
		")",
	}
	items := parseBusinessAPILogLines(lines)
	if len(items) != 1 {
		t.Fatalf("expected one PHP log event, got %+v", items)
	}
	item := items[0]
	if item.Method != "ANY" || item.Path != "/api/loyalty/services/consumers/:value/tier" || item.Status != 200 {
		t.Fatalf("unexpected PHP log event: %+v", item)
	}
	if !item.HasLatency || item.LatencyMS != 843 {
		t.Fatalf("expected 843ms latency, got %+v", item)
	}
	if item.At.IsZero() {
		t.Fatal("expected PHP log timestamp")
	}
}

func TestCollectBusinessAPIAccessLogBucketReadsPHPLogWithConfiguredRoute(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "request_points.txt")
	stamp := "13.10.2026 21:13:33"
	body := "----------------------------------------------------- " + stamp + "\n" +
		"Код ответа - 200\n" +
		"headers - Array\n" +
		"(\n" +
		"    [url] => https://api.example.test/api/loyalty/services/consumers/00000000-0000-4000-8000-000000000001/tier\n" +
		"    [http_code] => 200\n" +
		"    [total_time] => 0.843471\n" +
		")\n"
	if err := os.WriteFile(logPath, []byte(body), 0o600); err != nil {
		t.Fatalf("write PHP log: %v", err)
	}
	old := defaultBusinessAPIAccessLogPaths
	oldConfigFiles := businessAPIAccessLogConfigFiles
	oldConfigDirs := businessAPIAccessLogConfigDirs
	defaultBusinessAPIAccessLogPaths = func() []string { return nil }
	businessAPIAccessLogConfigFiles = func() []string { return nil }
	businessAPIAccessLogConfigDirs = func() []string { return nil }
	defer func() {
		defaultBusinessAPIAccessLogPaths = old
		businessAPIAccessLogConfigFiles = oldConfigFiles
		businessAPIAccessLogConfigDirs = oldConfigDirs
	}()
	from := time.Date(2026, 10, 13, 21, 13, 0, 0, time.Local)
	bucket := collectBusinessAPIAccessLogBucket([]agentBusinessAPIEndpointWatch{{
		ID: "points", Method: "GET", Path: "/api/loyalty/services/consumers/:uuid/tier", LogPath: logPath,
	}}, from, from.Add(time.Minute))
	if bucket == nil || bucket.Status != "ok" || len(bucket.Routes) != 1 {
		t.Fatalf("unexpected PHP log bucket: %+v", bucket)
	}
	route := bucket.Routes[0]
	if route.Requests != 1 || route.Successes != 1 || route.AverageLatencyMS != 843 || route.MaxLatencyMS != 843 {
		t.Fatalf("unexpected PHP log route metrics: %+v", route)
	}
}

func TestCollectBusinessAPIAccessLogBucketMarksUnknownNonEmptyFormatUnavailable(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "request_points.txt")
	if err := os.WriteFile(logPath, []byte("application output without a request record\n"), 0o600); err != nil {
		t.Fatalf("write unknown log: %v", err)
	}
	old := defaultBusinessAPIAccessLogPaths
	oldConfigFiles := businessAPIAccessLogConfigFiles
	oldConfigDirs := businessAPIAccessLogConfigDirs
	defaultBusinessAPIAccessLogPaths = func() []string { return nil }
	businessAPIAccessLogConfigFiles = func() []string { return nil }
	businessAPIAccessLogConfigDirs = func() []string { return nil }
	defer func() {
		defaultBusinessAPIAccessLogPaths = old
		businessAPIAccessLogConfigFiles = oldConfigFiles
		businessAPIAccessLogConfigDirs = oldConfigDirs
	}()
	from := time.Date(2026, 10, 13, 21, 13, 0, 0, time.Local)
	bucket := collectBusinessAPIAccessLogBucket([]agentBusinessAPIEndpointWatch{{
		ID: "points", Method: "GET", Path: "/api/points", LogPath: logPath,
	}}, from, from.Add(time.Minute))
	if bucket == nil || bucket.Status != "unavailable" {
		t.Fatalf("expected unsupported non-empty log to be unavailable, got %+v", bucket)
	}
}

func TestCollectBusinessAPIAccessLogBucketCountsOnlyConfiguredRoutes(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "access.log")
	stamp := "23/Jun/2026:22:55:00 +0500"
	body := `20.86.251.108 - - [` + stamp + `] "POST /api/orders/123/status HTTP/1.1" 201 123 "-" "agent" 0.200
` +
		`20.86.251.108 - - [` + stamp + `] "POST /api/orders/456/status HTTP/1.1" 503 123 "-" "agent" 0.900
` +
		`20.86.251.108 - - [` + stamp + `] "GET /health HTTP/1.1" 200 12 "-" "agent" 0.010
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
	if len(bucket.Sources) != 1 || bucket.Sources[0].Source != "20.86.251.xxx" {
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
	body := `20.86.251.108 - - [` + stamp + `] "POST /api/orders/123 HTTP/1.1" 201 123 "-" "agent" 0.200
` +
		`20.86.251.108 - - [` + stamp + `] "DELETE /api/orders/123 HTTP/1.1" 503 123 "-" "agent" 0.900
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

func TestCollectBusinessAPIAccessLogBucketMatchesNamedPathParameters(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "access.log")
	stamp := "09/Oct/2026:13:30:46 +0500"
	body := `20.113.102.83 - - [` + stamp + ` - 0.050] 200 "GET /api/packcode.check/Kz5hwa7k9 HTTP/1.1" 57 "-" "client" "-"
`
	if err := os.WriteFile(logPath, []byte(body), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}
	old := defaultBusinessAPIAccessLogPaths
	defaultBusinessAPIAccessLogPaths = func() []string { return []string{logPath} }
	defer func() { defaultBusinessAPIAccessLogPaths = old }()
	from := time.Date(2026, 10, 9, 8, 30, 0, 0, time.UTC)
	bucket := collectBusinessAPIAccessLogBucket([]agentBusinessAPIEndpointWatch{{ID: "packcode", Method: "GET", Path: "/api/packcode.check/:code"}}, from, from.Add(2*time.Minute))
	if bucket == nil || len(bucket.Routes) != 1 || bucket.Routes[0].Requests != 1 {
		t.Fatalf("expected named path parameter to match, got %+v", bucket)
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

func TestParseBusinessAPIAccessLogConfigFindsNginxAndApachePaths(t *testing.T) {
	got := parseBusinessAPIAccessLogConfig(`
access_log /srv/app/logs/api_access.log main;
CustomLog "/srv/httpd/access_log" combined
TransferLog /srv/httpd/transfer.log
access_log off;
`)
	if len(got) != 3 {
		t.Fatalf("expected three access log paths, got %q", got)
	}
	for _, want := range []string{"/srv/app/logs/api_access.log", "/srv/httpd/access_log", "/srv/httpd/transfer.log"} {
		found := false
		for _, value := range got {
			if value == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected %q in %q", want, got)
		}
	}
}
