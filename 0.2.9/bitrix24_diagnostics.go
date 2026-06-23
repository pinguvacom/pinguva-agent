package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	bitrix24DiagnosticsDefaultWindowMinutes = 15
	bitrix24DiagnosticsMaxLogBytes          = int64(2 << 20)
	bitrix24DiagnosticsMaxEndpoints         = 12
	bitrix24DiagnosticsMaxSources           = 8
	bitrix24DiagnosticsMaxQueryGroups       = 5
)

var (
	nginxAccessLogPattern = regexp.MustCompile(`^(\S+)\s+\S+\s+\S+\s+\[([^\]]+)\]\s+"([A-Z]+)\s+([^\s"]+)(?:\s+[^\"]*)?"\s+(\d{3})\s+`)
	endpointUUIDPattern   = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	endpointHexPattern    = regexp.MustCompile(`(?i)^[0-9a-f]{16,}$`)
	endpointNumberPattern = regexp.MustCompile(`^\d+$`)
)

// bitrix24DiagnosticsLocalConfig remains on the customer host. It only lists
// sources that the root-owned, bounded diagnostics task may inspect locally.
type bitrix24DiagnosticsLocalConfig struct {
	Enabled        bool     `json:"enabled"`
	AccessLogPaths []string `json:"accessLogPaths,omitempty"`
	WindowMinutes  int      `json:"windowMinutes,omitempty"`
}

// agentBitrix24Diagnostics contains only compact, redacted aggregates. It is
// written by the local diagnostics task and read by the regular agent process.
type agentBitrix24Diagnostics struct {
	Enabled       bool                           `json:"enabled"`
	Status        string                         `json:"status,omitempty"`
	CollectedAt   time.Time                      `json:"collectedAt,omitempty"`
	WindowMinutes int                            `json:"windowMinutes,omitempty"`
	AccessLog     *agentBitrix24AccessLogSummary `json:"accessLog,omitempty"`
	MySQL         *agentBitrix24MySQLDiagnostics `json:"mysql,omitempty"`
}

type agentBitrix24AccessLogSummary struct {
	Status        string                         `json:"status,omitempty"`
	Requests      int                            `json:"requests,omitempty"`
	Errors5xx     int                            `json:"errors5xx,omitempty"`
	UniqueSources int                            `json:"uniqueSources,omitempty"`
	FilesRead     int                            `json:"filesRead,omitempty"`
	TopEndpoints  []agentBitrix24EndpointSummary `json:"topEndpoints,omitempty"`
	TopSources    []agentBitrix24SourceSummary   `json:"topSources,omitempty"`
}

type agentBitrix24EndpointSummary struct {
	Path      string `json:"path,omitempty"`
	Requests  int    `json:"requests,omitempty"`
	Errors5xx int    `json:"errors5xx,omitempty"`
	Watched   bool   `json:"watched,omitempty"`
}

type agentBitrix24SourceSummary struct {
	Source   string `json:"source,omitempty"`
	Requests int    `json:"requests,omitempty"`
}

type bitrix24EndpointCounter struct {
	requests  int
	errors5xx int
	watched   bool
}

type agentBitrix24MySQLDiagnostics struct {
	Status           string                          `json:"status,omitempty"`
	ThreadsRunning   int                             `json:"threadsRunning,omitempty"`
	ThreadsConnected int                             `json:"threadsConnected,omitempty"`
	ActiveQueries    int                             `json:"activeQueries,omitempty"`
	LongestQuerySec  int64                           `json:"longestQuerySec,omitempty"`
	TopQueries       []agentBitrix24QueryFingerprint `json:"topQueries,omitempty"`
}

type bitrix24MySQLQueryRunner func(query string) ([]string, error)

type bitrix24MySQLQueryError struct {
	Code string
}

func (err *bitrix24MySQLQueryError) Error() string {
	return "mysql " + err.Code
}

// bitrix24MySQLDiagnosticsMeta stays in the local command path and is only
// used for redacted journald output. It is intentionally not sent to Pinguva.
type bitrix24MySQLDiagnosticsMeta struct {
	Connection       string
	DefaultsWarning  string
	StatusSource     string
	FallbackUsed     bool
	StatusError      string
	ProcesslistState string
	ProcesslistError string
	QueryGroupsState string
	QueryGroupsError string
}

type bitrix24MySQLStatusSource struct {
	Name  string
	Query string
	Parse func([]string) (int, int, bool)
}

const bitrix24MySQLPerformanceStatusQuery = `
SELECT
  COALESCE(MAX(CASE WHEN VARIABLE_NAME = 'Threads_running' THEN CAST(VARIABLE_VALUE AS UNSIGNED) END), 0),
  COALESCE(MAX(CASE WHEN VARIABLE_NAME = 'Threads_connected' THEN CAST(VARIABLE_VALUE AS UNSIGNED) END), 0)
FROM performance_schema.global_status
WHERE VARIABLE_NAME IN ('Threads_running', 'Threads_connected');`

