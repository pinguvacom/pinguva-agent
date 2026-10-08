package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyMySQLGlobalCounterDeltas(t *testing.T) {
	state := &databaseDiagnosticsState{GlobalCounters: map[string]uint64{"created_tmp_disk_tables": 10, "select_scan": 20}}
	metrics := &agentDatabaseDiagnosticMetrics{}
	if !applyMySQLGlobalCounterDeltas(metrics, state, []string{"Created_tmp_disk_tables\t13", "Select_scan\t27"}) {
		t.Fatal("expected MySQL global counters to be parsed")
	}
	if metrics.TempFiles != 3 || metrics.FullScanCount != 7 {
		t.Fatalf("unexpected MySQL deltas: %+v", metrics)
	}
	reset := &agentDatabaseDiagnosticMetrics{}
	if !applyMySQLGlobalCounterDeltas(reset, state, []string{"Created_tmp_disk_tables\t1", "Select_scan\t2"}) || reset.TempFiles != 0 || reset.FullScanCount != 0 {
		t.Fatalf("counter reset must produce zero deltas: %+v", reset)
	}
}

func TestDatabaseIncidentMetricsUsesPostgresCounterDeltas(t *testing.T) {
	state := &databaseDiagnosticsState{}
	first := databaseIncidentMetrics(agentDatabaseDiagnosticMetrics{Connections: 4, Deadlocks: 10, Commits: 100}, state)
	if first.Connections != 4 || first.Deadlocks != 0 || first.Commits != 0 {
		t.Fatalf("first cumulative sample must establish baseline: %+v", first)
	}
	second := databaseIncidentMetrics(agentDatabaseDiagnosticMetrics{Connections: 5, Deadlocks: 11, Commits: 125}, state)
	if second.Connections != 5 || second.Deadlocks != 1 || second.Commits != 25 {
		t.Fatalf("unexpected cumulative delta: %+v", second)
	}
}

func TestDatabaseDiagnosticWaitThresholdTriggersIncident(t *testing.T) {
	triggered, incidentType := databaseDiagnosticTriggered(agentDatabaseDiagnosticMetrics{WaitingQueries: 6}, databaseDiagnosticThresholds{Waits: 5}, "postgresql")
	if !triggered || incidentType != "wait_spike" {
		t.Fatalf("wait threshold did not trigger: triggered=%t type=%q", triggered, incidentType)
	}
}

func TestPostgresGroupsFallbackKeepsDatabaseInHash(t *testing.T) {
	state := &databaseDiagnosticsState{Digests: map[string]databaseDigestState{}}
	calls := 0
	runner := func(query string) ([]string, error) {
		if query == postgresStatementsQueryWithMax {
			return nil, errors.New("unsupported")
		}
		calls++
		if calls == 1 {
			return []string{"-7|42|10|100|10|50|20|80"}, nil
		}
		return []string{"-7|42|12|140|12|60|25|95"}, nil
	}
	if groups, ok := collectPostgresDatabaseGroups(runner, state); !ok || len(groups) != 0 {
		t.Fatalf("first PostgreSQL digest snapshot must establish a baseline: ok=%t groups=%+v", ok, groups)
	}
	groups, ok := collectPostgresDatabaseGroups(runner, state)
	if !ok || len(groups) != 1 || groups[0].Hash != "42:-7" || groups[0].Executions != 2 || groups[0].MaxTimeMS != 0 {
		t.Fatalf("unexpected PostgreSQL digest delta: ok=%t groups=%+v", ok, groups)
	}
}

func TestDatabaseDiagnosticPayloadHasNoRawSQLOrSecrets(t *testing.T) {
	event := newDatabaseSampleEvent(&agentDatabaseDiagnosticSample{
		DatabaseType: "postgresql", Instance: "default", Status: "ok",
		QueryGroups: []agentDatabaseDiagnosticQueryGroup{{Hash: "query_1234", Operation: "select", Database: "app", Executions: 1}},
	})
	body, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	for _, forbidden := range []string{"rawSql", "normalizedSql", "queryText", "password", "webhook", "authorization"} {
		if strings.Contains(strings.ToLower(string(body)), strings.ToLower(forbidden)) {
			t.Fatalf("payload contains forbidden field %q: %s", forbidden, body)
		}
	}
}

func TestLocalDiagnosticsConfigExistsBlocksMalformedOrSymlinkConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "database.json")
	if localDiagnosticsConfigExists(path) {
		t.Fatal("missing config must not be reported as present")
	}
	if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if !localDiagnosticsConfigExists(path) {
		t.Fatal("malformed config must still block a conflicting integration")
	}
	link := filepath.Join(dir, "bitrix24.json")
	if err := os.Symlink(path, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	if !localDiagnosticsConfigExists(link) {
		t.Fatal("symlink config must still block a conflicting integration")
	}
}
