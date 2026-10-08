package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNormalizeBitrix24DigestSQLRedactsAndRejectsUnsafeText(t *testing.T) {
	query := "SELECT * FROM b_crm_contact WHERE UF_CRM_1747304007 = '+7 777 123 45 67' AND ID = 42"
	normalized := normalizeBitrix24DigestSQL(query)
	if normalized == "" || strings.Contains(normalized, "+7") || strings.Contains(normalized, "42") {
		t.Fatalf("expected redacted SELECT, got %q", normalized)
	}
	for _, unsafe := range []string{
		"INSERT INTO b_crm_contact VALUES (1)",
		"SELECT * FROM b_user WHERE email = 'person@example.com'; DROP TABLE b_user",
		"SELECT * FROM b_user WHERE token=@secret",
	} {
		if got := normalizeBitrix24DigestSQL(unsafe); got != "" {
			t.Fatalf("unsafe SQL must be discarded, got %q for %q", got, unsafe)
		}
	}
}

func TestBitrix24LoadDiagnosticsEndpointOnlyAcceptsHTTPBaseURL(t *testing.T) {
	for _, raw := range []string{"", "ftp://monitor.example.test", "https:///missing-host"} {
		if _, err := bitrix24LoadDiagnosticsEndpoint(raw); err == nil {
			t.Fatalf("endpoint %q must be rejected", raw)
		}
	}
	got, err := bitrix24LoadDiagnosticsEndpoint("https://monitor.example.test/base/")
	if err != nil || got != "https://monitor.example.test/base/api/agent/v1/bitrix24-diagnostics" {
		t.Fatalf("unexpected endpoint %q err=%v", got, err)
	}
}

func TestPostBitrix24LoadEventsUsesBearerAndAcknowledgesOnlyServerIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agent/v1/bitrix24-diagnostics" || r.Header.Get("Authorization") != "Bearer agent-token" {
			t.Fatalf("unexpected request %s auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		var payload agentBitrix24LoadEventBatch
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(payload.Events) != 1 || payload.Events[0].EventID != "b24m_1234567890abcdef" {
			t.Fatalf("unexpected event payload: %+v", payload)
		}
		_ = json.NewEncoder(w).Encode(agentBitrix24LoadEventBatchResponse{EventAck: []string{"b24m_1234567890abcdef"}})
	}))
	defer server.Close()
	endpoint, err := bitrix24LoadDiagnosticsEndpoint(server.URL)
	if err != nil {
		t.Fatalf("build endpoint: %v", err)
	}
	ack, err := postBitrix24LoadEvents(server.Client(), endpoint, "agent-token", []agentBitrix24LoadEvent{{EventID: "b24m_1234567890abcdef"}})
	if err != nil || len(ack) != 1 || ack[0] != "b24m_1234567890abcdef" {
		t.Fatalf("unexpected acknowledgement: %#v err=%v", ack, err)
	}
}

func TestPostBitrix24LoadEventsSilentlyHandlesOlderBackend(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusMethodNotAllowed, http.StatusNotFound} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		}))
		endpoint, err := bitrix24LoadDiagnosticsEndpoint(server.URL)
		if err != nil {
			server.Close()
			t.Fatalf("build endpoint: %v", err)
		}
		_, err = postBitrix24LoadEvents(server.Client(), endpoint, "agent-token", []agentBitrix24LoadEvent{{EventID: "b24m_1234567890abcdef"}})
		server.Close()
		if !errors.Is(err, errBitrix24LoadEndpointUnavailable) {
			t.Fatalf("old backend status %d error = %v, want compatibility error", status, err)
		}
	}
}

func TestBitrix24DigestDeltaNeverGoesNegative(t *testing.T) {
	previous := bitrix24DigestCounters{Executions: 10, TotalTimePS: 10_000_000_000, RowsExamined: 100, RowsSent: 10, Errors: 1, NoIndexUsed: 1}
	current := bitrix24DigestCounters{Executions: 12, TotalTimePS: 14_000_000_000, RowsExamined: 140, RowsSent: 12, Errors: 1, NoIndexUsed: 2}
	if bitrix24DigestCounterReset(previous, current) {
		t.Fatal("increasing counters must not reset the baseline")
	}
	delta := bitrix24DigestCounterDelta(previous, current)
	if delta.Executions != 2 || delta.TotalTimePS != 4_000_000_000 || delta.RowsExamined != 40 || delta.NoIndexUsed != 1 {
		t.Fatalf("unexpected delta: %+v", delta)
	}
	if !bitrix24DigestCounterReset(current, previous) {
		t.Fatal("decreasing counters must reset the baseline instead of producing negative values")
	}
}