const bitrix24MySQLShowGlobalStatusQuery = `SHOW GLOBAL STATUS WHERE Variable_name IN ('Threads_running', 'Threads_connected');`

const bitrix24MySQLInformationSchemaStatusQuery = `
SELECT
  COALESCE(MAX(CASE WHEN VARIABLE_NAME = 'Threads_running' THEN CAST(VARIABLE_VALUE AS UNSIGNED) END), 0),
  COALESCE(MAX(CASE WHEN VARIABLE_NAME = 'Threads_connected' THEN CAST(VARIABLE_VALUE AS UNSIGNED) END), 0)
FROM information_schema.GLOBAL_STATUS
WHERE VARIABLE_NAME IN ('Threads_running', 'Threads_connected');`

const bitrix24MySQLProcesslistSummaryQuery = `SELECT COUNT(*), COALESCE(MAX(TIME), 0) FROM information_schema.PROCESSLIST WHERE COMMAND = 'Query' AND TIME > 0;`

const bitrix24MySQLProcesslistGroupsQuery = `
SELECT
  CASE
    WHEN INFO LIKE '%b_uts_crm_contact%' AND INFO LIKE '%UPPER(%' AND INFO LIKE '%LIKE%' THEN 'crm_contact_case_insensitive_lookup'
    WHEN INFO LIKE '%b_crm_contact%' THEN 'crm_contact_query'
    WHEN INFO LIKE '%b_crm_deal%' THEN 'crm_deal_query'
    WHEN INFO LIKE '%b_crm_lead%' THEN 'crm_lead_query'
    ELSE 'other_active_query'
  END,
  COUNT(*),
  COALESCE(MAX(TIME), 0)
FROM information_schema.PROCESSLIST
WHERE COMMAND = 'Query' AND TIME > 0 AND INFO IS NOT NULL
GROUP BY 1
ORDER BY 2 DESC, 3 DESC
LIMIT 5;`

type agentBitrix24QueryFingerprint struct {
	Kind           string `json:"kind,omitempty"`
	Count          int    `json:"count,omitempty"`
	MaxDurationSec int64  `json:"maxDurationSec,omitempty"`
}

func defaultBitrix24DiagnosticsPath() string {
	if runtime.GOOS == "linux" {
		return "/var/lib/pinguva-agent/bitrix24-diagnostics.json"
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".pinguva-agent", "bitrix24-diagnostics.json")
	}
	return "pinguva-agent-bitrix24-diagnostics.json"
}

func newBitrix24DiagnosticsLocalConfig(enabled bool, rawPaths string) *bitrix24DiagnosticsLocalConfig {
	paths := compactBitrix24AccessLogPaths(strings.Split(rawPaths, ","))
	if len(paths) == 0 {
		// Keep a bounded, known-safe list. A missing log is reported as unavailable
		// instead of making the next configuration change depend on timing.
		paths = defaultBitrix24AccessLogPaths()
	}
	return &bitrix24DiagnosticsLocalConfig{
		Enabled:        enabled,
		AccessLogPaths: paths,
		WindowMinutes:  bitrix24DiagnosticsDefaultWindowMinutes,
	}
}

func defaultBitrix24AccessLogPaths() []string {
	return []string{
		"/var/log/nginx/access.log",
		"/var/log/nginx/bitrix.access.log",
		"/var/log/apache2/access.log",
		"/var/log/httpd/access_log",
	}
}

func compactBitrix24AccessLogPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, raw := range paths {
		path := strings.TrimSpace(raw)
		if path == "" || !filepath.IsAbs(path) {
			continue
		}
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
		if len(out) >= 8 {
			break
		}
	}
	sort.Strings(out)
	return out
}

func normalizeBitrix24DiagnosticsConfig(item *bitrix24DiagnosticsLocalConfig) *bitrix24DiagnosticsLocalConfig {
	if item == nil {
		return nil
	}
	out := &bitrix24DiagnosticsLocalConfig{
		Enabled:        item.Enabled,
		AccessLogPaths: compactBitrix24AccessLogPaths(item.AccessLogPaths),
		WindowMinutes:  item.WindowMinutes,
	}
	if out.WindowMinutes <= 0 {
		out.WindowMinutes = bitrix24DiagnosticsDefaultWindowMinutes
	}
	if out.WindowMinutes > 60 {
		out.WindowMinutes = 60
	}
	return out
}

