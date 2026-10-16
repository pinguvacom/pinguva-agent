package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	bitrix24LoadSchemaVersion       = 1
	bitrix24LoadBucketSeconds       = 60
	bitrix24LoadRetention           = 24 * time.Hour
	bitrix24LoadMaxStorageBytes     = int64(100 << 20)
	bitrix24LoadMaxQueueEvents      = 4
	bitrix24LoadMaxRoutes           = 20
	bitrix24LoadMaxDigestGroups     = 10
	bitrix24LoadMaxIncidentSamples  = 200
	bitrix24LoadMaxQueueLineBytes   = 256 << 10
	bitrix24LoadMaxDigestRows       = 50
	bitrix24LongQueryThresholdMS    = int64(2000)
	bitrix24ThreadsRunningThreshold = 8
	bitrix24ActiveQueriesThreshold  = 5
	bitrix24RESTSpikeThreshold      = 500
	bitrix245xxSpikeThreshold       = 20
	bitrix24IncidentSampleInterval  = 10 * time.Second
	bitrix24IncidentSampleMax       = 20 * time.Second
	bitrix24IncidentCooldown        = 2 * time.Minute
)

var (
	bitrix24SQLNumberPattern           = regexp.MustCompile(`\b(?:0x[0-9a-fA-F]+|\d+(?:\.\d+)?)\b`)
	bitrix24SQLUUIDPattern             = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
	bitrix24SQLEmailPattern            = regexp.MustCompile(`(?i)\b[\w.+-]+@[\w.-]+\.[a-z]{2,}\b`)
	errBitrix24LoadEndpointUnavailable = errors.New("historical diagnostics endpoint is unavailable")
)

// These types are intentionally mirrored in the backend contract. No raw SQL,
// access-log line, MySQL credential, processlist.INFO, or CRM value belongs in
// any of these structures.
type agentBitrix24LoadEvent struct {
	SchemaVersion int                            `json:"schemaVersion"`
	EventID       string                         `json:"eventId"`
	Kind          string                         `json:"kind"`
	Bucket        *agentBitrix24DiagnosticBucket `json:"bucket,omitempty"`
	Incident      *agentBitrix24LoadIncident     `json:"incident,omitempty"`
}

// agentBitrix24LoadEventBatch has no agent id by design. The server derives
// it from the bearer token belonging to this installed agent.
type agentBitrix24LoadEventBatch struct {
	Events []agentBitrix24LoadEvent `json:"events"`
}

type agentBitrix24LoadEventBatchResponse struct {
	EventAck []string `json:"eventAck,omitempty"`
}

type agentBitrix24DiagnosticCapabilities struct {
	MySQLGlobalStatus  bool `json:"mysqlGlobalStatus"`
	MySQLProcesslist   bool `json:"mysqlProcesslist"`
	MySQLDigestSummary bool `json:"mysqlDigestSummary"`
	MySQLLocks         bool `json:"mysqlLocks"`
	RESTAccessLog      bool `json:"restAccessLog"`
	IncidentMode       bool `json:"incidentMode"`
}

type agentBitrix24DiagnosticBucket struct {
	BucketStart   time.Time                            `json:"bucketStart"`
	BucketSeconds int                                  `json:"bucketSeconds"`
	Capabilities  agentBitrix24DiagnosticCapabilities  `json:"capabilities"`
	REST          agentBitrix24DiagnosticRESTBucket    `json:"rest"`
	MySQL         agentBitrix24DiagnosticMySQLBucket   `json:"mysql"`
	DigestGroups  []agentBitrix24DiagnosticDigestGroup `json:"digestGroups,omitempty"`
	BaselineReset bool                                 `json:"baselineReset,omitempty"`
}

type agentBitrix24DiagnosticRESTBucket struct {
	Status           string                         `json:"status"`
	RequestCount     int                            `json:"requestCount"`
	ServerErrorCount int                            `json:"serverErrorCount"`
	SourceCount      int                            `json:"sourceCount"`
	TopRoutes        []agentBitrix24DiagnosticRoute `json:"topRoutes,omitempty"`
}

type agentBitrix24DiagnosticRoute struct {
	Route            string `json:"route"`
	RequestCount     int    `json:"requestCount"`
	ServerErrorCount int    `json:"serverErrorCount"`
}

type agentBitrix24DiagnosticMySQLBucket struct {
	Status               string `json:"status"`
	ThreadsRunningMax    int    `json:"threadsRunningMax"`
	ThreadsConnectedMax  int    `json:"threadsConnectedMax"`
	ActiveQueriesMax     int    `json:"activeQueriesMax"`
	LongestQueryMS       int64  `json:"longestQueryMs"`
	LockWaitCount        int    `json:"lockWaitCount"`
	OldestLockWaitMS     int64  `json:"oldestLockWaitMs"`
	BlockingTransactions int    `json:"blockingTransactions"`
	WaitingTransactions  int    `json:"waitingTransactions"`
	LockDiagnostics      string `json:"lockDiagnostics,omitempty"`
}

type agentBitrix24DiagnosticDigestGroup struct {
	Digest        string    `json:"digest"`
	Category      string    `json:"category"`
	NormalizedSQL string    `json:"normalizedSql,omitempty"`
	Schema        string    `json:"schema,omitempty"`
	Executions    int64     `json:"executions"`
	TotalTimeMS   int64     `json:"totalTimeMs"`
	AvgTimeMS     int64     `json:"avgTimeMs"`
	MaxTimeMS     int64     `json:"maxTimeMs"`
	RowsExamined  int64     `json:"rowsExamined"`
	RowsSent      int64     `json:"rowsSent"`
	Errors        int64     `json:"errors"`
	NoIndexUsed   int64     `json:"noIndexUsed"`
	FirstSeen     time.Time `json:"firstSeen,omitempty"`
	LastSeen      time.Time `json:"lastSeen,omitempty"`
}

