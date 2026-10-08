package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	databaseDiagnosticsSchemaVersion      = 1
	databaseDiagnosticsRetention          = 24 * time.Hour
	databaseDiagnosticsMaxStorageBytes    = int64(100 << 20)
	databaseDiagnosticsMaxQueueEvents     = 4
	databaseDiagnosticsMaxQueueLineBytes  = 256 << 10
	databaseDiagnosticsMaxQueryGroups     = 20
	databaseDiagnosticsIncidentInterval   = 10 * time.Second
	databaseDiagnosticsIncidentSamples    = 2
	databaseDiagnosticsConfigDefaultPath  = "/etc/pinguva-agent/database.json"
	databaseDiagnosticsStorageDefaultPath = "/var/lib/pinguva-agent/diagnostics/database"
)

type databaseDiagnosticsLocalConfig struct {
	Enabled       bool                         `json:"enabled"`
	IntegrationID string                       `json:"integrationId"`
	RequestedType string                       `json:"requestedType"`
	DatabaseType  string                       `json:"databaseType"`
	Thresholds    databaseDiagnosticThresholds `json:"thresholds"`
	CreatedAt     time.Time                    `json:"createdAt"`
	UpdatedAt     time.Time                    `json:"updatedAt"`
}

type databaseDiagnosticThresholds struct {
	Connections       int   `json:"connections"`
	ActiveQueries     int   `json:"activeQueries"`
	IdleInTransaction int   `json:"idleInTransaction"`
	LongestQueryMS    int64 `json:"longestQueryMs"`
	Waits             int   `json:"waits"`
	Locks             int   `json:"locks"`
	Deadlocks         int   `json:"deadlocks"`
}

type agentDatabaseDiagnosticCapabilities struct {
	StatusMetrics     bool   `json:"statusMetrics"`
	Processlist       bool   `json:"processlist"`
	QueryGroups       bool   `json:"queryGroups"`
	LockDiagnostics   bool   `json:"lockDiagnostics"`
	PermissionsCheck  bool   `json:"permissionsCheck"`
	SourceName        string `json:"sourceName,omitempty"`
	ConnectionMethod  string `json:"connectionMethod,omitempty"`
	PerformanceSchema bool   `json:"performanceSchema,omitempty"`
	PGStatStatements  bool   `json:"pgStatStatements,omitempty"`
	IncidentMode      bool   `json:"incidentMode,omitempty"`
}

type agentDatabaseDiagnosticMetrics struct {
	ThreadsRunning    int     `json:"threadsRunning,omitempty"`
	ThreadsConnected  int     `json:"threadsConnected,omitempty"`
	Connections       int     `json:"connections,omitempty"`
	ActiveConnections int     `json:"activeConnections,omitempty"`
	ActiveQueries     int     `json:"activeQueries,omitempty"`
	IdleInTransaction int     `json:"idleInTransaction,omitempty"`
	LongestQueryMS    int64   `json:"longestQueryMs,omitempty"`
	WaitingQueries    int     `json:"waitingQueries,omitempty"`
	LockWaitCount     int     `json:"lockWaitCount,omitempty"`
	BlockingSessions  int     `json:"blockingSessions,omitempty"`
	BlockedSessions   int     `json:"blockedSessions,omitempty"`
	Deadlocks         int64   `json:"deadlocks,omitempty"`
	Conflicts         int64   `json:"conflicts,omitempty"`
	Commits           int64   `json:"commits,omitempty"`
	Rollbacks         int64   `json:"rollbacks,omitempty"`
	TuplesReturned    int64   `json:"tuplesReturned,omitempty"`
	TuplesFetched     int64   `json:"tuplesFetched,omitempty"`
	TuplesInserted    int64   `json:"tuplesInserted,omitempty"`
	TuplesUpdated     int64   `json:"tuplesUpdated,omitempty"`
	TuplesDeleted     int64   `json:"tuplesDeleted,omitempty"`
	TempFiles         int64   `json:"tempFiles,omitempty"`
	TempBytes         int64   `json:"tempBytes,omitempty"`
	CacheHitRatio     float64 `json:"cacheHitRatio,omitempty"`
	WALBytes          int64   `json:"walBytes,omitempty"`
	FullScanCount     int64   `json:"fullScanCount,omitempty"`
	NoIndexCount      int64   `json:"noIndexCount,omitempty"`
}

type agentDatabaseDiagnosticQueryGroup struct {
	Hash             string `json:"hash"`
	Operation        string `json:"operation"`
	Database         string `json:"database,omitempty"`
	Executions       int64  `json:"executions"`
	TotalTimeMS      int64  `json:"totalTimeMs"`
	AvgTimeMS        int64  `json:"avgTimeMs"`
	MaxTimeMS        int64  `json:"maxTimeMs"`
	RowsExamined     int64  `json:"rowsExamined,omitempty"`
	RowsReturned     int64  `json:"rowsReturned,omitempty"`
	TempTables       int64  `json:"tempTables,omitempty"`
	SharedBlocksRead int64  `json:"sharedBlocksRead,omitempty"`
	SharedBlocksHit  int64  `json:"sharedBlocksHit,omitempty"`
	Errors           int64  `json:"errors,omitempty"`
	NoIndexUsed      int64  `json:"noIndexUsed,omitempty"`
}

type agentDatabaseDiagnosticSample struct {
	EventID         string                              `json:"eventId,omitempty"`
	CapturedAt      time.Time                           `json:"capturedAt"`
	BucketSeconds   int                                 `json:"bucketSeconds"`
	SampleKind      string                              `json:"sampleKind"`
	DatabaseType    string                              `json:"databaseType"`
	DatabaseVersion string                              `json:"databaseVersion,omitempty"`
	Instance        string                              `json:"instance"`
	Endpoint        string                              `json:"endpoint,omitempty"`
	ConnectionType  string                              `json:"connectionType,omitempty"`
	Status          string                              `json:"status"`
	Metrics         agentDatabaseDiagnosticMetrics      `json:"metrics"`
	Capabilities    agentDatabaseDiagnosticCapabilities `json:"capabilities"`
	SourceStatuses  map[string]string                   `json:"sourceStatuses"`
	QueryGroups     []agentDatabaseDiagnosticQueryGroup `json:"queryGroups,omitempty"`
}

type agentDatabaseDiagnosticIncident struct {
	EventID       string                          `json:"eventId,omitempty"`
	IncidentType  string                          `json:"incidentType"`
	DatabaseType  string                          `json:"databaseType"`
	Severity      string                          `json:"severity"`
	Status        string                          `json:"status"`
	StartedAt     time.Time                       `json:"startedAt"`
	EndedAt       time.Time                       `json:"endedAt,omitempty"`
	DurationMS    int64                           `json:"durationMs"`
	Peaks         agentDatabaseDiagnosticMetrics  `json:"peaks"`
	RecoveryState string                          `json:"recoveryState,omitempty"`
	Samples       []agentDatabaseDiagnosticSample `json:"samples,omitempty"`
}

type agentDatabaseDiagnosticEvent struct {
	SchemaVersion int                              `json:"schemaVersion"`
	EventID       string                           `json:"eventId"`
	Kind          string                           `json:"kind"`
	Sample        *agentDatabaseDiagnosticSample   `json:"sample,omitempty"`
	Incident      *agentDatabaseDiagnosticIncident `json:"incident,omitempty"`
}

type agentDatabaseDiagnosticEventBatch struct {
	Events []agentDatabaseDiagnosticEvent `json:"events"`
}
type agentDatabaseDiagnosticEventBatchResponse struct {
	EventAck   []string                     `json:"eventAck,omitempty"`
	Enabled    bool                         `json:"enabled"`
	Thresholds databaseDiagnosticThresholds `json:"thresholds"`
}
type databaseDiagnosticQueueRecord struct {
	AgentID string                       `json:"agentId"`
	Event   agentDatabaseDiagnosticEvent `json:"event"`
}

