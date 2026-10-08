package main

import (
	"bufio"
	"errors"
	"io"
	"net/url"
	"os"
	"path"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	businessAPIDiagnosticsSchemaVersion = 1
	businessAPIDiagnosticsBucketSeconds = 60
	businessAPIDiagnosticsMaxRoutes     = 200
	businessAPIDiagnosticsMaxLogBytes   = int64(32 << 20)
	businessAPIAccessLogConfigMaxBytes  = int64(1 << 20)
	businessAPIAccessLogConfigMaxFiles  = 64
)

var (
	businessAPIAccessPrefixPattern = regexp.MustCompile(`^(\S+)\s+\S+\s+\S+\s+\[([^\]]+)\]\s+"([^"]*)"\s+(\d{3})\s+(\S+)(.*)$`)
	businessAPICustomAccessPattern = regexp.MustCompile(`^(\S+)\s+\S+\s+\S+\s+\[([^\]]+)\]\s+(\d{3})\s+"([^"]*)"\s+(\S+)(.*)$`)
	businessAPIDynamicUUIDPattern  = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	businessAPIDynamicHexPattern   = regexp.MustCompile(`(?i)^[0-9a-f]{16,}$`)
	businessAPIDynamicNumPattern   = regexp.MustCompile(`^\d+$`)
)

type agentBusinessAPIEndpointWatch struct {
	ID               string `json:"id,omitempty"`
	Name             string `json:"name,omitempty"`
	Method           string `json:"method,omitempty"`
	Path             string `json:"path"`
	LogPath          string `json:"logPath,omitempty"`
	TrafficExpected  bool   `json:"trafficExpected,omitempty"`
	ExpectedStatuses []int  `json:"expectedStatuses,omitempty"`
}

type agentBusinessAPIDiagnosticBucket struct {
	SchemaVersion int                                `json:"schemaVersion"`
	EventID       string                             `json:"eventId"`
	BucketStart   time.Time                          `json:"bucketStart"`
	BucketSeconds int                                `json:"bucketSeconds"`
	Source        string                             `json:"source"`
	Status        string                             `json:"status"`
	LatencyStatus string                             `json:"latencyStatus,omitempty"`
	Routes        []agentBusinessAPIDiagnosticRoute  `json:"routes,omitempty"`
	Sources       []agentBusinessAPIDiagnosticSource `json:"sources,omitempty"`
}

type agentBusinessAPIDiagnosticRoute struct {
	WatchID          string `json:"watchId,omitempty"`
	Name             string `json:"name,omitempty"`
	Method           string `json:"method"`
	Path             string `json:"path"`
	Requests         int    `json:"requests"`
	Successes        int    `json:"successes"`
	BusinessRejects  int    `json:"businessRejects"`
	ClientErrors     int    `json:"clientErrors"`
	ServerErrors     int    `json:"serverErrors"`
	Unexpected       int    `json:"unexpected"`
	LatencyAvailable bool   `json:"latencyAvailable"`
	AverageLatencyMS int64  `json:"averageLatencyMs,omitempty"`
	MaxLatencyMS     int64  `json:"maxLatencyMs,omitempty"`
}

type agentBusinessAPIDiagnosticSource struct {
	Source   string `json:"source"`
	Requests int    `json:"requests"`
	Errors   int    `json:"errors"`
}

type businessAPILogLine struct {
	Source     string
	At         time.Time
	Method     string
	Path       string
	Status     int
	LatencyMS  int64
	HasLatency bool
}

type businessAPIRouteCounter struct {
	watch           agentBusinessAPIEndpointWatch
	requests        int
	successes       int
	businessRejects int
	clientErrors    int
	serverErrors    int
	unexpected      int
	latencyCount    int
	latencyTotalMS  int64
	latencyMaxMS    int64
}

type businessAPISourceCounter struct {
	requests int
	errors   int
}