type agentBitrix24LoadIncident struct {
	EventID       string                               `json:"-"`
	IncidentType  string                               `json:"incidentType"`
	Severity      string                               `json:"severity"`
	Status        string                               `json:"status"`
	StartedAt     time.Time                            `json:"startedAt"`
	EndedAt       time.Time                            `json:"endedAt,omitempty"`
	DurationMS    int64                                `json:"durationMs"`
	Peaks         agentBitrix24DiagnosticMySQLBucket   `json:"peaks"`
	RESTRequests  int                                  `json:"restRequestsPerMinute"`
	ServerErrors  int                                  `json:"serverErrors"`
	RelatedRoutes []agentBitrix24DiagnosticRoute       `json:"relatedRoutes,omitempty"`
	DigestGroups  []agentBitrix24DiagnosticDigestGroup `json:"digestGroups,omitempty"`
	Samples       []agentBitrix24DiagnosticSample      `json:"samples,omitempty"`
}

type agentBitrix24DiagnosticSample struct {
	CapturedAt time.Time                          `json:"capturedAt"`
	REST       agentBitrix24DiagnosticRESTBucket  `json:"rest"`
	MySQL      agentBitrix24DiagnosticMySQLBucket `json:"mysql"`
}

type bitrix24LoadQueueRecord struct {
	AgentID string                 `json:"agentId"`
	Event   agentBitrix24LoadEvent `json:"event"`
}

type bitrix24DigestCounters struct {
	Executions   uint64    `json:"executions"`
	TotalTimePS  uint64    `json:"totalTimePs"`
	RowsExamined uint64    `json:"rowsExamined"`
	RowsSent     uint64    `json:"rowsSent"`
	Errors       uint64    `json:"errors"`
	NoIndexUsed  uint64    `json:"noIndexUsed"`
	LastSeen     time.Time `json:"lastSeen,omitempty"`
}

type bitrix24DigestSnapshot struct {
	Digest        string
	Category      string
	NormalizedSQL string
	Schema        string
	Counters      bitrix24DigestCounters
	AvgTimeMS     int64
	MaxTimeMS     int64
	FirstSeen     time.Time
	LastSeen      time.Time
}

type bitrix24LoadActiveIncident struct {
	EventID       string                               `json:"eventId"`
	IncidentType  string                               `json:"incidentType"`
	Severity      string                               `json:"severity"`
	StartedAt     time.Time                            `json:"startedAt"`
	Peaks         agentBitrix24DiagnosticMySQLBucket   `json:"peaks"`
	RESTRequests  int                                  `json:"restRequests"`
	ServerErrors  int                                  `json:"serverErrors"`
	RelatedRoutes []agentBitrix24DiagnosticRoute       `json:"relatedRoutes"`
	DigestGroups  []agentBitrix24DiagnosticDigestGroup `json:"digestGroups"`
	Samples       []agentBitrix24DiagnosticSample      `json:"samples"`
	RecoveryTicks int                                  `json:"recoveryTicks"`
}

type bitrix24LoadLocalState struct {
	LastBucketStart        time.Time                         `json:"lastBucketStart,omitempty"`
	LastIncidentResolvedAt time.Time                         `json:"lastIncidentResolvedAt,omitempty"`
	Digests                map[string]bitrix24DigestCounters `json:"digests,omitempty"`
	ActiveIncident         *bitrix24LoadActiveIncident       `json:"activeIncident,omitempty"`
}

var bitrix24LoadDiagnosticsRoot = bitrix24LoadDiagnosticsDefaultDir

func bitrix24LoadDiagnosticsDefaultDir() string {
	if runtime.GOOS == "linux" {
		return "/var/lib/pinguva-agent/diagnostics/bitrix24"
	}
	return filepath.Join(filepath.Dir(defaultBitrix24DiagnosticsPath()), "diagnostics", "bitrix24")
}

func defaultBitrix24LoadDiagnosticsDir() string {
	return bitrix24LoadDiagnosticsRoot()
}

func bitrix24LoadStatePath() string {
	return filepath.Join(defaultBitrix24LoadDiagnosticsDir(), "state.json")
}

func bitrix24LoadQueuePath(at time.Time) string {
	return filepath.Join(defaultBitrix24LoadDiagnosticsDir(), "minute-"+at.UTC().Format("2006-01-02-15")+".jsonl")
}

func acquireBitrix24LoadLock() (func(), bool, error) {
	dir := defaultBitrix24LoadDiagnosticsDir()
	if err := ensureBitrix24LoadDiagnosticsDir(dir); err != nil {
		return nil, false, err
	}
	path := filepath.Join(dir, ".collector.lock")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil && errors.Is(err, os.ErrExist) {
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.Mode().IsRegular() {
			return nil, false, errors.New("Bitrix24 diagnostics lock is unsafe")
		}
		if time.Since(info.ModTime()) > 5*time.Minute {
			if removeErr := os.Remove(path); removeErr == nil {
				return acquireBitrix24LoadLock()
			}
		}
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	_, _ = io.WriteString(file, strconv.FormatInt(time.Now().UTC().Unix(), 10))
	_ = file.Close()
	return func() { _ = os.Remove(path) }, true, nil
}

func ensureBitrix24LoadDiagnosticsDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("Bitrix24 diagnostics directory is unsafe")
	}
	if runtime.GOOS == "linux" && !bitrix24MySQLDefaultsRootOwned(info) {
		return errors.New("Bitrix24 diagnostics directory is not owned by root")
	}
	return os.Chmod(dir, 0o700)
}