type databaseDigestState struct {
	Executions       uint64 `json:"executions"`
	TotalTime        uint64 `json:"totalTime"`
	RowsExamined     uint64 `json:"rowsExamined"`
	RowsReturned     uint64 `json:"rowsReturned"`
	TempTables       uint64 `json:"tempTables"`
	SharedBlocksRead uint64 `json:"sharedBlocksRead"`
	SharedBlocksHit  uint64 `json:"sharedBlocksHit"`
	Errors           uint64 `json:"errors"`
	NoIndexUsed      uint64 `json:"noIndexUsed"`
}

type databaseDiagnosticsState struct {
	Digests        map[string]databaseDigestState   `json:"digests,omitempty"`
	GlobalCounters map[string]uint64                `json:"globalCounters,omitempty"`
	PostgresTotals *agentDatabaseDiagnosticMetrics  `json:"postgresTotals,omitempty"`
	ActiveIncident *agentDatabaseDiagnosticIncident `json:"activeIncident,omitempty"`
}

func defaultDatabaseDiagnosticsConfigPath() string {
	if runtime.GOOS == "linux" {
		return databaseDiagnosticsConfigDefaultPath
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".pinguva-agent", "database.json")
	}
	return "pinguva-agent-database.json"
}

func defaultDatabaseDiagnosticsStoragePath() string {
	if runtime.GOOS == "linux" {
		return databaseDiagnosticsStorageDefaultPath
	}
	return filepath.Join(filepath.Dir(defaultStatePath()), "diagnostics", "database")
}

func runDatabaseCommand(args []string, logger *log.Logger) error {
	if len(args) == 0 {
		return errors.New("usage: pinguva-agent database configure --type auto --integration-id dbi_xxx")
	}
	switch args[0] {
	case "configure", "connect":
		return runDatabaseConfigure(args[1:], logger)
	case "diagnostics", "check":
		return runDatabaseDiagnostics(args[1:], logger)
	case "status":
		return runDatabaseStatus(args[1:])
	case "disable":
		return runDatabaseDisable(args[1:], logger)
	default:
		return fmt.Errorf("unknown database command %q", args[0])
	}
}

func runDatabaseConfigure(args []string, logger *log.Logger) error {
	fs := flag.NewFlagSet("database configure", flag.ContinueOnError)
	typeFlag := fs.String("type", "auto", "auto, mysql, mariadb or postgresql")
	integrationFlag := fs.String("integration-id", "", "Pinguva database integration id")
	configFlag := fs.String("config-path", defaultDatabaseDiagnosticsConfigPath(), "Local database diagnostics config")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if runtime.GOOS != "linux" {
		return errors.New("database diagnostics are available only on Linux")
	}
	if os.Geteuid() != 0 {
		return errors.New("run database configuration through sudo")
	}
	if localDiagnosticsConfigExists(defaultBitrix24ConfigPath()) {
		return errors.New("database diagnostics are already provided by the local Bitrix24 integration")
	}
	requested := normalizeAgentDatabaseType(*typeFlag, true)
	if requested == "" {
		return errors.New("database type must be auto, mysql, mariadb or postgresql")
	}
	integrationID := normalizeAgentDatabaseIdentifier(*integrationFlag, 4, 96)
	if integrationID == "" {
		return errors.New("integration-id is required")
	}
	detected, found, err := detectLocalDatabaseTypes(requested)
	if err != nil {
		return err
	}
	if requested == "auto" && len(found) > 1 {
		return fmt.Errorf("multiple database engines detected (%s); run the command again with --type", strings.Join(found, ", "))
	}
	if detected == "" {
		return errors.New("no supported local database connection was detected")
	}
	now := time.Now().UTC()
	config := &databaseDiagnosticsLocalConfig{Enabled: true, IntegrationID: integrationID, RequestedType: requested, DatabaseType: detected, Thresholds: defaultAgentDatabaseThresholds(detected), CreatedAt: now, UpdatedAt: now}
	if previous, err := loadDatabaseDiagnosticsConfig(*configFlag); err == nil && previous != nil && !previous.CreatedAt.IsZero() {
		config.CreatedAt = previous.CreatedAt
		config.Thresholds = previous.Thresholds
	}
	if err := saveDatabaseDiagnosticsConfig(*configFlag, config); err != nil {
		return err
	}
	if err := installDatabaseDiagnosticsTimer(); err != nil {
		return err
	}
	if logger != nil {
		logger.Printf("database diagnostics configured: type=%s connection=local", detected)
	}
	return nil
}

func runDatabaseStatus(args []string) error {
	fs := flag.NewFlagSet("database status", flag.ContinueOnError)
	configFlag := fs.String("config-path", defaultDatabaseDiagnosticsConfigPath(), "Local config")
	if err := fs.Parse(args); err != nil {
		return err
	}
	config, err := loadDatabaseDiagnosticsConfig(*configFlag)
	if err != nil {
		return err
	}
	if config == nil || !config.Enabled {
		return errors.New("database diagnostics are not configured")
	}
	fmt.Fprintf(os.Stdout, "Database diagnostics: enabled\nType: %s\nIntegration: %s\n", config.DatabaseType, config.IntegrationID)
	return nil
}

func runDatabaseDisable(args []string, logger *log.Logger) error {
	fs := flag.NewFlagSet("database disable", flag.ContinueOnError)
	configFlag := fs.String("config-path", defaultDatabaseDiagnosticsConfigPath(), "Local config")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if runtime.GOOS != "linux" {
		return errors.New("database diagnostics are available only on Linux")
	}
	if os.Geteuid() != 0 {
		return errors.New("run database disable through sudo")
	}
	for _, unit := range []string{"pinguva-database-diagnostics.timer", "pinguva-database-diagnostics.service"} {
		_, _ = exec.Command("systemctl", "disable", "--now", unit).CombinedOutput()
	}
	if err := os.Remove(*configFlag); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if logger != nil {
		logger.Printf("database diagnostics disabled; the main Pinguva Agent remains installed")
	}
	return nil
}

func normalizeAgentDatabaseType(value string, allowAuto bool) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if allowAuto && value == "auto" {
		return value
	}
	switch value {
	case "mysql", "mariadb", "postgresql":
		return value
	case "postgres", "pgsql":
		return "postgresql"
	}
	return ""
}
func normalizeAgentDatabaseIdentifier(value string, min, max int) string {
	value = strings.TrimSpace(value)
	if len(value) < min || len(value) > max {
		return ""
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("_-.:", r) {
			continue
		}
		return ""
	}
	return value
}

func defaultAgentDatabaseThresholds(databaseType string) databaseDiagnosticThresholds {
	if databaseType == "postgresql" {
		return databaseDiagnosticThresholds{Connections: 100, ActiveQueries: 20, IdleInTransaction: 5, LongestQueryMS: 5000, Waits: 5, Locks: 5, Deadlocks: 1}
	}
	return databaseDiagnosticThresholds{Connections: 100, ActiveQueries: 10, LongestQueryMS: 5000, Waits: 5, Locks: 5}
}

func loadDatabaseDiagnosticsConfig(path string) (*databaseDiagnosticsLocalConfig, error) {
	body, err := readDatabaseDiagnosticsFile(path, 1<<20)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var item databaseDiagnosticsLocalConfig
	if err := json.Unmarshal(body, &item); err != nil {
		return nil, err
	}
	item.RequestedType = normalizeAgentDatabaseType(item.RequestedType, true)
	item.DatabaseType = normalizeAgentDatabaseType(item.DatabaseType, false)
	if item.IntegrationID == "" || item.DatabaseType == "" {
		return nil, errors.New("database diagnostics config is invalid")
	}
	return &item, nil
}