func runBitrix24Diagnostics(args []string, logger *log.Logger) error {
	fs := flag.NewFlagSet("bitrix24 diagnostics", flag.ContinueOnError)
	configPathFlag := fs.String("config-path", envOr("AGENT_BITRIX24_CONFIG_PATH", defaultBitrix24ConfigPath()), "Local Bitrix24 config path")
	outputPathFlag := fs.String("output-path", defaultBitrix24DiagnosticsPath(), "Safe local diagnostics output path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	config, err := loadBitrix24Config(*configPathFlag)
	if err != nil {
		return err
	}
	if config == nil {
		return errors.New("Bitrix24 integration is not configured")
	}
	state, err := loadAgentState(defaultStatePath())
	if err != nil {
		return err
	}
	watchPaths := []string(nil)
	if state != nil {
		watchPaths = state.Bitrix24EndpointWatches
	}
	diagnostics := collectBitrix24DiagnosticsWithLogger(*config, watchPaths, logger)
	if err := saveBitrix24Diagnostics(*outputPathFlag, diagnostics); err != nil {
		return err
	}
	if logger != nil {
		logger.Printf("Bitrix24 diagnostics: status=%s traffic=%d mysql_running=%d", diagnostics.Status, diagnosticsAccessLogRequests(diagnostics), diagnosticsMySQLThreadsRunning(diagnostics))
	}
	return nil
}

func diagnosticsAccessLogRequests(item *agentBitrix24Diagnostics) int {
	if item == nil || item.AccessLog == nil {
		return 0
	}
	return item.AccessLog.Requests
}

func diagnosticsMySQLThreadsRunning(item *agentBitrix24Diagnostics) int {
	if item == nil || item.MySQL == nil {
		return 0
	}
	return item.MySQL.ThreadsRunning
}

func collectBitrix24Diagnostics(config bitrix24LocalConfig, watchPaths []string) *agentBitrix24Diagnostics {
	return collectBitrix24DiagnosticsWithLogger(config, watchPaths, nil)
}

func collectBitrix24DiagnosticsWithLogger(config bitrix24LocalConfig, watchPaths []string, logger *log.Logger) *agentBitrix24Diagnostics {
	local := normalizeBitrix24DiagnosticsConfig(config.Diagnostics)
	if local == nil || !local.Enabled {
		return &agentBitrix24Diagnostics{Enabled: false, Status: "disabled", CollectedAt: time.Now().UTC()}
	}

	now := time.Now().UTC()
	result := &agentBitrix24Diagnostics{
		Enabled:       true,
		Status:        "ok",
		CollectedAt:   now,
		WindowMinutes: local.WindowMinutes,
		AccessLog:     collectBitrix24AccessLogSummary(local.AccessLogPaths, local.WindowMinutes, now, watchPaths),
		MySQL:         collectBitrix24MySQLDiagnosticsWithLogger(logger),
	}
	accessOK := result.AccessLog != nil && result.AccessLog.Status == "ok"
	mysqlOK := result.MySQL != nil && result.MySQL.Status == "ok"
	switch {
	case accessOK && mysqlOK:
		result.Status = "ok"
	case accessOK || mysqlOK:
		result.Status = "partial"
	default:
		result.Status = "unavailable"
	}
	return result
}

func saveBitrix24Diagnostics(path string, diagnostics *agentBitrix24Diagnostics) error {
	if diagnostics == nil {
		return errors.New("Bitrix24 diagnostics are empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	body, err := json.Marshal(diagnostics)
	if err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, body, 0o640); err != nil {
		return err
	}
	if runtime.GOOS == "linux" {
		if gid, ok := lookupUnixGroupID("pinguva-agent"); ok {
			_ = os.Chown(temporary, -1, gid)
		}
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return os.Chmod(path, 0o640)
}

func loadBitrix24Diagnostics(config *bitrix24DiagnosticsLocalConfig) *agentBitrix24Diagnostics {
	local := normalizeBitrix24DiagnosticsConfig(config)
	if local == nil || !local.Enabled {
		return nil
	}
	body, err := os.ReadFile(defaultBitrix24DiagnosticsPath())
	if err != nil {
		return nil
	}
	var diagnostics agentBitrix24Diagnostics
	if err := json.Unmarshal(body, &diagnostics); err != nil {
		return nil
	}
	return normalizeAgentBitrix24Diagnostics(&diagnostics)
}

func collectBitrix24AccessLogSummary(paths []string, windowMinutes int, now time.Time, watchPaths []string) *agentBitrix24AccessLogSummary {
	paths = compactBitrix24AccessLogPaths(paths)
	if len(paths) == 0 {
		return &agentBitrix24AccessLogSummary{Status: "unavailable"}
	}
	if windowMinutes <= 0 {
		windowMinutes = bitrix24DiagnosticsDefaultWindowMinutes
	}
	cutoff := now.Add(-time.Duration(windowMinutes) * time.Minute)
	endpoints := make(map[string]bitrix24EndpointCounter)
	for _, path := range normalizeBitrix24EndpointWatchPaths(watchPaths) {
		endpoints[path] = bitrix24EndpointCounter{watched: true}
	}
	sources := make(map[string]int)
	summary := &agentBitrix24AccessLogSummary{Status: "ok"}
	readableFile := false
	for _, path := range paths {
		lines, err := readBitrix24LogTail(path, bitrix24DiagnosticsMaxLogBytes)
		if err != nil {
			continue
		}
		readableFile = true
		summary.FilesRead++
		for _, line := range lines {
			match := nginxAccessLogPattern.FindStringSubmatch(line)
			if len(match) != 6 {
				continue
			}
			at, err := time.Parse("02/Jan/2006:15:04:05 -0700", match[2])
			if err != nil || at.Before(cutoff) || at.After(now.Add(2*time.Minute)) {
				continue
			}
			path := normalizeBitrix24EndpointPath(match[4])
			if !isBitrix24DynamicPath(path) {
				continue
			}
			statusCode, _ := strconv.Atoi(match[5])
			summary.Requests++
			if statusCode >= 500 {
				summary.Errors5xx++
			}
			source := maskBitrix24Source(match[1])
			if source != "" {
				sources[source]++
			}
			current, knownEndpoint := endpoints[path]
			if !knownEndpoint && len(endpoints) >= 2000 {
				// Keep the report bounded even when a malformed route contains IDs.
				continue
			}
			current.requests++
			if statusCode >= 500 {
				current.errors5xx++
			}
			endpoints[path] = current
		}
	}
	if !readableFile {
		summary.Status = "unavailable"
		return summary
	}
	summary.UniqueSources = len(sources)
	summary.TopEndpoints = topBitrix24Endpoints(endpoints, bitrix24DiagnosticsMaxEndpoints)
	summary.TopSources = topBitrix24Sources(sources, bitrix24DiagnosticsMaxSources)
	return summary
}

func readBitrix24LogTail(path string, maxBytes int64) ([]string, error) {
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
		// The first chunk can be the end of a previous line and is not reliable.
		_ = scanner.Scan()
	}
	lines := make([]string, 0, 4096)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func normalizeBitrix24EndpointPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "-" {
		return ""
	}
	if idx := strings.IndexByte(raw, '?'); idx >= 0 {
		raw = raw[:idx]
	}
	if idx := strings.IndexByte(raw, '#'); idx >= 0 {
		raw = raw[:idx]
	}
	if !strings.HasPrefix(raw, "/") {
		return ""
	}
	segments := strings.Split(raw, "/")
	for idx := range segments {
		segment := segments[idx]
		if endpointUUIDPattern.MatchString(segment) || endpointHexPattern.MatchString(segment) || endpointNumberPattern.MatchString(segment) {
			segments[idx] = ":id"
			continue
		}
		if bitrix24SensitivePathSegment(segment) {
			segments[idx] = ":value"
		}
	}
	value := strings.Join(segments, "/")
	if len(value) > 180 {
		return value[:180]
	}
	return value
}

func bitrix24SensitivePathSegment(segment string) bool {
	segment = strings.TrimSpace(segment)
	if segment == "" {
		return false
	}
	if len(segment) > 48 || strings.ContainsAny(segment, "@%") {
		return true
	}
	if len(segment) < 24 {
		return false
	}
	allowed := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-"
	for _, symbol := range segment {
		if !strings.ContainsRune(allowed, symbol) {
			return false
		}
	}
	return true
}

func isBitrix24DynamicPath(path string) bool {
	if path == "" {
		return false
	}
	lower := strings.ToLower(path)
	for _, suffix := range []string{".js", ".css", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".woff", ".woff2", ".map"} {
		if strings.HasSuffix(lower, suffix) {
			return false
		}
	}
	return true
}

func maskBitrix24Source(raw string) string {
	ip := net.ParseIP(strings.TrimSpace(raw))
	if ip == nil {
		return ""
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		return fmt.Sprintf("%d.%d.%d.xxx", ipv4[0], ipv4[1], ipv4[2])
	}
	parts := strings.Split(ip.String(), ":")
	if len(parts) < 4 {
		return "IPv6"
	}
	return strings.Join(parts[:4], ":") + "::"
}

func topBitrix24Endpoints(items map[string]bitrix24EndpointCounter, limit int) []agentBitrix24EndpointSummary {
	type entry struct {
		path      string
		requests  int
		errors5xx int
		watched   bool
	}
	values := make([]entry, 0, len(items))
	for path, item := range items {
		values = append(values, entry{path: path, requests: item.requests, errors5xx: item.errors5xx, watched: item.watched})
	}
	sort.Slice(values, func(left, right int) bool {
		if values[left].watched != values[right].watched {
			return values[left].watched
		}
		if values[left].requests != values[right].requests {
			return values[left].requests > values[right].requests
		}
		if values[left].errors5xx != values[right].errors5xx {
			return values[left].errors5xx > values[right].errors5xx
		}
		return values[left].path < values[right].path
	})
	if len(values) > limit {
		values = values[:limit]
	}
	out := make([]agentBitrix24EndpointSummary, 0, len(values))
	for _, item := range values {
		out = append(out, agentBitrix24EndpointSummary{Path: item.path, Requests: item.requests, Errors5xx: item.errors5xx, Watched: item.watched})
	}
	return out
}

func normalizeBitrix24EndpointWatchPaths(values []string) []string {
	indexed := make(map[string]struct{}, len(values))
	for _, value := range values {
		path := normalizeBitrix24EndpointPath(value)
		if path == "" || !isBitrix24DynamicPath(path) {
			continue
		}
		indexed[path] = struct{}{}
	}
	out := make([]string, 0, len(indexed))
	for path := range indexed {
		out = append(out, path)
	}
	sort.Strings(out)
	if len(out) > 10 {
		out = out[:10]
	}
	return out
}

func topBitrix24Sources(items map[string]int, limit int) []agentBitrix24SourceSummary {
	type entry struct {
		source   string
		requests int
	}
	values := make([]entry, 0, len(items))
	for source, requests := range items {
		values = append(values, entry{source: source, requests: requests})
	}
	sort.Slice(values, func(left, right int) bool {
		if values[left].requests != values[right].requests {
			return values[left].requests > values[right].requests
		}
		return values[left].source < values[right].source
	})
	if len(values) > limit {
		values = values[:limit]
	}
	out := make([]agentBitrix24SourceSummary, 0, len(values))
	for _, item := range values {
		out = append(out, agentBitrix24SourceSummary{Source: item.source, Requests: item.requests})
	}
	return out
}

func collectBitrix24MySQLDiagnostics() *agentBitrix24MySQLDiagnostics {
	return collectBitrix24MySQLDiagnosticsWithLogger(nil)
}

func collectBitrix24MySQLDiagnosticsWithLogger(logger *log.Logger) *agentBitrix24MySQLDiagnostics {
	binary, err := findBitrix24MySQLClient()
	if err != nil {
		result := &agentBitrix24MySQLDiagnostics{Status: "unavailable"}
		logBitrix24MySQLDiagnostics(logger, result, bitrix24MySQLDiagnosticsMeta{Connection: "unavailable", StatusError: "client_not_found", ProcesslistState: "unavailable", QueryGroupsState: "unavailable"})
		return result
	}

	defaultsOption, connection, defaultsWarning := bitrix24RootMySQLConnection()
	runner := func(query string) ([]string, error) {
		return runBitrix24MySQLQueryWithOption(binary, query, defaultsOption)
	}
	result, meta := collectBitrix24MySQLDiagnosticsWithRunner(runner)
	meta.Connection = connection
	meta.DefaultsWarning = defaultsWarning
	logBitrix24MySQLDiagnostics(logger, result, meta)
	return result
}

// collectBitrix24MySQLDiagnosticsWithRunner keeps status and PROCESSLIST
// independent: either group can be useful even when the other is unavailable.
func collectBitrix24MySQLDiagnosticsWithRunner(run bitrix24MySQLQueryRunner) (*agentBitrix24MySQLDiagnostics, bitrix24MySQLDiagnosticsMeta) {
	result := &agentBitrix24MySQLDiagnostics{Status: "unavailable"}
	meta := bitrix24MySQLDiagnosticsMeta{StatusSource: "unavailable", ProcesslistState: "unavailable", QueryGroupsState: "unavailable"}
	if run == nil {
		meta.StatusError = "runner_unavailable"
		return result, meta
	}

	statusAvailable := false
	for index, source := range []bitrix24MySQLStatusSource{
		{Name: "performance_schema.global_status", Query: bitrix24MySQLPerformanceStatusQuery, Parse: parseBitrix24MySQLStatusPair},
		{Name: "show_global_status", Query: bitrix24MySQLShowGlobalStatusQuery, Parse: parseBitrix24MySQLShowGlobalStatus},
		{Name: "information_schema.global_status", Query: bitrix24MySQLInformationSchemaStatusQuery, Parse: parseBitrix24MySQLStatusPair},
	} {
		lines, err := run(source.Query)
		if err != nil {
			meta.StatusError = bitrix24MySQLQueryErrorCode(err)
			if bitrix24MySQLStatusFallbackAllowed(meta.StatusError) {
				continue
			}
			break
		}
		running, connected, ok := source.Parse(lines)
		if !ok {
			meta.StatusError = "invalid_result"
			continue
		}
		result.ThreadsRunning = running
		result.ThreadsConnected = connected
		meta.StatusSource = source.Name
		meta.FallbackUsed = index > 0
		meta.StatusError = ""
		statusAvailable = true
		break
	}

	processlistAvailable := false
	grants, err := run("SHOW GRANTS;")
	if err != nil {
		meta.ProcesslistError = bitrix24MySQLQueryErrorCode(err)
	} else if !bitrix24MySQLHasProcessPrivilege(grants) {
		// A MySQL user without PROCESS can see only its own sessions. Reporting
		// that as a complete server-wide view would be misleading.
		meta.ProcesslistState = "restricted"
		meta.ProcesslistError = "process_privilege_missing"
	} else if activeLines, activeErr := run(bitrix24MySQLProcesslistSummaryQuery); activeErr != nil {
		meta.ProcesslistError = bitrix24MySQLQueryErrorCode(activeErr)
	} else if active, longest, ok := parseBitrix24MySQLProcesslistSummary(activeLines); !ok {
		meta.ProcesslistError = "invalid_result"
	} else {
		// 0 / 0 is a healthy valid snapshot: there were no running queries over
		// one second at the collection moment.
		result.ActiveQueries = active
		result.LongestQuerySec = longest
		processlistAvailable = true
		meta.ProcesslistState = "ok"

		queryLines, queryErr := run(bitrix24MySQLProcesslistGroupsQuery)
		if queryErr != nil {
			meta.QueryGroupsError = bitrix24MySQLQueryErrorCode(queryErr)
		} else {
			result.TopQueries = parseBitrix24QueryFingerprints(queryLines)
			meta.QueryGroupsState = "ok"
		}
	}

	switch {
	case statusAvailable && processlistAvailable && meta.QueryGroupsError == "":
		result.Status = "ok"
	case statusAvailable || processlistAvailable:
		result.Status = "partial"
	default:
		result.Status = "unavailable"
	}
	return result, meta
}

func parseBitrix24MySQLStatusPair(lines []string) (int, int, bool) {
	if len(lines) != 1 {
		return 0, 0, false
	}
	values := strings.Split(lines[0], "\t")
	if len(values) != 2 {
		return 0, 0, false
	}
	running, runningOK := safeBitrix24Int(values[0])
	connected, connectedOK := safeBitrix24Int(values[1])
	return running, connected, runningOK && connectedOK
}

func parseBitrix24MySQLShowGlobalStatus(lines []string) (int, int, bool) {
	values := make(map[string]string, 2)
	for _, line := range lines {
		parts := strings.Split(line, "\t")
		if len(parts) != 2 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(parts[0]))
		if name == "threads_running" || name == "threads_connected" {
			values[name] = parts[1]
		}
	}
	running, runningOK := safeBitrix24Int(values["threads_running"])
	connected, connectedOK := safeBitrix24Int(values["threads_connected"])
	return running, connected, runningOK && connectedOK
}

func parseBitrix24MySQLProcesslistSummary(lines []string) (int, int64, bool) {
	if len(lines) != 1 {
		return 0, 0, false
	}
	parts := strings.Split(lines[0], "\t")
	if len(parts) != 2 {
		return 0, 0, false
	}
	active, activeOK := safeBitrix24Int(parts[0])
	longest, longestOK := safeBitrix24Int64(parts[1])
	return active, longest, activeOK && longestOK
}

func bitrix24MySQLHasProcessPrivilege(grants []string) bool {
	for _, grant := range grants {
		upper := strings.ToUpper(grant)
		if strings.Contains(upper, "GRANT PROCESS ") || strings.Contains(upper, ", PROCESS ") {
			return true
		}
	}
	return false
}

func bitrix24MySQLStatusFallbackAllowed(code string) bool {
	return code == "compatibility" || code == "permission" || code == "invalid_result"
}

func bitrix24MySQLQueryErrorCode(err error) string {
	var queryErr *bitrix24MySQLQueryError
	if errors.As(err, &queryErr) && queryErr != nil && queryErr.Code != "" {
		return queryErr.Code
	}
	return "query_failed"
}

func logBitrix24MySQLDiagnostics(logger *log.Logger, result *agentBitrix24MySQLDiagnostics, meta bitrix24MySQLDiagnosticsMeta) {
	if logger == nil || result == nil {
		return
	}
	fields := []string{
		"connection=" + logBitrix24MySQLValue(meta.Connection, "unavailable"),
		"status_source=" + logBitrix24MySQLValue(meta.StatusSource, "unavailable"),
		fmt.Sprintf("fallback_used=%t", meta.FallbackUsed),
		fmt.Sprintf("threads_running=%d", result.ThreadsRunning),
		fmt.Sprintf("threads_connected=%d", result.ThreadsConnected),
		fmt.Sprintf("active_queries=%d", result.ActiveQueries),
		fmt.Sprintf("longest_query_seconds=%d", result.LongestQuerySec),
		"processlist_status=" + logBitrix24MySQLValue(meta.ProcesslistState, "unavailable"),
		"query_groups_status=" + logBitrix24MySQLValue(meta.QueryGroupsState, "unavailable"),
		"status=" + logBitrix24MySQLValue(result.Status, "unavailable"),
	}
	for _, item := range []struct{ key, value string }{
		{"defaults_warning", meta.DefaultsWarning},
		{"status_error", meta.StatusError},
		{"processlist_error", meta.ProcesslistError},
		{"query_groups_error", meta.QueryGroupsError},
	} {
		if item.value != "" {
			fields = append(fields, item.key+"="+item.value+"("+bitrix24MySQLSafeErrorText(item.value)+")")
		}
	}
	logger.Printf("Bitrix24 MySQL diagnostics: %s", strings.Join(fields, " "))
}

func logBitrix24MySQLValue(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func bitrix24MySQLSafeErrorText(code string) string {
	switch code {
	case "compatibility":
		return "source_unavailable"
	case "permission", "process_privilege_missing":
		return "permission_denied"
	case "connection":
		return "connection_failed"
	case "timeout":
		return "query_timed_out"
	case "client_not_found":
		return "client_not_found"
	case "invalid_result":
		return "invalid_metric_result"
	case "unsafe_defaults_file":
		return "using_local_socket"
	case "defaults_file_unreadable":
		return "using_local_socket"
	default:
		return "query_failed"
	}
}

func findBitrix24MySQLClient() (string, error) {
	for _, name := range []string{"mysql", "mariadb"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", errors.New("mysql client not found")
}

func runBitrix24MySQLQueryWithOption(binary, query, defaultsOption string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	args := []string{"--batch", "--skip-column-names", "--raw", "--connect-timeout=2", "-e", query}
	if defaultsOption != "" {
		// This keeps the credential in a root-only local file, never in an
		// environment variable, command history, agent report, or cloud log.
		args = append([]string{defaultsOption}, args...)
	}
	command := exec.CommandContext(ctx, binary, args...)
	command.Env = append(os.Environ(), "LANG=C")
	body, err := command.CombinedOutput()
	if ctx.Err() != nil {
		return nil, &bitrix24MySQLQueryError{Code: "timeout"}
	}
	if err != nil {
		return nil, &bitrix24MySQLQueryError{Code: classifyBitrix24MySQLCommandError(string(body))}
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) == 1 && strings.TrimSpace(lines[0]) == "" {
		return nil, nil
	}
	if len(lines) > 16 {
		lines = lines[:16]
	}
	return lines, nil
}

func bitrix24MySQLDefaultsOption(path string) string {
	info, err := os.Lstat(path)
	if err != nil || !bitrix24MySQLDefaultsFileIsSafe(info) {
		return ""
	}
	return "--defaults-extra-file=" + path
}

func bitrix24RootMySQLConnection() (option, connection, warning string) {
	const path = "/root/.my.cnf"
	info, err := os.Lstat(path)
	if err == nil {
		if bitrix24MySQLDefaultsFileIsSafe(info) {
			return "--defaults-extra-file=" + path, "defaults_extra_file", ""
		}
		return "", "socket", "unsafe_defaults_file"
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", "socket", "defaults_file_unreadable"
	}
	return "", "socket", ""
}

func bitrix24MySQLDefaultsFileIsSafe(info os.FileInfo) bool {
	return info != nil && info.Mode().IsRegular() && info.Mode().Perm() == 0o600 && bitrix24MySQLDefaultsRootOwned(info)
}

func classifyBitrix24MySQLCommandError(output string) string {
	message := strings.ToLower(output)
	switch {
	case strings.Contains(message, "access denied for user"):
		return "connection"
	case strings.Contains(message, "command denied"), strings.Contains(message, "select command denied"), strings.Contains(message, "access denied"):
		return "permission"
	case strings.Contains(message, "unknown table"), strings.Contains(message, "doesn't exist"), strings.Contains(message, "does not exist"), strings.Contains(message, "syntax error"), strings.Contains(message, "unsupported"):
		return "compatibility"
	case strings.Contains(message, "can't connect"), strings.Contains(message, "cannot connect"), strings.Contains(message, "connection refused"), strings.Contains(message, "no such file or directory"), strings.Contains(message, "lost connection"):
		return "connection"
	default:
		return "query_failed"
	}
}

func safeBitrix24Int(raw string) (int, bool) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 0 || value > 1_000_000 {
		return 0, false
	}
	return value, true
}

func safeBitrix24Int64(raw string) (int64, bool) {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || value < 0 || value > 9_999_999_999 {
		return 0, false
	}
	return value, true
}

func parseBitrix24QueryFingerprints(lines []string) []agentBitrix24QueryFingerprint {
	items := make([]agentBitrix24QueryFingerprint, 0, len(lines))
	for _, line := range lines {
		parts := strings.Split(line, "\t")
		if len(parts) != 3 {
			continue
		}
		kind := strings.TrimSpace(parts[0])
		switch kind {
		case "crm_contact_case_insensitive_lookup", "crm_contact_query", "crm_deal_query", "crm_lead_query", "other_active_query":
		default:
			continue
		}
		count, countOK := safeBitrix24Int(parts[1])
		maxDuration, durationOK := safeBitrix24Int64(parts[2])
		if !countOK || !durationOK || count == 0 {
			continue
		}
		items = append(items, agentBitrix24QueryFingerprint{Kind: kind, Count: count, MaxDurationSec: maxDuration})
		if len(items) >= bitrix24DiagnosticsMaxQueryGroups {
			break
		}
	}
	return items
}

func normalizeAgentBitrix24Diagnostics(item *agentBitrix24Diagnostics) *agentBitrix24Diagnostics {
	if item == nil || !item.Enabled {
		return nil
	}
	out := &agentBitrix24Diagnostics{
		Enabled:       true,
		Status:        normalizeBitrix24DiagnosticsStatus(item.Status),
		CollectedAt:   item.CollectedAt,
		WindowMinutes: item.WindowMinutes,
		AccessLog:     normalizeAgentBitrix24AccessLogSummary(item.AccessLog),
		MySQL:         normalizeAgentBitrix24MySQLDiagnostics(item.MySQL),
	}
	if out.WindowMinutes <= 0 || out.WindowMinutes > 60 {
		out.WindowMinutes = bitrix24DiagnosticsDefaultWindowMinutes
	}
	return out
}

func normalizeBitrix24DiagnosticsStatus(value string) string {
	switch strings.TrimSpace(value) {
	case "ok", "partial", "unavailable", "disabled":
		return strings.TrimSpace(value)
	default:
		return "unavailable"
	}
}

func normalizeAgentBitrix24AccessLogSummary(item *agentBitrix24AccessLogSummary) *agentBitrix24AccessLogSummary {
	if item == nil {
		return nil
	}
	out := &agentBitrix24AccessLogSummary{Status: normalizeBitrix24DiagnosticsStatus(item.Status)}
	if out.Status == "disabled" || out.Status == "partial" {
		out.Status = "unavailable"
	}
	out.Requests = clampBitrix24Int(item.Requests, 0, 10_000_000)
	out.Errors5xx = clampBitrix24Int(item.Errors5xx, 0, out.Requests)
	out.UniqueSources = clampBitrix24Int(item.UniqueSources, 0, 1_000_000)
	out.FilesRead = clampBitrix24Int(item.FilesRead, 0, 8)
	out.TopEndpoints = normalizeAgentBitrix24TopEndpoints(item.TopEndpoints)
	out.TopSources = normalizeAgentBitrix24TopSources(item.TopSources)
	return out
}

func normalizeAgentBitrix24TopEndpoints(items []agentBitrix24EndpointSummary) []agentBitrix24EndpointSummary {
	if len(items) == 0 {
		return nil
	}
	out := make([]agentBitrix24EndpointSummary, 0, min(len(items), bitrix24DiagnosticsMaxEndpoints))
	for _, item := range items {
		path := normalizeBitrix24EndpointPath(item.Path)
		if path == "" || !isBitrix24DynamicPath(path) {
			continue
		}
		requests := clampBitrix24Int(item.Requests, 0, 10_000_000)
		if requests == 0 {
			continue
		}
		out = append(out, agentBitrix24EndpointSummary{Path: path, Requests: requests, Errors5xx: clampBitrix24Int(item.Errors5xx, 0, requests)})
		if len(out) >= bitrix24DiagnosticsMaxEndpoints {
			break
		}
	}
	return out
}

func normalizeAgentBitrix24TopSources(items []agentBitrix24SourceSummary) []agentBitrix24SourceSummary {
	if len(items) == 0 {
		return nil
	}
	out := make([]agentBitrix24SourceSummary, 0, min(len(items), bitrix24DiagnosticsMaxSources))
	for _, item := range items {
		source := strings.TrimSpace(item.Source)
		if len(source) == 0 || len(source) > 64 || strings.Contains(source, "\n") || strings.Contains(source, "\t") {
			continue
		}
		requests := clampBitrix24Int(item.Requests, 0, 10_000_000)
		if requests == 0 {
			continue
		}
		out = append(out, agentBitrix24SourceSummary{Source: source, Requests: requests})
		if len(out) >= bitrix24DiagnosticsMaxSources {
			break
		}
	}
	return out
}

func normalizeAgentBitrix24MySQLDiagnostics(item *agentBitrix24MySQLDiagnostics) *agentBitrix24MySQLDiagnostics {
	if item == nil {
		return nil
	}
	out := &agentBitrix24MySQLDiagnostics{
		Status:           normalizeBitrix24DiagnosticsStatus(item.Status),
		ThreadsRunning:   clampBitrix24Int(item.ThreadsRunning, 0, 1_000_000),
		ThreadsConnected: clampBitrix24Int(item.ThreadsConnected, 0, 1_000_000),
		ActiveQueries:    clampBitrix24Int(item.ActiveQueries, 0, 1_000_000),
		LongestQuerySec:  clampBitrix24Int64(item.LongestQuerySec, 0, 86_400),
	}
	if out.Status == "disabled" {
		out.Status = "unavailable"
	}
	for _, item := range item.TopQueries {
		kind := strings.TrimSpace(item.Kind)
		switch kind {
		case "crm_contact_case_insensitive_lookup", "crm_contact_query", "crm_deal_query", "crm_lead_query", "other_active_query":
		default:
			continue
		}
		count := clampBitrix24Int(item.Count, 0, 1_000_000)
		if count == 0 {
			continue
		}
		out.TopQueries = append(out.TopQueries, agentBitrix24QueryFingerprint{Kind: kind, Count: count, MaxDurationSec: clampBitrix24Int64(item.MaxDurationSec, 0, 86_400)})
		if len(out.TopQueries) >= bitrix24DiagnosticsMaxQueryGroups {
			break
		}
	}
	return out
}

func clampBitrix24Int(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func clampBitrix24Int64(value, minValue, maxValue int64) int64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