func collectAndQueueBitrix24LoadDiagnostics(agentID string, config bitrix24LocalConfig, watchPaths []string, snapshot *agentBitrix24Diagnostics, logger *log.Logger) (bool, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" || snapshot == nil || !snapshot.Enabled {
		return false, nil
	}
	release, acquired, err := acquireBitrix24LoadLock()
	if err != nil {
		return false, err
	}
	if !acquired {
		if logger != nil {
			logger.Printf("Bitrix24 load diagnostics skipped: previous_cycle_running")
		}
		return false, nil
	}
	defer release()

	state, err := loadBitrix24LoadLocalState()
	if err != nil {
		return false, err
	}
	now := time.Now().UTC()
	bucketStart := now.Truncate(time.Minute).Add(-time.Minute)
	if state.LastBucketStart.Equal(bucketStart) {
		return false, nil
	}
	access := collectBitrix24AccessLogSummaryRange(config.Diagnostics.AccessLogPaths, bucketStart, bucketStart.Add(time.Minute), watchPaths)
	bucket := buildBitrix24LoadBucket(bucketStart, access, snapshot.MySQL)
	digestGroups, baselineReset, digestAvailable := collectBitrix24DigestDeltas(state)
	bucket.DigestGroups = digestGroups
	bucket.BaselineReset = baselineReset
	bucket.Capabilities.MySQLDigestSummary = digestAvailable
	incidentMode, _, _ := bitrix24LoadIncidentSignal(bucket)

	events := []agentBitrix24LoadEvent{{
		SchemaVersion: bitrix24LoadSchemaVersion,
		EventID:       deterministicBitrix24BucketEventID(agentID, bucketStart),
		Kind:          "minute_bucket",
		Bucket:        &bucket,
	}}
	if incident := updateBitrix24LoadIncident(state, bucket, now); incident != nil {
		events = append(events, agentBitrix24LoadEvent{SchemaVersion: bitrix24LoadSchemaVersion, EventID: incident.EventID, Kind: "load_incident", Incident: incident})
	}
	for _, event := range events {
		if err := appendBitrix24LoadQueueRecord(bitrix24LoadQueueRecord{AgentID: agentID, Event: event}, bucketStart); err != nil {
			return false, err
		}
	}
	state.LastBucketStart = bucketStart
	if err := saveBitrix24LoadLocalState(state); err != nil {
		return false, err
	}
	if err := trimBitrix24LoadStorage(now); err != nil && logger != nil {
		logger.Printf("Bitrix24 load diagnostics storage cleanup failed: %v", err)
	}
	return incidentMode && state.ActiveIncident != nil, nil
}

// collectBitrix24LoadIncidentSamples adds a small number of MySQL-only samples
// while the current root-owned timer process is still active. It deliberately
// does not start a daemon, touch Bitrix24 REST or rescan access logs: the normal
// minute collector owns those sources and this mode remains bounded read-only.
func collectBitrix24LoadIncidentSamples(agentID string, logger *log.Logger) {
	deadline := time.Now().UTC().Add(bitrix24IncidentSampleMax)
	for next := time.Now().UTC().Add(bitrix24IncidentSampleInterval); !next.After(deadline); next = next.Add(bitrix24IncidentSampleInterval) {
		time.Sleep(time.Until(next))
		if err := appendBitrix24LoadIncidentSample(agentID, time.Now().UTC(), logger); err != nil {
			if logger != nil {
				logger.Printf("Bitrix24 incident sample skipped: %v", err)
			}
			return
		}
	}
}

func appendBitrix24LoadIncidentSample(agentID string, capturedAt time.Time, logger *log.Logger) error {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil
	}
	release, acquired, err := acquireBitrix24LoadLock()
	if err != nil {
		return err
	}
	if !acquired {
		return nil
	}
	defer release()
	state, err := loadBitrix24LoadLocalState()
	if err != nil {
		return err
	}
	if state.ActiveIncident == nil {
		return nil
	}
	mysql := collectBitrix24MySQLDiagnosticsWithLogger(logger)
	bucket := agentBitrix24DiagnosticBucket{
		REST:  agentBitrix24DiagnosticRESTBucket{Status: "unavailable"},
		MySQL: buildBitrix24LoadBucket(capturedAt, nil, mysql).MySQL,
	}
	mergeBitrix24LoadIncidentPeaks(state.ActiveIncident, bucket)
	if len(state.ActiveIncident.Samples) < bitrix24LoadMaxIncidentSamples {
		state.ActiveIncident.Samples = append(state.ActiveIncident.Samples, agentBitrix24DiagnosticSample{CapturedAt: capturedAt.UTC(), REST: bucket.REST, MySQL: bucket.MySQL})
	}
	incident := state.ActiveIncident.toEvent("active", time.Time{})
	if err := appendBitrix24LoadQueueRecord(bitrix24LoadQueueRecord{
		AgentID: agentID,
		Event:   agentBitrix24LoadEvent{SchemaVersion: bitrix24LoadSchemaVersion, EventID: incident.EventID, Kind: "load_incident", Incident: incident},
	}, capturedAt); err != nil {
		return err
	}
	return saveBitrix24LoadLocalState(state)
}

func buildBitrix24LoadBucket(start time.Time, access *agentBitrix24AccessLogSummary, mysql *agentBitrix24MySQLDiagnostics) agentBitrix24DiagnosticBucket {
	rest := agentBitrix24DiagnosticRESTBucket{Status: "unavailable", TopRoutes: []agentBitrix24DiagnosticRoute{}}
	if access != nil {
		rest.Status = normalizeBitrix24LoadStatus(access.Status)
		rest.RequestCount = clampBitrix24LoadInt(access.Requests, 0, 10_000_000)
		rest.ServerErrorCount = clampBitrix24LoadInt(access.Errors5xx, 0, rest.RequestCount)
		rest.SourceCount = clampBitrix24LoadInt(access.UniqueSources, 0, 1_000_000)
		for _, item := range access.TopEndpoints {
			path := normalizeBitrix24EndpointPath(item.Path)
			if path == "" || !isBitrix24DynamicPath(path) || item.Requests <= 0 {
				continue
			}
			rest.TopRoutes = append(rest.TopRoutes, agentBitrix24DiagnosticRoute{Route: path, RequestCount: clampBitrix24LoadInt(item.Requests, 0, 10_000_000), ServerErrorCount: clampBitrix24LoadInt(item.Errors5xx, 0, item.Requests)})
			if len(rest.TopRoutes) >= bitrix24LoadMaxRoutes {
				break
			}
		}
	}
	mysqlBucket := agentBitrix24DiagnosticMySQLBucket{Status: "unavailable", LockDiagnostics: "unsupported"}
	if mysql != nil {
		mysqlBucket.Status = normalizeBitrix24LoadStatus(mysql.Status)
		mysqlBucket.ThreadsRunningMax = clampBitrix24LoadInt(mysql.ThreadsRunning, 0, 1_000_000)
		mysqlBucket.ThreadsConnectedMax = clampBitrix24LoadInt(mysql.ThreadsConnected, 0, 1_000_000)
		mysqlBucket.ActiveQueriesMax = clampBitrix24LoadInt(mysql.ActiveQueries, 0, 1_000_000)
		mysqlBucket.LongestQueryMS = clampBitrix24LoadInt64(mysql.LongestQuerySec*1000, 0, 86_400_000)
		mysqlBucket.LockWaitCount = clampBitrix24LoadInt(mysql.LockWaitCount, 0, 1_000_000)
		mysqlBucket.OldestLockWaitMS = clampBitrix24LoadInt64(mysql.OldestLockWaitMS, 0, 86_400_000)
		mysqlBucket.BlockingTransactions = clampBitrix24LoadInt(mysql.BlockingTransactions, 0, 1_000_000)
		mysqlBucket.WaitingTransactions = clampBitrix24LoadInt(mysql.WaitingTransactions, 0, 1_000_000)
		mysqlBucket.LockDiagnostics = normalizeBitrix24MySQLLockDiagnostics(mysql.LockDiagnostics)
	}
	return agentBitrix24DiagnosticBucket{
		BucketStart:   start.UTC(),
		BucketSeconds: bitrix24LoadBucketSeconds,
		Capabilities: agentBitrix24DiagnosticCapabilities{
			MySQLGlobalStatus: mysql != nil && (mysql.Status == "ok" || mysql.Status == "partial"),
			MySQLProcesslist:  mysql != nil && mysql.ProcesslistStatus == "ok",
			MySQLLocks:        mysql != nil && mysql.LockDiagnostics == "ok",
			RESTAccessLog:     access != nil && access.Status == "ok",
			IncidentMode:      true,
		},
		REST:  rest,
		MySQL: mysqlBucket,
	}
}