func saveDatabaseDiagnosticsConfig(path string, item *databaseDiagnosticsLocalConfig) error {
	if item == nil {
		return errors.New("database diagnostics config is empty")
	}
	body, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return err
	}
	return writeDatabaseDiagnosticsFile(path, body)
}

func detectLocalDatabaseTypes(requested string) (string, []string, error) {
	found := []string{}
	if requested == "auto" || requested == "mysql" || requested == "mariadb" {
		if databaseType, _, _, err := detectMySQLDatabase(); err == nil {
			found = append(found, databaseType)
			if requested != "auto" {
				if requested == "mariadb" && databaseType != "mariadb" {
					return "", found, errors.New("the local server reports MySQL, not MariaDB")
				}
				if requested == "mysql" && databaseType == "mariadb" {
					return "", found, errors.New("the local server reports MariaDB, not MySQL")
				}
				return databaseType, found, nil
			}
		}
	}
	if requested == "auto" || requested == "postgresql" {
		if _, _, err := detectPostgreSQLDatabase(); err == nil {
			found = append(found, "postgresql")
			if requested == "postgresql" {
				return "postgresql", found, nil
			}
		}
	}
	if requested != "auto" && len(found) == 0 {
		return "", found, fmt.Errorf("%s local read-only connection is unavailable", requested)
	}
	if len(found) == 1 {
		return found[0], found, nil
	}
	return "", found, nil
}

func detectMySQLDatabase() (string, string, string, error) {
	binary, err := findBitrix24MySQLClient()
	if err != nil {
		return "", "", "", err
	}
	defaults, connection, _ := bitrix24RootMySQLConnection()
	lines, err := runBitrix24MySQLQueryWithOption(binary, "SELECT @@version, @@version_comment;", defaults)
	if err != nil || len(lines) != 1 {
		return "", "", connection, firstNonNil(err, errors.New("MySQL version query failed"))
	}
	parts := strings.Split(lines[0], "\t")
	if len(parts) != 2 {
		return "", "", connection, errors.New("MySQL version result is invalid")
	}
	databaseType := "mysql"
	if strings.Contains(strings.ToLower(parts[0]+" "+parts[1]), "mariadb") {
		databaseType = "mariadb"
	}
	return databaseType, safeDatabaseVersion(parts[0]), connection, nil
}

func firstNonNil(primary, errorFallback error) error {
	if primary != nil {
		return primary
	}
	return errorFallback
}
func safeDatabaseVersion(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 120 {
		value = value[:120]
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("._-+ ()", r) {
			continue
		}
		return ""
	}
	return value
}

type postgresQueryRunner func(string) ([]string, error)

func detectPostgreSQLDatabase() (string, string, error) {
	lines, err := runPostgresLocalQuery("SHOW server_version;")
	if err != nil || len(lines) != 1 {
		return "", "", firstNonNil(err, errors.New("PostgreSQL version query failed"))
	}
	return safeDatabaseVersion(lines[0]), "local_peer", nil
}

func runPostgresLocalQuery(query string) ([]string, error) {
	binary, err := exec.LookPath("psql")
	if err != nil {
		return nil, errors.New("psql client not found")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	args := []string{"-X", "-A", "-t", "-q", "-v", "ON_ERROR_STOP=1", "-c", query}
	var command *exec.Cmd
	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		if runuser, runErr := exec.LookPath("runuser"); runErr == nil {
			command = exec.CommandContext(ctx, runuser, append([]string{"-u", "postgres", "--", binary}, args...)...)
		} else {
			command = exec.CommandContext(ctx, binary, args...)
		}
	} else {
		command = exec.CommandContext(ctx, binary, args...)
	}
	command.Env = append(os.Environ(), "LANG=C", "PGAPPNAME=pinguva-agent-diagnostics")
	body, err := command.CombinedOutput()
	if ctx.Err() != nil {
		return nil, errors.New("timeout")
	}
	if err != nil {
		return nil, errors.New(classifyDatabaseCommandError(string(body)))
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) == 1 && strings.TrimSpace(lines[0]) == "" {
		return nil, nil
	}
	if len(lines) > 64 {
		lines = lines[:64]
	}
	return lines, nil
}

func classifyDatabaseCommandError(value string) string {
	value = strings.ToLower(value)
	switch {
	case strings.Contains(value, "permission denied"), strings.Contains(value, "must be superuser"), strings.Contains(value, "insufficient privilege"):
		return "permission_denied"
	case strings.Contains(value, "does not exist"), strings.Contains(value, "undefined table"), strings.Contains(value, "unrecognized configuration"):
		return "unsupported"
	case strings.Contains(value, "could not connect"), strings.Contains(value, "connection refused"), strings.Contains(value, "no such file"):
		return "connection_failed"
	default:
		return "query_failed"
	}
}

func runDatabaseDiagnostics(args []string, logger *log.Logger) error {
	fs := flag.NewFlagSet("database diagnostics", flag.ContinueOnError)
	configFlag := fs.String("config-path", defaultDatabaseDiagnosticsConfigPath(), "Local config")
	if err := fs.Parse(args); err != nil {
		return err
	}
	config, err := loadDatabaseDiagnosticsConfig(*configFlag)
	if err != nil {
		return err
	}
	if config == nil || !config.Enabled {
		return nil
	}
	if localDiagnosticsConfigExists(defaultBitrix24ConfigPath()) {
		return errors.New("database diagnostics stopped because Bitrix24 diagnostics are configured")
	}
	release, err := acquireDatabaseDiagnosticsLock()
	if err != nil {
		return err
	}
	defer release()
	state, err := loadDatabaseDiagnosticsState()
	if err != nil {
		state = &databaseDiagnosticsState{Digests: map[string]databaseDigestState{}, GlobalCounters: map[string]uint64{}}
	}
	sample, err := collectDatabaseDiagnosticSample(config, state, "minute", logger)
	if err != nil {
		return err
	}
	events := []agentDatabaseDiagnosticEvent{newDatabaseSampleEvent(sample)}
	incidentMetrics := sample.Metrics
	if sample.DatabaseType == "postgresql" {
		incidentMetrics = databaseIncidentMetrics(sample.Metrics, state)
	}
	triggered, incidentType := databaseDiagnosticTriggered(incidentMetrics, config.Thresholds, sample.DatabaseType)
	if triggered && state.ActiveIncident == nil {
		incident := &agentDatabaseDiagnosticIncident{EventID: newDatabaseEventID("dbx"), IncidentType: incidentType, DatabaseType: sample.DatabaseType, Severity: databaseIncidentSeverity(incidentMetrics, config.Thresholds), Status: "active", StartedAt: sample.CapturedAt, Peaks: incidentMetrics}
		state.ActiveIncident = incident
		for index := 0; index < databaseDiagnosticsIncidentSamples; index++ {
			time.Sleep(databaseDiagnosticsIncidentInterval)
			extra, extraErr := collectDatabaseDiagnosticSample(config, state, "incident", logger)
			if extraErr == nil {
				events = append(events, newDatabaseSampleEvent(extra))
				incident.Samples = append(incident.Samples, *extra)
				extraMetrics := extra.Metrics
				if extra.DatabaseType == "postgresql" {
					extraMetrics = databaseIncidentMetrics(extra.Metrics, state)
				}
				mergeDatabaseMetricPeaks(&incident.Peaks, extraMetrics)
			}
		}
		events = append(events, newDatabaseIncidentEvent(incident))
	} else if !triggered && state.ActiveIncident != nil {
		resolved := *state.ActiveIncident
		resolved.Status = "resolved"
		resolved.EndedAt = sample.CapturedAt
		resolved.DurationMS = sample.CapturedAt.Sub(resolved.StartedAt).Milliseconds()
		resolved.RecoveryState = "recovered"
		events = append(events, newDatabaseIncidentEvent(&resolved))
		state.ActiveIncident = nil
	} else if triggered && state.ActiveIncident != nil {
		mergeDatabaseMetricPeaks(&state.ActiveIncident.Peaks, incidentMetrics)
	}
	agentState, err := loadAgentState(defaultStatePath())
	if err != nil || agentState == nil || agentState.AgentID == "" || agentState.Token == "" {
		return errors.New("agent state is unavailable")
	}
	for _, event := range events {
		if err := appendDatabaseDiagnosticsQueue(databaseDiagnosticQueueRecord{AgentID: agentState.AgentID, Event: event}, sample.CapturedAt); err != nil {
			return err
		}
	}
	if err := saveDatabaseDiagnosticsState(state); err != nil {
		return err
	}
	_ = trimDatabaseDiagnosticsStorage(time.Now().UTC())
	enabled, thresholds := flushDatabaseDiagnosticsEvents(envOr("AGENT_SERVER", ""), agentState, logger)
	if !enabled {
		config.Enabled = false
	}
	if thresholds.LongestQueryMS > 0 {
		config.Thresholds = thresholds
	}
	config.UpdatedAt = time.Now().UTC()
	_ = saveDatabaseDiagnosticsConfig(*configFlag, config)
	if logger != nil {
		logger.Printf("database diagnostics: type=%s status=%s active_queries=%d longest_query_ms=%d", sample.DatabaseType, sample.Status, sample.Metrics.ActiveQueries, sample.Metrics.LongestQueryMS)
	}
	return nil
}