func collectBusinessAPIDiagnostics(watches []agentBusinessAPIEndpointWatch, now time.Time) []agentBusinessAPIDiagnosticBucket {
	watches = normalizeAgentBusinessAPIEndpointWatches(watches)
	if len(watches) == 0 || runtimeGOOS() != "linux" {
		return nil
	}
	bucketStart := now.UTC().Truncate(time.Minute).Add(-time.Minute)
	bucketEnd := bucketStart.Add(time.Minute)
	bucket := collectBusinessAPIAccessLogBucket(watches, bucketStart, bucketEnd)
	if bucket == nil {
		return nil
	}
	return []agentBusinessAPIDiagnosticBucket{*bucket}
}

func runtimeGOOS() string { return runtimeGOOSValue }

var runtimeGOOSValue = func() string { return runtime.GOOS }()

func collectBusinessAPIAccessLogBucket(watches []agentBusinessAPIEndpointWatch, from, until time.Time) *agentBusinessAPIDiagnosticBucket {
	paths := businessAPIAccessLogPaths(watches)
	if len(paths) == 0 || !until.After(from) {
		return nil
	}
	byKey := map[string]agentBusinessAPIEndpointWatch{}
	for _, watch := range watches {
		byKey[businessAPIWatchKey(watch.Method, watch.Path)] = watch
	}
	counters := make(map[string]*businessAPIRouteCounter, len(byKey))
	for key, watch := range byKey {
		counters[key] = &businessAPIRouteCounter{watch: watch}
	}
	sources := map[string]businessAPISourceCounter{}
	readable := false
	latencySeen := false
	for _, path := range paths {
		lines, err := readBusinessAPILogTail(path, businessAPIDiagnosticsMaxLogBytes)
		if err != nil {
			continue
		}
		readable = true
		for _, line := range lines {
			item, ok := parseBusinessAPIAccessLogLine(line)
			if !ok || item.At.Before(from) || !item.At.Before(until) {
				continue
			}
			pathValue := normalizeAgentBusinessAPIPath(item.Path)
			matched := []*businessAPIRouteCounter{counters[businessAPIWatchKey(item.Method, pathValue)]}
			if wildcard := counters[businessAPIWatchKey("ANY", pathValue)]; wildcard != nil {
				matched = append(matched, wildcard)
			}
			matched = compactBusinessAPIRouteCounters(matched)
			if len(matched) == 0 {
				continue
			}
			for _, counter := range matched {
				counter.requests++
				classifyBusinessAPIStatus(counter, item.Status)
				if item.HasLatency {
					latencySeen = true
					counter.latencyCount++
					counter.latencyTotalMS += item.LatencyMS
					if item.LatencyMS > counter.latencyMaxMS {
						counter.latencyMaxMS = item.LatencyMS
					}
				}
			}
			masked := maskBusinessAPISource(item.Source)
			if masked != "" {
				source := sources[masked]
				source.requests++
				if item.Status >= 500 {
					source.errors++
				}
				sources[masked] = source
			}
		}
	}
	if !readable {
		return &agentBusinessAPIDiagnosticBucket{SchemaVersion: businessAPIDiagnosticsSchemaVersion, EventID: deterministicBusinessAPIBucketEventID(from), BucketStart: from.UTC(), BucketSeconds: businessAPIDiagnosticsBucketSeconds, Source: "access_log", Status: "unavailable", LatencyStatus: "unavailable"}
	}
	routes := make([]agentBusinessAPIDiagnosticRoute, 0, len(counters))
	for _, counter := range counters {
		avg := int64(0)
		if counter.latencyCount > 0 {
			avg = counter.latencyTotalMS / int64(counter.latencyCount)
		}
		routes = append(routes, agentBusinessAPIDiagnosticRoute{WatchID: counter.watch.ID, Name: counter.watch.Name, Method: counter.watch.Method, Path: counter.watch.Path, Requests: counter.requests, Successes: counter.successes, BusinessRejects: counter.businessRejects, ClientErrors: counter.clientErrors, ServerErrors: counter.serverErrors, Unexpected: counter.unexpected, LatencyAvailable: counter.latencyCount > 0, AverageLatencyMS: avg, MaxLatencyMS: counter.latencyMaxMS})
	}
	sort.Slice(routes, func(left, right int) bool {
		if routes[left].Requests != routes[right].Requests {
			return routes[left].Requests > routes[right].Requests
		}
		return routes[left].Path < routes[right].Path
	})
	return &agentBusinessAPIDiagnosticBucket{SchemaVersion: businessAPIDiagnosticsSchemaVersion, EventID: deterministicBusinessAPIBucketEventID(from), BucketStart: from.UTC(), BucketSeconds: businessAPIDiagnosticsBucketSeconds, Source: "access_log", Status: "ok", LatencyStatus: businessAPILatencyStatus(latencySeen), Routes: routes, Sources: topBusinessAPISources(sources, 10)}
}