func normalizeBitrix24LoadStatus(value string) string {
	switch strings.TrimSpace(value) {
	case "ok", "partial", "unavailable":
		return strings.TrimSpace(value)
	default:
		return "unavailable"
	}
}

func clampBitrix24LoadInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func clampBitrix24LoadInt64(value, minValue, maxValue int64) int64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func deterministicBitrix24BucketEventID(agentID string, bucketStart time.Time) string {
	sum := sha256.Sum256([]byte(agentID + "|" + bucketStart.UTC().Format(time.RFC3339)))
	return "b24m_" + hex.EncodeToString(sum[:20])
}

func newBitrix24IncidentEventID() string {
	var bytes [20]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("b24i_%x", time.Now().UTC().UnixNano())
	}
	return "b24i_" + hex.EncodeToString(bytes[:])
}

func updateBitrix24LoadIncident(state *bitrix24LoadLocalState, bucket agentBitrix24DiagnosticBucket, now time.Time) *agentBitrix24LoadIncident {
	if state == nil {
		return nil
	}
	triggered, incidentType, severity := bitrix24LoadIncidentSignal(bucket)
	sample := agentBitrix24DiagnosticSample{CapturedAt: now.UTC(), REST: bucket.REST, MySQL: bucket.MySQL}
	if state.ActiveIncident == nil && !triggered {
		return nil
	}
	if state.ActiveIncident == nil {
		if !state.LastIncidentResolvedAt.IsZero() && now.Sub(state.LastIncidentResolvedAt) < bitrix24IncidentCooldown {
			return nil
		}
		state.ActiveIncident = &bitrix24LoadActiveIncident{
			EventID:       newBitrix24IncidentEventID(),
			IncidentType:  incidentType,
			Severity:      severity,
			StartedAt:     bucket.BucketStart,
			Peaks:         bucket.MySQL,
			RESTRequests:  bucket.REST.RequestCount,
			ServerErrors:  bucket.REST.ServerErrorCount,
			RelatedRoutes: append([]agentBitrix24DiagnosticRoute(nil), bucket.REST.TopRoutes...),
			DigestGroups:  append([]agentBitrix24DiagnosticDigestGroup(nil), bucket.DigestGroups...),
			Samples:       []agentBitrix24DiagnosticSample{sample},
		}
	} else {
		active := state.ActiveIncident
		if triggered {
			active.RecoveryTicks = 0
			if severity == "critical" {
				active.Severity = "critical"
			}
			if active.IncidentType != "combined_rest_mysql_spike" && incidentType == "combined_rest_mysql_spike" {
				active.IncidentType = incidentType
			}
			mergeBitrix24LoadIncidentPeaks(active, bucket)
		} else {
			active.RecoveryTicks++
		}
		if len(active.Samples) < 200 {
			active.Samples = append(active.Samples, sample)
		}
	}
	active := state.ActiveIncident
	if active == nil {
		return nil
	}
	if !triggered && active.RecoveryTicks >= 1 {
		incident := active.toEvent("resolved", bucket.BucketStart.Add(time.Minute))
		state.ActiveIncident = nil
		state.LastIncidentResolvedAt = incident.EndedAt
		return incident
	}
	return active.toEvent("active", time.Time{})
}

func bitrix24LoadIncidentSignal(bucket agentBitrix24DiagnosticBucket) (bool, string, string) {
	mysqlTriggered := bucket.MySQL.LongestQueryMS >= bitrix24LongQueryThresholdMS || bucket.MySQL.ThreadsRunningMax >= bitrix24ThreadsRunningThreshold || bucket.MySQL.ActiveQueriesMax >= bitrix24ActiveQueriesThreshold || bucket.MySQL.LockWaitCount > 0
	restTriggered := bucket.REST.RequestCount >= bitrix24RESTSpikeThreshold || bucket.REST.ServerErrorCount >= bitrix245xxSpikeThreshold
	severity := "warning"
	if bucket.MySQL.LongestQueryMS >= 10_000 || bucket.MySQL.ThreadsRunningMax >= 20 || bucket.REST.ServerErrorCount >= 30 || bucket.REST.RequestCount >= 1_000 || bucket.MySQL.LockWaitCount >= 3 {
		severity = "critical"
	}
	switch {
	case mysqlTriggered && restTriggered:
		return true, "combined_rest_mysql_spike", severity
	case bucket.MySQL.LockWaitCount > 0:
		return true, "database_lock_wait", severity
	case bucket.MySQL.LongestQueryMS >= bitrix24LongQueryThresholdMS:
		return true, "long_sql_query", severity
	case mysqlTriggered:
		return true, "mysql_activity_spike", severity
	case bucket.REST.ServerErrorCount >= bitrix245xxSpikeThreshold:
		return true, "server_error_spike", severity
	case restTriggered:
		return true, "rest_traffic_spike", severity
	default:
		return false, "", ""
	}
}