func acquireDatabaseDiagnosticsLock() (func(), error) {
	dir := defaultDatabaseDiagnosticsStoragePath()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "collector.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) {
		if info, statErr := os.Lstat(path); statErr == nil && info.Mode().IsRegular() && time.Since(info.ModTime()) > 2*time.Minute {
			_ = os.Remove(path)
			file, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		}
	}
	if err != nil {
		return nil, errors.New("database diagnostics are already running")
	}
	_ = file.Close()
	return func() { _ = os.Remove(path) }, nil
}

func collectDatabaseDiagnosticSample(config *databaseDiagnosticsLocalConfig, state *databaseDiagnosticsState, kind string, logger *log.Logger) (*agentDatabaseDiagnosticSample, error) {
	switch config.DatabaseType {
	case "mysql", "mariadb":
		return collectMySQLDatabaseSample(config, state, kind, logger)
	case "postgresql":
		return collectPostgresDatabaseSample(config, state, kind, logger)
	default:
		return nil, errors.New("unsupported database type")
	}
}

func collectMySQLDatabaseSample(config *databaseDiagnosticsLocalConfig, state *databaseDiagnosticsState, kind string, logger *log.Logger) (*agentDatabaseDiagnosticSample, error) {
	databaseType, version, connection, detectErr := detectMySQLDatabase()
	diagnostics := collectBitrix24MySQLDiagnosticsWithLogger(logger)
	status := "unavailable"
	if diagnostics != nil {
		status = diagnostics.Status
	}
	sources := map[string]string{"global_status": "unavailable", "processlist": "unavailable", "query_groups": "unavailable", "locks": "unsupported", "permissions": "unavailable"}
	capabilities := agentDatabaseDiagnosticCapabilities{SourceName: "mysql", ConnectionMethod: connection, IncidentMode: true}
	metrics := agentDatabaseDiagnosticMetrics{}
	if diagnostics != nil {
		metrics.ThreadsRunning = diagnostics.ThreadsRunning
		metrics.ThreadsConnected = diagnostics.ThreadsConnected
		metrics.Connections = diagnostics.ThreadsConnected
		metrics.ActiveConnections = diagnostics.ThreadsRunning
		metrics.ActiveQueries = diagnostics.ActiveQueries
		metrics.WaitingQueries = diagnostics.WaitingTransactions
		metrics.LongestQueryMS = diagnostics.LongestQuerySec * 1000
		metrics.LockWaitCount = diagnostics.LockWaitCount
		metrics.BlockingSessions = diagnostics.BlockingTransactions
		metrics.BlockedSessions = diagnostics.WaitingTransactions
		capabilities.StatusMetrics = diagnostics.Status != "unavailable"
		capabilities.Processlist = diagnostics.ProcesslistStatus == "ok"
		capabilities.LockDiagnostics = diagnostics.LockDiagnostics == "ok"
		capabilities.PermissionsCheck = diagnostics.ProcesslistStatus == "ok"
		capabilities.PerformanceSchema = diagnostics.QueryGroupsStatus == "ok"
		sources["global_status"] = componentStatus(capabilities.StatusMetrics)
		sources["processlist"] = diagnostics.ProcesslistStatus
		sources["locks"] = diagnostics.LockDiagnostics
		sources["permissions"] = componentStatus(capabilities.PermissionsCheck)
	}
	groups, groupsOK := collectMySQLDatabaseDigestGroups(state)
	collectMySQLGlobalCounterDeltas(&metrics, state)
	for _, group := range groups {
		metrics.NoIndexCount += group.NoIndexUsed
	}
	capabilities.QueryGroups = groupsOK
	sources["query_groups"] = componentStatus(groupsOK)
	if !groupsOK && status == "ok" {
		status = "partial"
	}
	if detectErr != nil {
		sources["connection"] = "unavailable"
	}
	return &agentDatabaseDiagnosticSample{CapturedAt: time.Now().UTC(), BucketSeconds: databaseSampleBucketSeconds(kind), SampleKind: kind, DatabaseType: firstDatabaseValue(databaseType, config.DatabaseType), DatabaseVersion: version, Instance: "default", Endpoint: "local socket", ConnectionType: connection, Status: status, Metrics: metrics, Capabilities: capabilities, SourceStatuses: sources, QueryGroups: groups}, nil
}

func collectMySQLGlobalCounterDeltas(metrics *agentDatabaseDiagnosticMetrics, state *databaseDiagnosticsState) bool {
	if metrics == nil || state == nil {
		return false
	}
	binary, err := findBitrix24MySQLClient()
	if err != nil {
		return false
	}
	defaults, _, _ := bitrix24RootMySQLConnection()
	lines, err := runBitrix24MySQLQueryWithOption(binary, `SHOW GLOBAL STATUS WHERE Variable_name IN ('Created_tmp_disk_tables', 'Select_scan');`, defaults)
	if err != nil {
		return false
	}
	return applyMySQLGlobalCounterDeltas(metrics, state, lines)
}

func applyMySQLGlobalCounterDeltas(metrics *agentDatabaseDiagnosticMetrics, state *databaseDiagnosticsState, lines []string) bool {
	if metrics == nil || state == nil {
		return false
	}
	current := map[string]uint64{}
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}
		value, parseErr := strconv.ParseUint(parts[1], 10, 64)
		if parseErr == nil {
			current[strings.ToLower(parts[0])] = value
		}
	}
	if len(current) == 0 {
		return false
	}
	if state.GlobalCounters == nil {
		state.GlobalCounters = map[string]uint64{}
	}
	if value, ok := current["created_tmp_disk_tables"]; ok {
		if previous, exists := state.GlobalCounters["created_tmp_disk_tables"]; exists {
			metrics.TempFiles = safeUint64Int64(counterDelta(previous, value))
		}
		state.GlobalCounters["created_tmp_disk_tables"] = value
	}
	if value, ok := current["select_scan"]; ok {
		if previous, exists := state.GlobalCounters["select_scan"]; exists {
			metrics.FullScanCount = safeUint64Int64(counterDelta(previous, value))
		}
		state.GlobalCounters["select_scan"] = value
	}
	return true
}

func firstDatabaseValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func componentStatus(value bool) string {
	if value {
		return "ok"
	}
	return "unavailable"
}
func databaseSampleBucketSeconds(kind string) int {
	if kind == "incident" {
		return 10
	}
	return 60
}

func collectMySQLDatabaseDigestGroups(state *databaseDiagnosticsState) ([]agentDatabaseDiagnosticQueryGroup, bool) {
	if state == nil {
		return nil, false
	}
	binary, err := findBitrix24MySQLClient()
	if err != nil {
		return nil, false
	}
	defaults, _, _ := bitrix24RootMySQLConnection()
	snapshot, ok := collectBitrix24DigestSnapshot(func(query string) ([]string, error) { return runBitrix24MySQLQueryWithOption(binary, query, defaults) })
	if !ok {
		return nil, false
	}
	if state.Digests == nil {
		state.Digests = map[string]databaseDigestState{}
	}
	out := []agentDatabaseDiagnosticQueryGroup{}
	for _, current := range snapshot {
		next := databaseDigestState{Executions: current.Counters.Executions, TotalTime: current.Counters.TotalTimePS, RowsExamined: current.Counters.RowsExamined, RowsReturned: current.Counters.RowsSent, Errors: current.Counters.Errors, NoIndexUsed: current.Counters.NoIndexUsed}
		previous, exists := state.Digests[current.Digest]
		state.Digests[current.Digest] = next
		if !exists || databaseDigestReset(previous, next) {
			continue
		}
		delta := databaseDigestDelta(previous, next)
		if delta.Executions == 0 {
			continue
		}
		out = append(out, agentDatabaseDiagnosticQueryGroup{Hash: current.Digest, Operation: databaseSQLCategory(current.NormalizedSQL), Database: safeDatabaseName(current.Schema), Executions: int64(delta.Executions), TotalTimeMS: bitrix24PicoToMilliseconds(delta.TotalTime), AvgTimeMS: current.AvgTimeMS, MaxTimeMS: current.MaxTimeMS, RowsExamined: safeUint64Int64(delta.RowsExamined), RowsReturned: safeUint64Int64(delta.RowsReturned), Errors: safeUint64Int64(delta.Errors), NoIndexUsed: safeUint64Int64(delta.NoIndexUsed)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TotalTimeMS > out[j].TotalTimeMS })
	if len(out) > databaseDiagnosticsMaxQueryGroups {
		out = out[:databaseDiagnosticsMaxQueryGroups]
	}
	trimDatabaseDigestState(state.Digests)
	return out, true
}

func databaseSQLCategory(value string) string {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(value)))
	if len(fields) == 0 {
		return "other"
	}
	switch fields[0] {
	case "select", "insert", "update", "delete", "alter", "create", "drop":
		return fields[0]
	}
	return "other"
}
func safeDatabaseName(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 96 {
		value = value[:96]
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("_-", r) {
			continue
		}
		return ""
	}
	return value
}
func databaseDigestReset(a, b databaseDigestState) bool {
	return b.Executions < a.Executions || b.TotalTime < a.TotalTime
}
func databaseDigestDelta(a, b databaseDigestState) databaseDigestState {
	return databaseDigestState{Executions: b.Executions - a.Executions, TotalTime: b.TotalTime - a.TotalTime, RowsExamined: counterDelta(a.RowsExamined, b.RowsExamined), RowsReturned: counterDelta(a.RowsReturned, b.RowsReturned), TempTables: counterDelta(a.TempTables, b.TempTables), SharedBlocksRead: counterDelta(a.SharedBlocksRead, b.SharedBlocksRead), SharedBlocksHit: counterDelta(a.SharedBlocksHit, b.SharedBlocksHit), Errors: counterDelta(a.Errors, b.Errors), NoIndexUsed: counterDelta(a.NoIndexUsed, b.NoIndexUsed)}
}
func counterDelta(a, b uint64) uint64 {
	if b < a {
		return 0
	}
	return b - a
}
func safeUint64Int64(value uint64) int64 {
	if value > uint64(^uint64(0)>>1) {
		return int64(^uint64(0) >> 1)
	}
	return int64(value)
}
func trimDatabaseDigestState(items map[string]databaseDigestState) {
	if len(items) <= 200 {
		return
	}
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys[:len(keys)-200] {
		delete(items, key)
	}
}

const postgresActivityQuery = `SELECT count(*),count(*) FILTER (WHERE state='active'),count(*) FILTER (WHERE state='idle in transaction'),COALESCE(max(EXTRACT(EPOCH FROM (clock_timestamp()-query_start))*1000) FILTER (WHERE state='active' AND pid<>pg_backend_pid()),0)::bigint,count(*) FILTER (WHERE wait_event_type IS NOT NULL),count(*) FILTER (WHERE state='active' AND pid<>pg_backend_pid()) FROM pg_stat_activity;`
const postgresDatabaseStatsQuery = `SELECT COALESCE(sum(xact_commit),0),COALESCE(sum(xact_rollback),0),COALESCE(sum(tup_returned),0),COALESCE(sum(tup_fetched),0),COALESCE(sum(tup_inserted),0),COALESCE(sum(tup_updated),0),COALESCE(sum(tup_deleted),0),COALESCE(sum(temp_files),0),COALESCE(sum(temp_bytes),0),COALESCE(sum(deadlocks),0),CASE WHEN COALESCE(sum(blks_hit)+sum(blks_read),0)>0 THEN round(100.0*sum(blks_hit)/(sum(blks_hit)+sum(blks_read)),2) ELSE 0 END FROM pg_stat_database;`
const postgresConflictsQuery = `SELECT COALESCE(sum(confl_tablespace+confl_lock+confl_snapshot+confl_bufferpin+confl_deadlock),0) FROM pg_stat_database_conflicts;`
const postgresLocksQuery = `SELECT count(*) FILTER (WHERE NOT granted),count(DISTINCT pid) FILTER (WHERE NOT granted),count(DISTINCT pid) FILTER (WHERE granted) FROM pg_locks;`
const postgresWALQuery = `SELECT COALESCE(wal_bytes,0)::bigint FROM pg_stat_wal;`
const postgresStatementsQueryWithMax = `SELECT queryid::text,dbid::text,calls::bigint,total_exec_time::bigint,mean_exec_time::bigint,max_exec_time::bigint,rows::bigint,shared_blks_read::bigint,shared_blks_hit::bigint FROM pg_stat_statements WHERE queryid IS NOT NULL ORDER BY total_exec_time DESC LIMIT 50;`
const postgresStatementsQuery = `SELECT queryid::text,dbid::text,calls::bigint,total_exec_time::bigint,mean_exec_time::bigint,rows::bigint,shared_blks_read::bigint,shared_blks_hit::bigint FROM pg_stat_statements WHERE queryid IS NOT NULL ORDER BY total_exec_time DESC LIMIT 50;`