func classifyBusinessAPIStatus(counter *businessAPIRouteCounter, status int) {
	if counter == nil {
		return
	}
	expected := map[int]struct{}{}
	for _, value := range counter.watch.ExpectedStatuses {
		expected[value] = struct{}{}
	}
	expectedStatus := len(expected) == 0
	if _, ok := expected[status]; ok {
		expectedStatus = true
	}
	switch {
	case status >= 500:
		counter.serverErrors++
	case !expectedStatus:
		counter.unexpected++
	case status == 200 || status == 201 || status == 204 || (status >= 200 && status < 300):
		counter.successes++
	case status == 400 || status == 409 || status == 422:
		counter.businessRejects++
	case status >= 400:
		counter.clientErrors++
	default:
		counter.unexpected++
	}
}

func businessAPILatencyStatus(seen bool) string {
	if seen {
		return "ok"
	}
	return "unavailable"
}

func parseBusinessAPIAccessLogLine(line string) (businessAPILogLine, bool) {
	match := businessAPIAccessPrefixPattern.FindStringSubmatch(line)
	if len(match) == 7 {
		return buildBusinessAPILogLine(match[1], match[2], match[3], match[4], match[6])
	}
	match = businessAPICustomAccessPattern.FindStringSubmatch(line)
	if len(match) != 7 {
		return businessAPILogLine{}, false
	}
	return buildBusinessAPILogLine(match[1], match[2], match[4], match[3], match[2])
}

func buildBusinessAPILogLine(source, timestamp, request, statusValue, latencyValue string) (businessAPILogLine, bool) {
	timestampValue, latencyMS, hasLatency := splitBusinessAPIAccessTimestamp(timestamp)
	at, err := time.Parse("02/Jan/2006:15:04:05 -0700", timestampValue)
	if err != nil {
		return businessAPILogLine{}, false
	}
	requestParts := strings.Fields(request)
	if len(requestParts) < 2 {
		return businessAPILogLine{}, false
	}
	status, err := strconv.Atoi(statusValue)
	if err != nil {
		return businessAPILogLine{}, false
	}
	if !hasLatency {
		latencyMS, hasLatency = parseBusinessAPILatencyMS(latencyValue)
	}
	return businessAPILogLine{Source: source, At: at.UTC(), Method: strings.ToUpper(requestParts[0]), Path: normalizeAgentBusinessAPIPath(requestParts[1]), Status: status, LatencyMS: latencyMS, HasLatency: hasLatency}, true
}

func splitBusinessAPIAccessTimestamp(value string) (string, int64, bool) {
	value = strings.TrimSpace(value)
	separator := strings.LastIndex(value, " - ")
	if separator <= 0 {
		return value, 0, false
	}
	latencyMS, ok := parseBusinessAPILatencyMS(value[separator+3:])
	if !ok {
		return value, 0, false
	}
	return strings.TrimSpace(value[:separator]), latencyMS, true
}

func parseBusinessAPILatencyMS(rest string) (int64, bool) {
	rest = strings.TrimSpace(strings.ReplaceAll(rest, "\"", " "))
	fields := strings.Fields(rest)
	for index := len(fields) - 1; index >= 0; index-- {
		value := strings.Trim(fields[index], "[] ,")
		if !strings.Contains(value, ".") {
			continue
		}
		seconds, err := strconv.ParseFloat(value, 64)
		if err != nil || seconds < 0 || seconds > 3600 {
			continue
		}
		return int64(seconds * 1000), true
	}
	return 0, false
}