func mergeBitrix24LoadIncidentPeaks(active *bitrix24LoadActiveIncident, bucket agentBitrix24DiagnosticBucket) {
	if active == nil {
		return
	}
	active.Peaks.ThreadsRunningMax = max(active.Peaks.ThreadsRunningMax, bucket.MySQL.ThreadsRunningMax)
	active.Peaks.ThreadsConnectedMax = max(active.Peaks.ThreadsConnectedMax, bucket.MySQL.ThreadsConnectedMax)
	active.Peaks.ActiveQueriesMax = max(active.Peaks.ActiveQueriesMax, bucket.MySQL.ActiveQueriesMax)
	active.Peaks.LongestQueryMS = max(active.Peaks.LongestQueryMS, bucket.MySQL.LongestQueryMS)
	active.Peaks.LockWaitCount = max(active.Peaks.LockWaitCount, bucket.MySQL.LockWaitCount)
	active.Peaks.OldestLockWaitMS = max(active.Peaks.OldestLockWaitMS, bucket.MySQL.OldestLockWaitMS)
	active.Peaks.BlockingTransactions = max(active.Peaks.BlockingTransactions, bucket.MySQL.BlockingTransactions)
	active.Peaks.WaitingTransactions = max(active.Peaks.WaitingTransactions, bucket.MySQL.WaitingTransactions)
	active.RESTRequests = max(active.RESTRequests, bucket.REST.RequestCount)
	active.ServerErrors = max(active.ServerErrors, bucket.REST.ServerErrorCount)
	if len(bucket.REST.TopRoutes) > 0 {
		active.RelatedRoutes = append([]agentBitrix24DiagnosticRoute(nil), bucket.REST.TopRoutes...)
	}
	if len(bucket.DigestGroups) > 0 {
		active.DigestGroups = append([]agentBitrix24DiagnosticDigestGroup(nil), bucket.DigestGroups...)
	}
}

func (active *bitrix24LoadActiveIncident) toEvent(status string, endedAt time.Time) *agentBitrix24LoadIncident {
	if active == nil {
		return nil
	}
	ending := endedAt.UTC()
	duration := int64(0)
	if !ending.IsZero() && ending.After(active.StartedAt) {
		duration = ending.Sub(active.StartedAt).Milliseconds()
	}
	return &agentBitrix24LoadIncident{
		EventID:       active.EventID,
		IncidentType:  active.IncidentType,
		Severity:      active.Severity,
		Status:        status,
		StartedAt:     active.StartedAt.UTC(),
		EndedAt:       ending,
		DurationMS:    duration,
		Peaks:         active.Peaks,
		RESTRequests:  active.RESTRequests,
		ServerErrors:  active.ServerErrors,
		RelatedRoutes: append([]agentBitrix24DiagnosticRoute(nil), active.RelatedRoutes...),
		DigestGroups:  append([]agentBitrix24DiagnosticDigestGroup(nil), active.DigestGroups...),
		Samples:       append([]agentBitrix24DiagnosticSample(nil), active.Samples...),
	}
}

func loadBitrix24LoadLocalState() (*bitrix24LoadLocalState, error) {
	state := &bitrix24LoadLocalState{Digests: map[string]bitrix24DigestCounters{}}
	body, err := readBitrix24LoadFile(bitrix24LoadStatePath(), bitrix24LoadMaxQueueLineBytes)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(body, state); err != nil {
		return nil, err
	}
	if state.Digests == nil {
		state.Digests = map[string]bitrix24DigestCounters{}
	}
	return state, nil
}

func saveBitrix24LoadLocalState(state *bitrix24LoadLocalState) error {
	if state == nil {
		return errors.New("Bitrix24 diagnostics state is empty")
	}
	if err := ensureBitrix24LoadDiagnosticsDir(defaultBitrix24LoadDiagnosticsDir()); err != nil {
		return err
	}
	body, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return writeBitrix24LoadFile(bitrix24LoadStatePath(), body)
}