func collectPostgresDatabaseSample(config *databaseDiagnosticsLocalConfig, state *databaseDiagnosticsState, kind string, logger *log.Logger) (*agentDatabaseDiagnosticSample, error) {
	version, connection, versionErr := detectPostgreSQLDatabase()
	runner := postgresQueryRunner(runPostgresLocalQuery)
	metrics := agentDatabaseDiagnosticMetrics{}
	caps := agentDatabaseDiagnosticCapabilities{SourceName: "postgresql", ConnectionMethod: connection, IncidentMode: true}
	sources := map[string]string{"pg_stat_activity": "unavailable", "pg_stat_database": "unavailable", "pg_stat_database_conflicts": "unsupported", "pg_locks": "unavailable", "pg_stat_statements": "unsupported", "pg_stat_wal": "unsupported", "permissions": "unavailable"}
	activity, err := runner(postgresActivityQuery)
	if err == nil && len(activity) == 1 {
		parts := strings.Split(activity[0], "|")
		if len(parts) == 6 {
			metrics.Connections = safeAgentDBInt(parts[0])
			metrics.ActiveConnections = safeAgentDBInt(parts[1])
			metrics.IdleInTransaction = safeAgentDBInt(parts[2])
			metrics.LongestQueryMS = safeAgentDBInt64(parts[3])
			metrics.WaitingQueries = safeAgentDBInt(parts[4])
			metrics.ActiveQueries = safeAgentDBInt(parts[5])
			caps.StatusMetrics = true
			caps.Processlist = true
			caps.PermissionsCheck = true
			sources["pg_stat_activity"] = "ok"
			sources["permissions"] = "ok"
		}
	} else if err != nil {
		sources["pg_stat_activity"] = databasePostgresSourceStatus(err)
		sources["permissions"] = databasePostgresSourceStatus(err)
	}
	conflicts, err := runner(postgresConflictsQuery)
	if err == nil && len(conflicts) == 1 {
		metrics.Conflicts = safeAgentDBInt64(conflicts[0])
		sources["pg_stat_database_conflicts"] = "ok"
	} else if err != nil {
		sources["pg_stat_database_conflicts"] = databasePostgresSourceStatus(err)
	}
	stats, err := runner(postgresDatabaseStatsQuery)
	if err == nil && len(stats) == 1 {
		parts := strings.Split(stats[0], "|")
		if len(parts) == 11 {
			metrics.Commits = safeAgentDBInt64(parts[0])
			metrics.Rollbacks = safeAgentDBInt64(parts[1])
			metrics.TuplesReturned = safeAgentDBInt64(parts[2])
			metrics.TuplesFetched = safeAgentDBInt64(parts[3])
			metrics.TuplesInserted = safeAgentDBInt64(parts[4])
			metrics.TuplesUpdated = safeAgentDBInt64(parts[5])
			metrics.TuplesDeleted = safeAgentDBInt64(parts[6])
			metrics.TempFiles = safeAgentDBInt64(parts[7])
			metrics.TempBytes = safeAgentDBInt64(parts[8])
			metrics.Deadlocks = safeAgentDBInt64(parts[9])
			metrics.CacheHitRatio = safeAgentDBFloat(parts[10])
			sources["pg_stat_database"] = "ok"
		}
	} else if err != nil {
		sources["pg_stat_database"] = databasePostgresSourceStatus(err)
	}
	locks, err := runner(postgresLocksQuery)
	if err == nil && len(locks) == 1 {
		parts := strings.Split(locks[0], "|")
		if len(parts) == 3 {
			metrics.LockWaitCount = safeAgentDBInt(parts[0])
			metrics.BlockedSessions = safeAgentDBInt(parts[1])
			metrics.BlockingSessions = safeAgentDBInt(parts[2])
			caps.LockDiagnostics = true
			sources["pg_locks"] = "ok"
		}
	} else if err != nil {
		sources["pg_locks"] = databasePostgresSourceStatus(err)
	}
	groups, groupsOK := collectPostgresDatabaseGroups(runner, state)
	caps.QueryGroups = groupsOK
	caps.PGStatStatements = groupsOK
	if groupsOK {
		sources["pg_stat_statements"] = "ok"
	}
	wal, err := runner(postgresWALQuery)
	if err == nil && len(wal) == 1 {
		metrics.WALBytes = safeAgentDBInt64(wal[0])
		sources["pg_stat_wal"] = "ok"
	} else if err != nil {
		sources["pg_stat_wal"] = databasePostgresSourceStatus(err)
	}
	status := "ok"
	if !caps.StatusMetrics {
		status = "unavailable"
	} else if !caps.LockDiagnostics || !caps.QueryGroups || sources["pg_stat_database"] != "ok" {
		status = "partial"
	}
	if versionErr != nil {
		sources["connection"] = "unavailable"
		status = "unavailable"
	}
	if logger != nil {
		logger.Printf("PostgreSQL diagnostics: connection=%s status=%s active_queries=%d longest_query_ms=%d pg_stat_statements=%s", connection, status, metrics.ActiveQueries, metrics.LongestQueryMS, sources["pg_stat_statements"])
	}
	return &agentDatabaseDiagnosticSample{CapturedAt: time.Now().UTC(), BucketSeconds: databaseSampleBucketSeconds(kind), SampleKind: kind, DatabaseType: "postgresql", DatabaseVersion: version, Instance: "default", Endpoint: "local socket", ConnectionType: connection, Status: status, Metrics: metrics, Capabilities: caps, SourceStatuses: sources, QueryGroups: groups}, nil
}

func databasePostgresSourceStatus(err error) string {
	if err == nil {
		return "ok"
	}
	switch strings.TrimSpace(err.Error()) {
	case "permission_denied":
		return "restricted"
	case "unsupported":
		return "unsupported"
	default:
		return "unavailable"
	}
}