func normalizeAgentBusinessAPIEndpointWatches(items []agentBusinessAPIEndpointWatch) []agentBusinessAPIEndpointWatch {
	out := make([]agentBusinessAPIEndpointWatch, 0, min(len(items), businessAPIDiagnosticsMaxRoutes))
	seen := map[string]struct{}{}
	for _, item := range items {
		item.ID = strings.TrimSpace(item.ID)
		item.Name = strings.TrimSpace(item.Name)
		item.Method = strings.ToUpper(strings.TrimSpace(item.Method))
		if item.Method == "" {
			item.Method = "ANY"
		}
		if item.Method == "*" {
			item.Method = "ANY"
		}
		item.Path = normalizeAgentBusinessAPIPath(item.Path)
		item.LogPath = normalizeAgentBusinessAPILogPath(item.LogPath)
		if item.Path == "" || item.Method == "" {
			continue
		}
		key := businessAPIWatchKey(item.Method, item.Path)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
		if len(out) >= businessAPIDiagnosticsMaxRoutes {
			break
		}
	}
	return out
}

func businessAPIWatchKey(method, path string) string {
	return strings.ToUpper(strings.TrimSpace(method)) + "|" + normalizeAgentBusinessAPIPath(path)
}

func normalizeAgentBusinessAPIPath(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if parsed, err := url.Parse(value); err == nil {
		if parsed.Host != "" {
			value = parsed.Path
		} else if parsed.Path != "" {
			value = parsed.Path
		}
	}
	if value == "" {
		value = "/"
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + strings.TrimLeft(value, "/")
	}
	segments := strings.Split(value, "/")
	for idx, segment := range segments {
		if segment == "" {
			continue
		}
		if strings.HasPrefix(segment, ":") || businessAPIDynamicUUIDPattern.MatchString(segment) || businessAPIDynamicHexPattern.MatchString(segment) || businessAPIDynamicNumPattern.MatchString(segment) {
			segments[idx] = ":value"
		}
	}
	return strings.Join(segments, "/")
}

var defaultBusinessAPIAccessLogPaths = func() []string {
	return []string{"/var/log/nginx/access.log", "/var/log/nginx/api.access.log", "/var/log/apache2/access.log", "/var/log/httpd/access_log"}
}

var businessAPIAccessLogConfigFiles = func() []string {
	return []string{
		"/etc/nginx/nginx.conf",
		"/etc/httpd/conf/httpd.conf",
		"/etc/apache2/apache2.conf",
	}
}

var businessAPIAccessLogConfigDirs = func() []string {
	return []string{
		"/etc/nginx/conf.d",
		"/etc/nginx/sites-enabled",
		"/etc/httpd/conf.d",
		"/etc/apache2/sites-enabled",
	}
}

func businessAPIAccessLogPaths(watches []agentBusinessAPIEndpointWatch) []string {
	paths := make([]string, 0, 16)
	seen := make(map[string]struct{}, 16)
	addPath := func(raw string) {
		value := normalizeAgentBusinessAPILogPath(raw)
		if value == "" || !isSafeBusinessAPIAccessLogPath(value) {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		paths = append(paths, value)
	}
	for _, value := range defaultBusinessAPIAccessLogPaths() {
		addPath(value)
	}
	if runtimeGOOS() == "linux" {
		for _, value := range discoverBusinessAPIAccessLogPaths() {
			addPath(value)
		}
	}
	for _, watch := range watches {
		addPath(watch.LogPath)
	}
	return paths
}

func discoverBusinessAPIAccessLogPaths() []string {
	paths := make([]string, 0, 8)
	seen := make(map[string]struct{}, 8)
	addFromConfig := func(configPath string) {
		body, err := os.ReadFile(configPath)
		if err != nil || len(body) > int(businessAPIAccessLogConfigMaxBytes) {
			return
		}
		for _, value := range parseBusinessAPIAccessLogConfig(string(body)) {
			if _, exists := seen[value]; exists {
				continue
			}
			seen[value] = struct{}{}
			paths = append(paths, value)
		}
	}
	for _, configPath := range businessAPIAccessLogConfigFiles() {
		addFromConfig(configPath)
	}
	filesRead := 0
	for _, dir := range businessAPIAccessLogConfigDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if filesRead >= businessAPIAccessLogConfigMaxFiles || entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			name := strings.ToLower(entry.Name())
			if !strings.HasSuffix(name, ".conf") && !strings.Contains(name, "httpd") && !strings.Contains(name, "apache") {
				continue
			}
			filesRead++
			addFromConfig(path.Join(dir, entry.Name()))
		}
	}
	return paths
}