func appendBitrix24LoadQueueRecord(record bitrix24LoadQueueRecord, bucketStart time.Time) error {
	if err := ensureBitrix24LoadDiagnosticsDir(defaultBitrix24LoadDiagnosticsDir()); err != nil {
		return err
	}
	path := bitrix24LoadQueuePath(bucketStart)
	records, err := readBitrix24LoadQueueFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, existing := range records {
		if existing.AgentID == record.AgentID && existing.Event.EventID == record.Event.EventID {
			if existing.Event.Kind == "load_incident" && record.Event.Kind == "load_incident" {
				for index := range records {
					if records[index].AgentID == record.AgentID && records[index].Event.EventID == record.Event.EventID {
						records[index] = record
						break
					}
				}
				return writeBitrix24LoadQueueRecords(path, records)
			}
			return nil
		}
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	body, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if len(body) > bitrix24LoadMaxQueueLineBytes {
		return errors.New("Bitrix24 diagnostics queue record exceeds limit")
	}
	if _, err := file.Write(append(body, '\n')); err != nil {
		return err
	}
	return file.Chmod(0o600)
}

func writeBitrix24LoadQueueRecords(path string, records []bitrix24LoadQueueRecord) error {
	if len(records) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	var body strings.Builder
	for _, record := range records {
		line, err := json.Marshal(record)
		if err != nil {
			return err
		}
		body.Write(line)
		body.WriteByte('\n')
	}
	return writeBitrix24LoadFile(path, []byte(body.String()))
}

func pendingBitrix24LoadEvents(agentID string, limit int) ([]agentBitrix24LoadEvent, error) {
	if limit <= 0 || limit > bitrix24LoadMaxQueueEvents {
		limit = bitrix24LoadMaxQueueEvents
	}
	files, err := listBitrix24LoadQueueFiles()
	if err != nil {
		return nil, err
	}
	out := make([]agentBitrix24LoadEvent, 0, limit)
	for _, path := range files {
		records, err := readBitrix24LoadQueueFile(path)
		if err != nil {
			continue
		}
		for _, record := range records {
			if record.AgentID != agentID || len(out) >= limit {
				continue
			}
			out = append(out, record.Event)
		}
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func acknowledgeBitrix24LoadEvents(agentID string, eventIDs []string) error {
	if len(eventIDs) == 0 {
		return nil
	}
	acked := make(map[string]struct{}, len(eventIDs))
	for _, eventID := range eventIDs {
		acked[eventID] = struct{}{}
	}
	files, err := listBitrix24LoadQueueFiles()
	if err != nil {
		return err
	}
	for _, path := range files {
		records, err := readBitrix24LoadQueueFile(path)
		if err != nil {
			continue
		}
		remaining := records[:0]
		changed := false
		for _, record := range records {
			if record.AgentID == agentID {
				if _, ok := acked[record.Event.EventID]; ok {
					changed = true
					continue
				}
			}
			remaining = append(remaining, record)
		}
		if !changed {
			continue
		}
		if err := writeBitrix24LoadQueueRecords(path, remaining); err != nil {
			return err
		}
	}
	return nil
}

func listBitrix24LoadQueueFiles() ([]string, error) {
	dir := defaultBitrix24LoadDiagnosticsDir()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "minute-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		path := filepath.Join(dir, name)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

func readBitrix24LoadQueueFile(path string) ([]bitrix24LoadQueueRecord, error) {
	body, err := readBitrix24LoadFile(path, bitrix24LoadMaxStorageBytes)
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	scanner.Buffer(make([]byte, 0, 64<<10), bitrix24LoadMaxQueueLineBytes)
	records := make([]bitrix24LoadQueueRecord, 0, 8)
	for scanner.Scan() {
		var record bitrix24LoadQueueRecord
		if json.Unmarshal(scanner.Bytes(), &record) != nil || strings.TrimSpace(record.AgentID) == "" || strings.TrimSpace(record.Event.EventID) == "" {
			continue
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func readBitrix24LoadFile(path string, maxBytes int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o600 || info.Size() > maxBytes {
		return nil, errors.New("Bitrix24 diagnostics file is unsafe")
	}
	if runtime.GOOS == "linux" && !bitrix24MySQLDefaultsRootOwned(info) {
		return nil, errors.New("Bitrix24 diagnostics file is not owned by root")
	}
	return os.ReadFile(path)
}

func writeBitrix24LoadFile(path string, body []byte) error {
	if int64(len(body)) > bitrix24LoadMaxStorageBytes {
		return errors.New("Bitrix24 diagnostics file exceeds storage limit")
	}
	if err := ensureBitrix24LoadDiagnosticsDir(filepath.Dir(path)); err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, body, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return os.Chmod(path, 0o600)
}

func flushBitrix24LoadEvents(serverURL string, state *agentState, logger *log.Logger) {
	if state == nil || strings.TrimSpace(state.AgentID) == "" || strings.TrimSpace(state.Token) == "" {
		return
	}
	endpoint, err := bitrix24LoadDiagnosticsEndpoint(serverURL)
	if err != nil {
		if logger != nil {
			logger.Printf("Bitrix24 historical diagnostics delivery skipped: %v", err)
		}
		return
	}
	events, err := pendingBitrix24LoadEvents(state.AgentID, bitrix24LoadMaxQueueEvents)
	if err != nil || len(events) == 0 {
		if err != nil && logger != nil {
			logger.Printf("Bitrix24 historical diagnostics queue read failed: %v", err)
		}
		return
	}
	ack, err := postBitrix24LoadEvents(&http.Client{Timeout: 10 * time.Second}, endpoint, state.Token, events)
	if err != nil {
		// A newer agent may temporarily run against an older Pinguva backend.
		// Keep the bounded queue without adding a harmless compatibility warning
		// to the customer's journal every minute.
		if errors.Is(err, errBitrix24LoadEndpointUnavailable) {
			return
		}
		if logger != nil {
			logger.Printf("Bitrix24 historical diagnostics delivery pending: %v", err)
		}
		return
	}
	if err := acknowledgeBitrix24LoadEvents(state.AgentID, ack); err != nil && logger != nil {
		logger.Printf("Bitrix24 historical diagnostics queue ack failed: %v", err)
	}
}

func bitrix24LoadDiagnosticsEndpoint(serverURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(serverURL))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return "", errors.New("AGENT_SERVER is not configured for historical diagnostics")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/api/agent/v1/bitrix24-diagnostics"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func postBitrix24LoadEvents(client *http.Client, endpoint, token string, events []agentBitrix24LoadEvent) ([]string, error) {
	body, err := json.Marshal(agentBitrix24LoadEventBatch{Events: events})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotFound {
		return nil, errBitrix24LoadEndpointUnavailable
	}
	if resp.StatusCode >= http.StatusMultipleChoices {
		return nil, &httpError{Status: resp.Status}
	}
	var payload agentBitrix24LoadEventBatchResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, bitrix24LoadMaxQueueLineBytes)).Decode(&payload); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return payload.EventAck, nil
}

func trimBitrix24LoadStorage(now time.Time) error {
	files, err := listBitrix24LoadQueueFiles()
	if err != nil {
		return err
	}
	type fileEntry struct {
		path string
		info os.FileInfo
	}
	entries := make([]fileEntry, 0, len(files))
	var total int64
	for _, path := range files {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if now.Sub(info.ModTime()) > bitrix24LoadRetention {
			_ = os.Remove(path)
			continue
		}
		entries = append(entries, fileEntry{path: path, info: info})
		total += info.Size()
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].info.ModTime().Before(entries[right].info.ModTime()) })
	current := bitrix24LoadQueuePath(now)
	for _, entry := range entries {
		if total <= bitrix24LoadMaxStorageBytes {
			break
		}
		if entry.path == current {
			continue
		}
		if err := os.Remove(entry.path); err == nil {
			total -= entry.info.Size()
		}
	}
	return nil
}

const bitrix24DigestColumnsQuery = `
SELECT COLUMN_NAME
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = 'performance_schema'
  AND TABLE_NAME = 'events_statements_summary_by_digest'
  AND COLUMN_NAME IN (
    'SCHEMA_NAME', 'DIGEST', 'DIGEST_TEXT', 'COUNT_STAR', 'SUM_TIMER_WAIT',
    'AVG_TIMER_WAIT', 'MAX_TIMER_WAIT', 'SUM_ROWS_EXAMINED', 'SUM_ROWS_SENT',
    'SUM_ERRORS', 'SUM_WARNINGS', 'SUM_NO_INDEX_USED', 'SUM_NO_GOOD_INDEX_USED',
    'FIRST_SEEN', 'LAST_SEEN'
  );`

func collectBitrix24DigestDeltas(state *bitrix24LoadLocalState) ([]agentBitrix24DiagnosticDigestGroup, bool, bool) {
	if state == nil {
		return nil, false, false
	}
	binary, err := findBitrix24MySQLClient()
	if err != nil {
		return nil, false, false
	}
	defaults, _, _ := bitrix24RootMySQLConnection()
	runner := func(query string) ([]string, error) { return runBitrix24MySQLQueryWithOption(binary, query, defaults) }
	snapshot, ok := collectBitrix24DigestSnapshot(runner)
	if !ok {
		return nil, false, false
	}
	if state.Digests == nil {
		state.Digests = map[string]bitrix24DigestCounters{}
	}
	baselineReset := false
	out := make([]agentBitrix24DiagnosticDigestGroup, 0, bitrix24LoadMaxDigestGroups)
	for _, current := range snapshot {
		previous, exists := state.Digests[current.Digest]
		state.Digests[current.Digest] = current.Counters
		if !exists || bitrix24DigestCounterReset(previous, current.Counters) {
			baselineReset = true
			continue
		}
		delta := bitrix24DigestCounterDelta(previous, current.Counters)
		if delta.Executions == 0 {
			continue
		}
		out = append(out, agentBitrix24DiagnosticDigestGroup{
			Digest:        current.Digest,
			Category:      current.Category,
			NormalizedSQL: current.NormalizedSQL,
			Schema:        current.Schema,
			Executions:    int64(delta.Executions),
			TotalTimeMS:   bitrix24PicoToMilliseconds(delta.TotalTimePS),
			AvgTimeMS:     current.AvgTimeMS,
			MaxTimeMS:     current.MaxTimeMS,
			RowsExamined:  saturatingBitrix24Uint64ToInt64(delta.RowsExamined),
			RowsSent:      saturatingBitrix24Uint64ToInt64(delta.RowsSent),
			Errors:        saturatingBitrix24Uint64ToInt64(delta.Errors),
			NoIndexUsed:   saturatingBitrix24Uint64ToInt64(delta.NoIndexUsed),
			FirstSeen:     current.FirstSeen,
			LastSeen:      current.LastSeen,
		})
	}
	trimBitrix24DigestState(state.Digests)
	sort.Slice(out, func(left, right int) bool {
		if out[left].TotalTimeMS != out[right].TotalTimeMS {
			return out[left].TotalTimeMS > out[right].TotalTimeMS
		}
		return out[left].Digest < out[right].Digest
	})
	if len(out) > bitrix24LoadMaxDigestGroups {
		out = out[:bitrix24LoadMaxDigestGroups]
	}
	return out, baselineReset, true
}

func collectBitrix24DigestSnapshot(run bitrix24MySQLQueryRunner) ([]bitrix24DigestSnapshot, bool) {
	columns, err := run(bitrix24DigestColumnsQuery)
	if err != nil {
		return nil, false
	}
	available := make(map[string]bool, len(columns))
	for _, column := range columns {
		available[strings.TrimSpace(column)] = true
	}
	for _, required := range []string{"DIGEST", "COUNT_STAR", "SUM_TIMER_WAIT"} {
		if !available[required] {
			return nil, false
		}
	}
	order := []string{"SCHEMA_NAME", "DIGEST", "DIGEST_TEXT", "COUNT_STAR", "SUM_TIMER_WAIT", "AVG_TIMER_WAIT", "MAX_TIMER_WAIT", "SUM_ROWS_EXAMINED", "SUM_ROWS_SENT", "SUM_ERRORS", "SUM_WARNINGS", "SUM_NO_INDEX_USED", "SUM_NO_GOOD_INDEX_USED", "FIRST_SEEN", "LAST_SEEN"}
	selected := make([]string, 0, len(order))
	for _, name := range order {
		if available[name] {
			selected = append(selected, name)
		}
	}
	query := "SELECT " + strings.Join(selected, ", ") + " FROM performance_schema.events_statements_summary_by_digest WHERE DIGEST IS NOT NULL ORDER BY SUM_TIMER_WAIT DESC LIMIT " + strconv.Itoa(bitrix24LoadMaxDigestRows) + ";"
	lines, err := run(query)
	if err != nil {
		return nil, false
	}
	items := make([]bitrix24DigestSnapshot, 0, len(lines))
	for _, line := range lines {
		fields := strings.Split(line, "\t")
		if len(fields) != len(selected) {
			continue
		}
		values := make(map[string]string, len(selected))
		for index, name := range selected {
			values[name] = fields[index]
		}
		digest := normalizeBitrix24Digest(strings.TrimSpace(values["DIGEST"]))
		if digest == "" {
			continue
		}
		executions, executionsOK := parseBitrix24Uint64(values["COUNT_STAR"])
		total, totalOK := parseBitrix24Uint64(values["SUM_TIMER_WAIT"])
		if !executionsOK || !totalOK || executions == 0 {
			continue
		}
		normalizedSQL := normalizeBitrix24DigestSQL(values["DIGEST_TEXT"])
		if normalizedSQL == "" {
			// Digest metadata stays useful, but an unprovable SQL representation
			// is intentionally discarded instead of risking client data leakage.
			normalizedSQL = ""
		}
		item := bitrix24DigestSnapshot{
			Digest:        digest,
			Category:      classifyBitrix24DigestSQL(normalizedSQL),
			NormalizedSQL: normalizedSQL,
			Schema:        normalizeBitrix24DigestSchema(values["SCHEMA_NAME"]),
			Counters: bitrix24DigestCounters{
				Executions:   executions,
				TotalTimePS:  total,
				RowsExamined: parseBitrix24Uint64OrZero(values["SUM_ROWS_EXAMINED"]),
				RowsSent:     parseBitrix24Uint64OrZero(values["SUM_ROWS_SENT"]),
				Errors:       parseBitrix24Uint64OrZero(values["SUM_ERRORS"]),
				NoIndexUsed:  parseBitrix24Uint64OrZero(values["SUM_NO_INDEX_USED"]) + parseBitrix24Uint64OrZero(values["SUM_NO_GOOD_INDEX_USED"]),
				LastSeen:     parseBitrix24MySQLTimestamp(values["LAST_SEEN"]),
			},
			AvgTimeMS: bitrix24PicoToMilliseconds(parseBitrix24Uint64OrZero(values["AVG_TIMER_WAIT"])),
			MaxTimeMS: bitrix24PicoToMilliseconds(parseBitrix24Uint64OrZero(values["MAX_TIMER_WAIT"])),
			FirstSeen: parseBitrix24MySQLTimestamp(values["FIRST_SEEN"]),
			LastSeen:  parseBitrix24MySQLTimestamp(values["LAST_SEEN"]),
		}
		items = append(items, item)
	}
	return items, true
}

func normalizeBitrix24Digest(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) < 8 || len(value) > 128 {
		return ""
	}
	for _, symbol := range value {
		if (symbol >= 'a' && symbol <= 'f') || (symbol >= '0' && symbol <= '9') || symbol == '_' || symbol == '-' {
			continue
		}
		return ""
	}
	return value
}

func parseBitrix24Uint64(value string) (uint64, bool) {
	parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	return parsed, err == nil
}

func parseBitrix24Uint64OrZero(value string) uint64 {
	parsed, _ := parseBitrix24Uint64(value)
	return parsed
}

func bitrix24PicoToMilliseconds(value uint64) int64 {
	const divisor = uint64(1_000_000_000)
	value /= divisor
	if value > uint64(86_400_000) {
		return 86_400_000
	}
	return int64(value)
}

func saturatingBitrix24Uint64ToInt64(value uint64) int64 {
	if value > uint64(^uint64(0)>>1) {
		return int64(^uint64(0) >> 1)
	}
	return int64(value)
}

func parseBitrix24MySQLTimestamp(value string) time.Time {
	value = strings.TrimSpace(value)
	for _, layout := range []string{"2006-01-02 15:04:05.999999", "2006-01-02 15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func bitrix24DigestCounterReset(previous, current bitrix24DigestCounters) bool {
	return current.Executions < previous.Executions || current.TotalTimePS < previous.TotalTimePS || current.RowsExamined < previous.RowsExamined || current.RowsSent < previous.RowsSent || current.Errors < previous.Errors || current.NoIndexUsed < previous.NoIndexUsed
}

func bitrix24DigestCounterDelta(previous, current bitrix24DigestCounters) bitrix24DigestCounters {
	return bitrix24DigestCounters{
		Executions:   current.Executions - previous.Executions,
		TotalTimePS:  current.TotalTimePS - previous.TotalTimePS,
		RowsExamined: current.RowsExamined - previous.RowsExamined,
		RowsSent:     current.RowsSent - previous.RowsSent,
		Errors:       current.Errors - previous.Errors,
		NoIndexUsed:  current.NoIndexUsed - previous.NoIndexUsed,
		LastSeen:     current.LastSeen,
	}
}

func trimBitrix24DigestState(items map[string]bitrix24DigestCounters) {
	if len(items) <= 100 {
		return
	}
	type entry struct {
		digest string
		value  bitrix24DigestCounters
	}
	values := make([]entry, 0, len(items))
	for digest, value := range items {
		values = append(values, entry{digest: digest, value: value})
	}
	sort.Slice(values, func(left, right int) bool { return values[left].value.TotalTimePS > values[right].value.TotalTimePS })
	for _, value := range values[100:] {
		delete(items, value.digest)
	}
}

func normalizeBitrix24DigestSchema(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 96 || strings.ContainsAny(value, "\x00\r\n\t") {
		return ""
	}
	return value
}

func normalizeBitrix24DigestSQL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 20_000 {
		return ""
	}
	var builder strings.Builder
	for index := 0; index < len(value); {
		if index+1 < len(value) && value[index] == '/' && value[index+1] == '*' {
			end := strings.Index(value[index+2:], "*/")
			if end < 0 {
				return ""
			}
			index += end + 4
			continue
		}
		if value[index] == '#' || (index+2 < len(value) && value[index] == '-' && value[index+1] == '-' && (value[index+2] == ' ' || value[index+2] == '\t')) {
			for index < len(value) && value[index] != '\n' {
				index++
			}
			continue
		}
		if value[index] == '\'' || value[index] == '"' {
			quote := value[index]
			builder.WriteByte('?')
			index++
			for index < len(value) {
				if value[index] == '\\' && index+1 < len(value) {
					index += 2
					continue
				}
				if value[index] == quote {
					index++
					break
				}
				index++
			}
			continue
		}
		builder.WriteByte(value[index])
		index++
	}
	clean := strings.Join(strings.Fields(builder.String()), " ")
	clean = bitrix24SQLUUIDPattern.ReplaceAllString(clean, "?")
	clean = bitrix24SQLEmailPattern.ReplaceAllString(clean, "?")
	clean = bitrix24SQLNumberPattern.ReplaceAllString(clean, "?")
	upper := strings.ToUpper(strings.TrimSpace(clean))
	if !strings.HasPrefix(upper, "SELECT ") || strings.ContainsAny(clean, ";@") || strings.ContainsAny(clean, "\x00\r\n\t") {
		return ""
	}
	if len(clean) > 500 {
		clean = clean[:500]
	}
	return clean
}

func classifyBitrix24DigestSQL(value string) string {
	lower := strings.ToLower(value)
	switch {
	case lower == "":
		return "unknown_select"
	case strings.Contains(lower, "b_uts_crm_contact") || strings.Contains(lower, "b_crm_contact"):
		if strings.Contains(lower, "upper(") && strings.Contains(lower, " like ") {
			return "crm_contact_search"
		}
		return "crm_contact_query"
	case strings.Contains(lower, "b_crm_deal"):
		return "crm_deal_query"
	case strings.Contains(lower, "b_crm_lead"):
		return "crm_lead_query"
	case strings.Contains(lower, "b_crm_company"):
		return "crm_company_query"
	case strings.Contains(lower, "b_crm_dynamic"):
		return "crm_dynamic_item_query"
	case strings.Contains(lower, "b_option"):
		return "bitrix_option_query"
	case strings.Contains(lower, "b_event"):
		return "bitrix_event_query"
	case strings.Contains(lower, "b_session"):
		return "bitrix_session_query"
	case strings.Contains(lower, "b_cache"):
		return "bitrix_cache_query"
	case strings.Contains(lower, "b_user"):
		return "bitrix_user_query"
	case strings.HasPrefix(lower, "select "):
		return "unknown_select"
	default:
		return "other"
	}
}