func TestCollectBitrix24DigestSnapshotUsesOnlySupportedColumns(t *testing.T) {
	runner := func(query string) ([]string, error) {
		if strings.Contains(query, "information_schema.COLUMNS") {
			return []string{"SCHEMA_NAME", "DIGEST", "DIGEST_TEXT", "COUNT_STAR", "SUM_TIMER_WAIT", "MAX_TIMER_WAIT", "SUM_ROWS_EXAMINED", "SUM_ROWS_SENT", "SUM_ERRORS", "SUM_NO_INDEX_USED", "FIRST_SEEN", "LAST_SEEN"}, nil
		}
		if strings.Contains(query, "events_statements_summary_by_digest") {
			return []string{"bitrix\tabcdef1234567890\tSELECT * FROM b_crm_contact WHERE ID = 42\t4\t4000000000\t2000000000\t120\t4\t0\t1\t2026-06-23 10:00:00\t2026-06-23 10:01:00"}, nil
		}
		t.Fatalf("unexpected query: %s", query)
		return nil, nil
	}
	items, ok := collectBitrix24DigestSnapshot(runner)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one supported digest, ok=%t items=%+v", ok, items)
	}
	if items[0].NormalizedSQL == "" || strings.Contains(items[0].NormalizedSQL, "42") || items[0].Counters.Executions != 4 {
		t.Fatalf("unexpected safe digest snapshot: %+v", items[0])
	}
}

func TestBitrix24LoadIncidentLifecycle(t *testing.T) {
	state := &bitrix24LoadLocalState{Digests: map[string]bitrix24DigestCounters{}}
	start := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	bucket := agentBitrix24DiagnosticBucket{
		BucketStart: start,
		REST:        agentBitrix24DiagnosticRESTBucket{Status: "ok", RequestCount: 74},
		MySQL:       agentBitrix24DiagnosticMySQLBucket{Status: "ok", LongestQueryMS: 2_100, ThreadsRunningMax: 4, ActiveQueriesMax: 2},
	}
	active := updateBitrix24LoadIncident(state, bucket, start.Add(time.Minute))
	if active == nil || active.Status != "active" || active.IncidentType != "long_sql_query" {
		t.Fatalf("expected an active long SQL incident, got %+v", active)
	}
	quiet := bucket
	quiet.BucketStart = start.Add(time.Minute)
	quiet.MySQL.LongestQueryMS = 0
	quiet.MySQL.ThreadsRunningMax = 1
	quiet.MySQL.ActiveQueriesMax = 0
	resolved := updateBitrix24LoadIncident(state, quiet, start.Add(2*time.Minute))
	if resolved == nil || resolved.Status != "resolved" || resolved.DurationMS <= 0 || state.ActiveIncident != nil {
		t.Fatalf("expected incident recovery, got incident=%+v state=%+v", resolved, state.ActiveIncident)
	}
}