func parseBusinessAPIAccessLogConfig(content string) []string {
	paths := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		for index, field := range fields {
			directive := strings.Trim(field, "\";")
			if directive != "access_log" && directive != "CustomLog" && directive != "TransferLog" {
				continue
			}
			if index+1 >= len(fields) {
				continue
			}
			value := strings.Trim(fields[index+1], "\";")
			if normalized := normalizeAgentBusinessAPILogPath(value); normalized != "" && isSafeBusinessAPIAccessLogPath(normalized) {
				if _, exists := seen[normalized]; !exists {
					seen[normalized] = struct{}{}
					paths = append(paths, normalized)
				}
			}
		}
	}
	return paths
}

func normalizeAgentBusinessAPILogPath(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" || len(value) > 512 || strings.ContainsRune(value, 0) || !strings.HasPrefix(value, "/") {
		return ""
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == ".." {
			return ""
		}
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == "/" {
		return ""
	}
	return cleaned
}

func isSafeBusinessAPIAccessLogPath(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return false
	}
	base := path.Base(lower)
	return strings.Contains(lower, "/log/") || strings.Contains(lower, "/logs/") || strings.Contains(base, "access_log") ||
		(strings.Contains(base, "access") && (strings.HasSuffix(base, ".log") || strings.HasSuffix(base, ".txt"))) ||
		(strings.Contains(base, "request") && (strings.HasSuffix(base, ".log") || strings.HasSuffix(base, ".txt"))) ||
		(strings.Contains(base, "transfer") && (strings.HasSuffix(base, ".log") || strings.HasSuffix(base, ".txt")))
}

func compactBusinessAPIRouteCounters(items []*businessAPIRouteCounter) []*businessAPIRouteCounter {
	out := items[:0]
	seen := make(map[*businessAPIRouteCounter]struct{}, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func readBusinessAPILogTail(path string, maxBytes int64) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, errors.New("access log path is a directory")
	}
	if info.Size() > maxBytes {
		if _, err := file.Seek(-maxBytes, io.SeekEnd); err != nil {
			return nil, err
		}
	}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64<<10), 256<<10)
	if info.Size() > maxBytes {
		_ = scanner.Scan()
	}
	lines := make([]string, 0, 4096)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

func maskBusinessAPISource(raw string) string {
	value := strings.TrimSpace(raw)
	parts := strings.Split(value, ".")
	if len(parts) == 4 {
		return parts[0] + "." + parts[1] + "." + parts[2] + ".xxx"
	}
	if strings.Contains(value, ":") {
		chunks := strings.Split(value, ":")
		if len(chunks) > 3 {
			return strings.Join(chunks[:3], ":") + ":xxxx"
		}
	}
	if len(value) > 80 {
		return value[:80]
	}
	return value
}

func topBusinessAPISources(values map[string]businessAPISourceCounter, limit int) []agentBusinessAPIDiagnosticSource {
	out := make([]agentBusinessAPIDiagnosticSource, 0, len(values))
	for source, counter := range values {
		out = append(out, agentBusinessAPIDiagnosticSource{Source: source, Requests: counter.requests, Errors: counter.errors})
	}
	sort.Slice(out, func(left, right int) bool {
		if out[left].Requests != out[right].Requests {
			return out[left].Requests > out[right].Requests
		}
		return out[left].Source < out[right].Source
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func deterministicBusinessAPIBucketEventID(bucketStart time.Time) string {
	return "bapi_" + strconv.FormatInt(bucketStart.UTC().Unix(), 10)
}