func collectPostgresDatabaseGroups(run postgresQueryRunner, state *databaseDiagnosticsState) ([]agentDatabaseDiagnosticQueryGroup, bool) {
	if run == nil || state == nil {
		return nil, false
	}
	lines, err := run(postgresStatementsQueryWithMax)
	hasMaximum := err == nil
	if err != nil {
		lines, err = run(postgresStatementsQuery)
	}
	if err != nil {
		return nil, false
	}
	if state.Digests == nil {
		state.Digests = map[string]databaseDigestState{}
	}
	out := []agentDatabaseDiagnosticQueryGroup{}
	for _, line := range lines {
		parts := strings.Split(line, "|")
		expectedParts := 8
		if hasMaximum {
			expectedParts = 9
		}
		if len(parts) != expectedParts {
			continue
		}
		hash := normalizeAgentDatabaseIdentifier(parts[1]+":"+parts[0], 1, 128)
		if hash == "" {
			continue
		}
		rowsIndex, readIndex, hitIndex := 5, 6, 7
		maximum := int64(0)
		if hasMaximum {
			maximum = safeAgentDBInt64(parts[5])
			rowsIndex, readIndex, hitIndex = 6, 7, 8
		}
		next := databaseDigestState{Executions: safeAgentDBUint(parts[2]), TotalTime: safeAgentDBUint(parts[3]), RowsReturned: safeAgentDBUint(parts[rowsIndex]), SharedBlocksRead: safeAgentDBUint(parts[readIndex]), SharedBlocksHit: safeAgentDBUint(parts[hitIndex])}
		previous, exists := state.Digests[hash]
		state.Digests[hash] = next
		if !exists || databaseDigestReset(previous, next) {
			continue
		}
		delta := databaseDigestDelta(previous, next)
		if delta.Executions == 0 {
			continue
		}
		out = append(out, agentDatabaseDiagnosticQueryGroup{Hash: hash, Operation: "other", Database: normalizeAgentDatabaseIdentifier(parts[1], 1, 96), Executions: int64(delta.Executions), TotalTimeMS: int64(delta.TotalTime), AvgTimeMS: safeAgentDBInt64(parts[4]), MaxTimeMS: maximum, RowsReturned: int64(delta.RowsReturned), SharedBlocksRead: int64(delta.SharedBlocksRead), SharedBlocksHit: int64(delta.SharedBlocksHit)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TotalTimeMS > out[j].TotalTimeMS })
	if len(out) > databaseDiagnosticsMaxQueryGroups {
		out = out[:databaseDiagnosticsMaxQueryGroups]
	}
	trimDatabaseDigestState(state.Digests)
	return out, true
}

func safeAgentDBInt(value string) int {
	parsed, _ := strconv.Atoi(strings.TrimSpace(value))
	if parsed < 0 {
		return 0
	}
	return parsed
}
func safeAgentDBInt64(value string) int64 {
	parsed, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if parsed < 0 {
		return 0
	}
	return parsed
}
func safeAgentDBUint(value string) uint64 {
	parsed, _ := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if parsed < 0 {
		return 0
	}
	return uint64(parsed)
}
func safeAgentDBFloat(value string) float64 {
	parsed, _ := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if parsed < 0 {
		return 0
	}
	if parsed > 100 {
		return 100
	}
	return parsed
}

func databaseDiagnosticTriggered(metrics agentDatabaseDiagnosticMetrics, thresholds databaseDiagnosticThresholds, databaseType string) (bool, string) {
	connections := metrics.Connections
	if databaseType != "postgresql" {
		connections = metrics.ThreadsConnected
	}
	switch {
	case metrics.Deadlocks >= int64(thresholds.Deadlocks) && thresholds.Deadlocks > 0:
		return true, "deadlock"
	case metrics.LockWaitCount >= thresholds.Locks && thresholds.Locks > 0:
		return true, "lock_wait"
	case metrics.LongestQueryMS >= thresholds.LongestQueryMS && thresholds.LongestQueryMS > 0:
		return true, "long_query"
	case metrics.ActiveQueries >= thresholds.ActiveQueries && thresholds.ActiveQueries > 0:
		return true, "query_spike"
	case metrics.WaitingQueries >= thresholds.Waits && thresholds.Waits > 0:
		return true, "wait_spike"
	case metrics.IdleInTransaction >= thresholds.IdleInTransaction && thresholds.IdleInTransaction > 0:
		return true, "idle_transaction"
	case connections >= thresholds.Connections && thresholds.Connections > 0:
		return true, "connection_spike"
	}
	return false, ""
}

func databaseIncidentMetrics(current agentDatabaseDiagnosticMetrics, state *databaseDiagnosticsState) agentDatabaseDiagnosticMetrics {
	if state == nil {
		return current
	}
	previous := state.PostgresTotals
	copyOfCurrent := current
	state.PostgresTotals = &copyOfCurrent
	if previous == nil {
		current.Deadlocks = 0
		current.Conflicts = 0
		current.Commits = 0
		current.Rollbacks = 0
		current.TuplesReturned = 0
		current.TuplesFetched = 0
		current.TuplesInserted = 0
		current.TuplesUpdated = 0
		current.TuplesDeleted = 0
		current.TempFiles = 0
		current.TempBytes = 0
		current.WALBytes = 0
		return current
	}
	current.Deadlocks = safeAgentCounterDelta(previous.Deadlocks, current.Deadlocks)
	current.Conflicts = safeAgentCounterDelta(previous.Conflicts, current.Conflicts)
	current.Commits = safeAgentCounterDelta(previous.Commits, current.Commits)
	current.Rollbacks = safeAgentCounterDelta(previous.Rollbacks, current.Rollbacks)
	current.TuplesReturned = safeAgentCounterDelta(previous.TuplesReturned, current.TuplesReturned)
	current.TuplesFetched = safeAgentCounterDelta(previous.TuplesFetched, current.TuplesFetched)
	current.TuplesInserted = safeAgentCounterDelta(previous.TuplesInserted, current.TuplesInserted)
	current.TuplesUpdated = safeAgentCounterDelta(previous.TuplesUpdated, current.TuplesUpdated)
	current.TuplesDeleted = safeAgentCounterDelta(previous.TuplesDeleted, current.TuplesDeleted)
	current.TempFiles = safeAgentCounterDelta(previous.TempFiles, current.TempFiles)
	current.TempBytes = safeAgentCounterDelta(previous.TempBytes, current.TempBytes)
	current.WALBytes = safeAgentCounterDelta(previous.WALBytes, current.WALBytes)
	return current
}

func safeAgentCounterDelta(previous, current int64) int64 {
	if current < previous {
		return 0
	}
	return current - previous
}
func databaseIncidentSeverity(metrics agentDatabaseDiagnosticMetrics, thresholds databaseDiagnosticThresholds) string {
	if metrics.LongestQueryMS >= thresholds.LongestQueryMS*3 || metrics.ActiveQueries >= thresholds.ActiveQueries*3 || metrics.Deadlocks > 0 {
		return "critical"
	}
	return "warning"
}
func mergeDatabaseMetricPeaks(target *agentDatabaseDiagnosticMetrics, item agentDatabaseDiagnosticMetrics) {
	if target == nil {
		return
	}
	if item.Connections > target.Connections {
		target.Connections = item.Connections
	}
	if item.ThreadsConnected > target.ThreadsConnected {
		target.ThreadsConnected = item.ThreadsConnected
	}
	if item.ThreadsRunning > target.ThreadsRunning {
		target.ThreadsRunning = item.ThreadsRunning
	}
	if item.ActiveQueries > target.ActiveQueries {
		target.ActiveQueries = item.ActiveQueries
	}
	if item.IdleInTransaction > target.IdleInTransaction {
		target.IdleInTransaction = item.IdleInTransaction
	}
	if item.LongestQueryMS > target.LongestQueryMS {
		target.LongestQueryMS = item.LongestQueryMS
	}
	if item.WaitingQueries > target.WaitingQueries {
		target.WaitingQueries = item.WaitingQueries
	}
	if item.LockWaitCount > target.LockWaitCount {
		target.LockWaitCount = item.LockWaitCount
	}
	if item.Deadlocks > target.Deadlocks {
		target.Deadlocks = item.Deadlocks
	}
}

func newDatabaseSampleEvent(sample *agentDatabaseDiagnosticSample) agentDatabaseDiagnosticEvent {
	id := newDatabaseEventID("dbm")
	sample.EventID = id
	return agentDatabaseDiagnosticEvent{SchemaVersion: databaseDiagnosticsSchemaVersion, EventID: id, Kind: "sample", Sample: sample}
}
func newDatabaseIncidentEvent(incident *agentDatabaseDiagnosticIncident) agentDatabaseDiagnosticEvent {
	if incident.EventID == "" {
		incident.EventID = newDatabaseEventID("dbx")
	}
	return agentDatabaseDiagnosticEvent{SchemaVersion: databaseDiagnosticsSchemaVersion, EventID: incident.EventID, Kind: "incident", Incident: incident}
}
func newDatabaseEventID(prefix string) string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return prefix + "_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return prefix + "_" + hex.EncodeToString(raw[:])
}