func TestBitrix24LoadQueueAcknowledgementAndRetention(t *testing.T) {
	originalRoot := bitrix24LoadDiagnosticsRoot
	root := t.TempDir()
	bitrix24LoadDiagnosticsRoot = func() string { return root }
	t.Cleanup(func() { bitrix24LoadDiagnosticsRoot = originalRoot })

	now := time.Now().UTC().Truncate(time.Minute)
	first := bitrix24LoadQueueRecord{AgentID: "agt_test", Event: agentBitrix24LoadEvent{SchemaVersion: 1, EventID: "b24m_1234567890abcdef", Kind: "minute_bucket", Bucket: &agentBitrix24DiagnosticBucket{BucketStart: now, BucketSeconds: 60}}}
	second := bitrix24LoadQueueRecord{AgentID: "agt_other", Event: agentBitrix24LoadEvent{SchemaVersion: 1, EventID: "b24m_fedcba0987654321", Kind: "minute_bucket", Bucket: &agentBitrix24DiagnosticBucket{BucketStart: now, BucketSeconds: 60}}}
	if err := appendBitrix24LoadQueueRecord(first, now); err != nil {
		t.Fatalf("append first record: %v", err)
	}
	if err := appendBitrix24LoadQueueRecord(first, now); err != nil {
		t.Fatalf("append duplicate record: %v", err)
	}
	if err := appendBitrix24LoadQueueRecord(second, now); err != nil {
		t.Fatalf("append second record: %v", err)
	}
	pending, err := pendingBitrix24LoadEvents("agt_test", 4)
	if err != nil || len(pending) != 1 || pending[0].EventID != first.Event.EventID {
		t.Fatalf("unexpected pending queue: events=%+v err=%v", pending, err)
	}
	if err := acknowledgeBitrix24LoadEvents("agt_test", []string{first.Event.EventID}); err != nil {
		t.Fatalf("acknowledge queue: %v", err)
	}
	pending, err = pendingBitrix24LoadEvents("agt_test", 4)
	if err != nil || len(pending) != 0 {
		t.Fatalf("expected acknowledged event to disappear: events=%+v err=%v", pending, err)
	}
}

func TestBitrix24LoadQueueReplacesIncidentWithLatestSafeState(t *testing.T) {
	originalRoot := bitrix24LoadDiagnosticsRoot
	root := t.TempDir()
	bitrix24LoadDiagnosticsRoot = func() string { return root }
	t.Cleanup(func() { bitrix24LoadDiagnosticsRoot = originalRoot })

	now := time.Now().UTC().Truncate(time.Minute)
	active := agentBitrix24LoadEvent{
		SchemaVersion: 1, EventID: "b24i_1234567890abcdef", Kind: "load_incident",
		Incident: &agentBitrix24LoadIncident{Status: "active", StartedAt: now, Samples: []agentBitrix24DiagnosticSample{{CapturedAt: now}}},
	}
	resolved := active
	resolved.Incident = &agentBitrix24LoadIncident{Status: "resolved", StartedAt: now, EndedAt: now.Add(time.Minute), DurationMS: 60_000, Samples: []agentBitrix24DiagnosticSample{{CapturedAt: now}, {CapturedAt: now.Add(10 * time.Second)}}}
	if err := appendBitrix24LoadQueueRecord(bitrix24LoadQueueRecord{AgentID: "agt_test", Event: active}, now); err != nil {
		t.Fatalf("append active incident: %v", err)
	}
	if err := appendBitrix24LoadQueueRecord(bitrix24LoadQueueRecord{AgentID: "agt_test", Event: resolved}, now); err != nil {
		t.Fatalf("replace incident: %v", err)
	}
	pending, err := pendingBitrix24LoadEvents("agt_test", 4)
	if err != nil || len(pending) != 1 || pending[0].Incident == nil || pending[0].Incident.Status != "resolved" || len(pending[0].Incident.Samples) != 2 {
		t.Fatalf("latest incident state was not retained: events=%+v err=%v", pending, err)
	}
}

func TestBitrix24LoadIncidentCooldownAvoidsAlertFlapping(t *testing.T) {
	start := time.Date(2026, 6, 24, 1, 0, 0, 0, time.UTC)
	state := &bitrix24LoadLocalState{Digests: map[string]bitrix24DigestCounters{}, LastIncidentResolvedAt: start}
	bucket := agentBitrix24DiagnosticBucket{BucketStart: start.Add(time.Minute), MySQL: agentBitrix24DiagnosticMySQLBucket{Status: "ok", LongestQueryMS: 2_100}}
	if incident := updateBitrix24LoadIncident(state, bucket, start.Add(time.Minute)); incident != nil {
		t.Fatalf("cooldown must suppress immediate duplicate incident, got %+v", incident)
	}
	if incident := updateBitrix24LoadIncident(state, bucket, start.Add(bitrix24IncidentCooldown+time.Minute)); incident == nil || incident.Status != "active" {
		t.Fatalf("incident should reopen after cooldown, got %+v", incident)
	}
}