func databaseDiagnosticsStatePath() string {
	return filepath.Join(defaultDatabaseDiagnosticsStoragePath(), "state.json")
}
func databaseDiagnosticsQueuePath(at time.Time) string {
	return filepath.Join(defaultDatabaseDiagnosticsStoragePath(), "minute-"+at.UTC().Format("2006010215")+".jsonl")
}
func loadDatabaseDiagnosticsState() (*databaseDiagnosticsState, error) {
	body, err := readDatabaseDiagnosticsFile(databaseDiagnosticsStatePath(), databaseDiagnosticsMaxQueueLineBytes)
	if errors.Is(err, os.ErrNotExist) {
		return &databaseDiagnosticsState{Digests: map[string]databaseDigestState{}, GlobalCounters: map[string]uint64{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var state databaseDiagnosticsState
	if err := json.Unmarshal(body, &state); err != nil {
		return nil, err
	}
	if state.Digests == nil {
		state.Digests = map[string]databaseDigestState{}
	}
	if state.GlobalCounters == nil {
		state.GlobalCounters = map[string]uint64{}
	}
	return &state, nil
}

func localDiagnosticsConfigExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil || !errors.Is(err, os.ErrNotExist)
}
func saveDatabaseDiagnosticsState(state *databaseDiagnosticsState) error {
	body, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return writeDatabaseDiagnosticsFile(databaseDiagnosticsStatePath(), body)
}

func appendDatabaseDiagnosticsQueue(record databaseDiagnosticQueueRecord, at time.Time) error {
	path := databaseDiagnosticsQueuePath(at)
	records, _ := readDatabaseDiagnosticsQueue(path)
	for index, existing := range records {
		if existing.AgentID == record.AgentID && existing.Event.EventID == record.Event.EventID {
			records[index] = record
			return writeDatabaseDiagnosticsQueue(path, records)
		}
	}
	body, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if len(body) > databaseDiagnosticsMaxQueueLineBytes {
		return errors.New("database diagnostics queue record exceeds limit")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(append(body, '\n'))
	return err
}
func readDatabaseDiagnosticsQueue(path string) ([]databaseDiagnosticQueueRecord, error) {
	body, err := readDatabaseDiagnosticsFile(path, databaseDiagnosticsMaxStorageBytes)
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64<<10), databaseDiagnosticsMaxQueueLineBytes)
	out := []databaseDiagnosticQueueRecord{}
	for scanner.Scan() {
		var item databaseDiagnosticQueueRecord
		if json.Unmarshal(scanner.Bytes(), &item) == nil && item.AgentID != "" && item.Event.EventID != "" {
			out = append(out, item)
		}
	}
	return out, scanner.Err()
}
func writeDatabaseDiagnosticsQueue(path string, records []databaseDiagnosticQueueRecord) error {
	if len(records) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	var body bytes.Buffer
	for _, item := range records {
		line, err := json.Marshal(item)
		if err != nil {
			return err
		}
		body.Write(line)
		body.WriteByte('\n')
	}
	return writeDatabaseDiagnosticsFile(path, body.Bytes())
}
func listDatabaseDiagnosticsQueue() ([]string, error) {
	entries, err := os.ReadDir(defaultDatabaseDiagnosticsStoragePath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "minute-") || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(defaultDatabaseDiagnosticsStoragePath(), entry.Name())
		info, err := os.Lstat(path)
		if err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out, nil
}
func pendingDatabaseDiagnosticsEvents(agentID string) ([]agentDatabaseDiagnosticEvent, error) {
	files, err := listDatabaseDiagnosticsQueue()
	if err != nil {
		return nil, err
	}
	out := []agentDatabaseDiagnosticEvent{}
	for _, path := range files {
		records, readErr := readDatabaseDiagnosticsQueue(path)
		if readErr != nil {
			continue
		}
		for _, item := range records {
			if item.AgentID == agentID && len(out) < databaseDiagnosticsMaxQueueEvents {
				out = append(out, item.Event)
			}
		}
		if len(out) >= databaseDiagnosticsMaxQueueEvents {
			break
		}
	}
	return out, nil
}
func acknowledgeDatabaseDiagnosticsEvents(agentID string, ids []string) error {
	acked := map[string]struct{}{}
	for _, id := range ids {
		acked[id] = struct{}{}
	}
	files, err := listDatabaseDiagnosticsQueue()
	if err != nil {
		return err
	}
	for _, path := range files {
		records, readErr := readDatabaseDiagnosticsQueue(path)
		if readErr != nil {
			continue
		}
		remaining := records[:0]
		changed := false
		for _, item := range records {
			if item.AgentID == agentID {
				if _, ok := acked[item.Event.EventID]; ok {
					changed = true
					continue
				}
			}
			remaining = append(remaining, item)
		}
		if changed {
			if err := writeDatabaseDiagnosticsQueue(path, remaining); err != nil {
				return err
			}
		}
	}
	return nil
}

func readDatabaseDiagnosticsFile(path string, maxBytes int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o600 || info.Size() > maxBytes {
		return nil, errors.New("database diagnostics file is unsafe")
	}
	if runtime.GOOS == "linux" && !bitrix24MySQLDefaultsRootOwned(info) {
		return nil, errors.New("database diagnostics file is not owned by root")
	}
	return os.ReadFile(path)
}
func writeDatabaseDiagnosticsFile(path string, body []byte) error {
	if int64(len(body)) > databaseDiagnosticsMaxStorageBytes {
		return errors.New("database diagnostics file exceeds storage limit")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
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
func trimDatabaseDiagnosticsStorage(now time.Time) error {
	files, err := listDatabaseDiagnosticsQueue()
	if err != nil {
		return err
	}
	type entry struct {
		path string
		info os.FileInfo
	}
	items := []entry{}
	var total int64
	for _, path := range files {
		info, err := os.Lstat(path)
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > databaseDiagnosticsRetention {
			_ = os.Remove(path)
			continue
		}
		items = append(items, entry{path, info})
		total += info.Size()
	}
	sort.Slice(items, func(i, j int) bool { return items[i].info.ModTime().Before(items[j].info.ModTime()) })
	for _, item := range items {
		if total <= databaseDiagnosticsMaxStorageBytes {
			break
		}
		if err := os.Remove(item.path); err == nil {
			total -= item.info.Size()
		}
	}
	return nil
}

func flushDatabaseDiagnosticsEvents(serverURL string, state *agentState, logger *log.Logger) (bool, databaseDiagnosticThresholds) {
	events, err := pendingDatabaseDiagnosticsEvents(state.AgentID)
	if err != nil || len(events) == 0 {
		return true, databaseDiagnosticThresholds{}
	}
	endpoint, err := databaseDiagnosticsEndpoint(serverURL)
	if err != nil {
		return true, databaseDiagnosticThresholds{}
	}
	body, _ := json.Marshal(agentDatabaseDiagnosticEventBatch{Events: events})
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return true, databaseDiagnosticThresholds{}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+state.Token)
	resp, err := (&http.Client{Timeout: 12 * time.Second}).Do(req)
	if err != nil {
		if logger != nil {
			logger.Printf("database diagnostics delivery pending: %v", err)
		}
		return true, databaseDiagnosticThresholds{}
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return false, databaseDiagnosticThresholds{}
	}
	if resp.StatusCode >= http.StatusMultipleChoices {
		return true, databaseDiagnosticThresholds{}
	}
	var payload agentDatabaseDiagnosticEventBatchResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, databaseDiagnosticsMaxQueueLineBytes)).Decode(&payload); err != nil && !errors.Is(err, io.EOF) {
		return true, databaseDiagnosticThresholds{}
	}
	_ = acknowledgeDatabaseDiagnosticsEvents(state.AgentID, payload.EventAck)
	return payload.Enabled, payload.Thresholds
}
func databaseDiagnosticsEndpoint(serverURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(serverURL))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return "", errors.New("AGENT_SERVER is not configured for database diagnostics")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/api/agent/v1/database-diagnostics"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

const databaseDiagnosticsServiceUnit = `[Unit]
Description=Pinguva local database diagnostics
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
User=root
Group=root
EnvironmentFile=-/etc/pinguva-agent.env
ExecStart=/usr/bin/pinguva-agent database diagnostics --config-path /etc/pinguva-agent/database.json
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ReadWritePaths=/var/lib/pinguva-agent /etc/pinguva-agent
TimeoutStartSec=50s
`

const databaseDiagnosticsTimerUnit = `[Unit]
Description=Run Pinguva database diagnostics every minute

[Timer]
OnBootSec=90s
OnUnitActiveSec=60s
Persistent=true
Unit=pinguva-database-diagnostics.service

[Install]
WantedBy=timers.target
`

func installDatabaseDiagnosticsTimer() error {
	for path, body := range map[string]string{"/etc/systemd/system/pinguva-database-diagnostics.service": databaseDiagnosticsServiceUnit, "/etc/systemd/system/pinguva-database-diagnostics.timer": databaseDiagnosticsTimerUnit} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return err
		}
	}
	for _, args := range [][]string{{"daemon-reload"}, {"enable", "--now", "pinguva-database-diagnostics.timer"}, {"start", "pinguva-database-diagnostics.service"}} {
		output, err := exec.Command("systemctl", args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("systemctl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}
