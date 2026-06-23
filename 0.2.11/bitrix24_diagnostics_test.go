package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBitrix24MySQLDefaultsOptionUsesOnlyStrictRootOnlyFile(t *testing.T) {
	originalRootOwned := bitrix24MySQLDefaultsRootOwned
	bitrix24MySQLDefaultsRootOwned = func(os.FileInfo) bool { return true }
	t.Cleanup(func() { bitrix24MySQLDefaultsRootOwned = originalRootOwned })

	path := filepath.Join(t.TempDir(), "my.cnf")
	if err := os.WriteFile(path, []byte("[client]\nuser=root\npassword=placeholder\n"), 0o600); err != nil {
		t.Fatalf("write defaults file: %v", err)
	}
	if got, want := bitrix24MySQLDefaultsOption(path), "--defaults-extra-file="+path; got != want {
		t.Fatalf("safe defaults option = %q, want %q", got, want)
	}
	for _, mode := range []os.FileMode{0o644, 0o660, 0o640, 0o700} {
		if err := os.Chmod(path, mode); err != nil {
			t.Fatalf("chmod defaults file %04o: %v", mode, err)
		}
		if got := bitrix24MySQLDefaultsOption(path); got != "" {
			t.Fatalf("defaults file mode %04o must be rejected, got %q", mode, got)
		}
	}
	link := filepath.Join(t.TempDir(), "my.cnf")
	if err := os.Symlink(path, link); err != nil {
		t.Fatalf("create defaults symlink: %v", err)
	}
	if got := bitrix24MySQLDefaultsOption(link); got != "" {
		t.Fatalf("defaults symlink must be rejected, got %q", got)
	}
}

func TestBitrix24MySQLDefaultsOptionRejectsNonRootOwner(t *testing.T) {
	originalRootOwned := bitrix24MySQLDefaultsRootOwned
	bitrix24MySQLDefaultsRootOwned = func(os.FileInfo) bool { return false }
	t.Cleanup(func() { bitrix24MySQLDefaultsRootOwned = originalRootOwned })

	path := filepath.Join(t.TempDir(), "my.cnf")
	if err := os.WriteFile(path, []byte("[client]\n"), 0o600); err != nil {
		t.Fatalf("write defaults file: %v", err)
	}
	if got := bitrix24MySQLDefaultsOption(path); got != "" {
		t.Fatalf("non-root-owned defaults file must be rejected, got %q", got)
	}
}

func TestBitrix24MySQLUsesPerformanceSchemaFirst(t *testing.T) {
	diagnostics, meta := collectBitrix24MySQLDiagnosticsWithRunner(newBitrix24MySQLScenarioRunner(t, bitrix24MySQLScenario{
		Performance: []string{"2\t1"},
		Grants:      []string{"GRANT PROCESS ON *.* TO 'monitor'@'localhost'"},
		Foreign:     []string{"0"},
		Summary:     []string{"0\t0"},
	}))
	if diagnostics.Status != "ok" || diagnostics.ThreadsRunning != 2 || diagnostics.ThreadsConnected != 1 {
		t.Fatalf("unexpected diagnostics: %+v", diagnostics)
	}
	if meta.StatusSource != "performance_schema.global_status" || meta.FallbackUsed {
		t.Fatalf("unexpected metadata: %+v", meta)
	}
}

func TestBitrix24MySQLFallsBackToShowGlobalStatus(t *testing.T) {
	diagnostics, meta := collectBitrix24MySQLDiagnosticsWithRunner(newBitrix24MySQLScenarioRunner(t, bitrix24MySQLScenario{
		PerformanceErr: mysqlTestError("compatibility"),
		Show:           []string{"Threads_connected\t4", "Threads_running\t1"},
		Grants:         []string{"GRANT PROCESS ON *.* TO 'monitor'@'localhost'"},
		Foreign:        []string{"0"},
		Summary:        []string{"0\t0"},
	}))
	if diagnostics.Status != "ok" || diagnostics.ThreadsRunning != 1 || diagnostics.ThreadsConnected != 4 {
		t.Fatalf("unexpected diagnostics: %+v", diagnostics)
	}
	if meta.StatusSource != "show_global_status" || !meta.FallbackUsed {
		t.Fatalf("expected SHOW GLOBAL STATUS fallback, got %+v", meta)
	}
}

func TestBitrix24MySQLFallsBackToInformationSchema(t *testing.T) {
	diagnostics, meta := collectBitrix24MySQLDiagnosticsWithRunner(newBitrix24MySQLScenarioRunner(t, bitrix24MySQLScenario{
		PerformanceErr: mysqlTestError("compatibility"),
		ShowErr:        mysqlTestError("compatibility"),
		Information:    []string{"3\t2"},
		Grants:         []string{"GRANT PROCESS ON *.* TO 'monitor'@'localhost'"},
		Foreign:        []string{"0"},
		Summary:        []string{"0\t0"},
	}))
	if diagnostics.Status != "ok" || diagnostics.ThreadsRunning != 3 || diagnostics.ThreadsConnected != 2 {
		t.Fatalf("unexpected diagnostics: %+v", diagnostics)
	}
	if meta.StatusSource != "information_schema.global_status" || !meta.FallbackUsed {
		t.Fatalf("expected information_schema fallback, got %+v", meta)
	}
}

func TestBitrix24MySQLGlobalPrivilegeParser(t *testing.T) {
	cases := []struct {
		name   string
		grants []string
		want   bool
	}{
		{"separate process", []string{"GRANT PROCESS ON *.* TO `monitor`@`localhost`"}, true},
		{"process in list", []string{"GRANT SELECT, INSERT, UPDATE, PROCESS, FILE ON *.* TO `root`@`localhost` WITH GRANT OPTION"}, true},
		{"no spaces after commas", []string{"GRANT SELECT,INSERT,UPDATE,PROCESS,FILE ON *.* TO `root`@`localhost`"}, true},
		{"mixed case", []string{"grant select, process, file on *.* to `root`@`localhost`"}, true},
		{"all privileges", []string{"GRANT ALL PRIVILEGES ON *.* TO `root`@`localhost`"}, true},
		{"missing process", []string{"GRANT SELECT, INSERT, UPDATE ON *.* TO `monitor`@`localhost`"}, false},
		{"dynamic privileges only", []string{"GRANT CONNECTION_ADMIN,SYSTEM_USER ON *.* TO `root`@`localhost`"}, false},
		{"database scoped all", []string{"GRANT ALL PRIVILEGES ON `bitrix`.* TO `root`@`localhost`"}, false},
		{"process in separate grant", []string{"GRANT CONNECTION_ADMIN ON *.* TO `root`@`localhost`", "GRANT PROCESS ON *.* TO `root`@`localhost`"}, true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := bitrix24MySQLHasProcessPrivilege(test.grants); got != test.want {
				t.Fatalf("has PROCESS = %t, want %t for %q", got, test.want, test.grants)
			}
		})
	}
}

func TestBitrix24MySQLTreatsEmptyProcesslistAsHealthy(t *testing.T) {
	diagnostics, _ := collectBitrix24MySQLDiagnosticsWithRunner(newBitrix24MySQLScenarioRunner(t, bitrix24MySQLScenario{
		Performance: []string{"2\t1"},
		Grants:      []string{"GRANT SELECT, PROCESS ON *.* TO 'monitor'@'localhost'"},
		Foreign:     []string{"0"},
		Summary:     []string{"0\t0"},
		Groups:      []string{},
	}))
	if diagnostics.Status != "ok" || diagnostics.ActiveQueries != 0 || diagnostics.LongestQuerySec != 0 || diagnostics.ProcesslistStatus != "ok" || diagnostics.QueryGroupsStatus != "ok" {
		t.Fatalf("empty processlist must be healthy, got %+v", diagnostics)
	}
	body, err := json.Marshal(diagnostics)
	if err != nil {
		t.Fatalf("marshal empty query groups: %v", err)
	}
	if !strings.Contains(string(body), `"topQueries":[]`) {
		t.Fatalf("empty query groups must be encoded as an array, got %s", body)
	}
}

func TestBitrix24MySQLRecordsLongRunningQuery(t *testing.T) {
	diagnostics, _ := collectBitrix24MySQLDiagnosticsWithRunner(newBitrix24MySQLScenarioRunner(t, bitrix24MySQLScenario{
		Performance: []string{"2\t1"},
		Grants:      []string{"GRANT PROCESS ON *.* TO 'monitor'@'localhost'"},
		Foreign:     []string{"1"},
		Summary:     []string{"1\t2"},
		Groups:      []string{"other_active_query\t1\t2"},
	}))
	if diagnostics.Status != "ok" || diagnostics.ActiveQueries != 1 || diagnostics.LongestQuerySec != 2 || len(diagnostics.TopQueries) != 1 || diagnostics.QueryGroupsStatus != "ok" {
		t.Fatalf("long-running query was not collected: %+v", diagnostics)
	}
}

func TestBitrix24MySQLUsesForeignSessionsAsFunctionalProof(t *testing.T) {
	diagnostics, _ := collectBitrix24MySQLDiagnosticsWithRunner(newBitrix24MySQLScenarioRunner(t, bitrix24MySQLScenario{
		Performance: []string{"2\t1"},
		Grants:      []string{"GRANT USAGE ON *.* TO 'monitor'@'localhost'"},
		Foreign:     []string{"2"},
		Summary:     []string{"0\t0"},
	}))
	if diagnostics.Status != "ok" || diagnostics.ProcesslistVisibility != "full" || !diagnostics.ProcessPrivilegeDetected || diagnostics.ProcessPrivilegeSource != "foreign_sessions_visible" {
		t.Fatalf("foreign sessions must prove visibility: %+v", diagnostics)
	}
}

func TestBitrix24MySQLDoesNotAssumeDeniedWhenVisibilityIsUnknown(t *testing.T) {
	diagnostics, _ := collectBitrix24MySQLDiagnosticsWithRunner(newBitrix24MySQLScenarioRunner(t, bitrix24MySQLScenario{
		Performance: []string{"2\t1"},
		Grants:      []string{"GRANT USAGE ON *.* TO 'monitor'@'localhost'"},
		Foreign:     []string{"0"},
		Summary:     []string{"0\t0"},
	}))
	if diagnostics.Status != "ok" || diagnostics.ProcesslistStatus != "ok" || diagnostics.ProcesslistVisibility != "unknown" || diagnostics.ProcessPrivilegeDetected || diagnostics.ProcesslistError != "" {
		t.Fatalf("unknown visibility without errors must remain healthy: %+v", diagnostics)
	}
}

func TestBitrix24MySQLContinuesWhenShowGrantsFails(t *testing.T) {
	diagnostics, meta := collectBitrix24MySQLDiagnosticsWithRunner(newBitrix24MySQLScenarioRunner(t, bitrix24MySQLScenario{
		Performance: []string{"2\t1"},
		GrantsErr:   mysqlTestError("permission"),
		Foreign:     []string{"0"},
		Summary:     []string{"0\t0"},
	}))
	if diagnostics.Status != "ok" || diagnostics.ProcesslistStatus != "ok" || diagnostics.ProcessPrivilegeSource != "functional_check" {
		t.Fatalf("SHOW GRANTS failure must not block processlist: diagnostics=%+v meta=%+v", diagnostics, meta)
	}
	if meta.GrantsError != "permission_denied" {
		t.Fatalf("expected safe grants error, got %+v", meta)
	}
}

func TestBitrix24MySQLMarksPartialOnlyForActualProcesslistError(t *testing.T) {
	diagnostics, _ := collectBitrix24MySQLDiagnosticsWithRunner(newBitrix24MySQLScenarioRunner(t, bitrix24MySQLScenario{
		Performance: []string{"2\t5"},
		Grants:      []string{"GRANT PROCESS ON *.* TO 'monitor'@'localhost'"},
		Foreign:     []string{"0"},
		SummaryErr:  mysqlTestError("permission"),
	}))
	if diagnostics.Status != "partial" || diagnostics.ProcesslistStatus != "restricted" || diagnostics.ProcesslistError != "permission_denied" {
		t.Fatalf("actual processlist permission error must be partial: %+v", diagnostics)
	}
}

func TestBitrix24MySQLKeepsProcesslistWhenGlobalStatusIsUnavailable(t *testing.T) {
	diagnostics, _ := collectBitrix24MySQLDiagnosticsWithRunner(newBitrix24MySQLScenarioRunner(t, bitrix24MySQLScenario{
		PerformanceErr: mysqlTestError("compatibility"),
		ShowErr:        mysqlTestError("compatibility"),
		InformationErr: mysqlTestError("compatibility"),
		Grants:         []string{"GRANT PROCESS ON *.* TO 'monitor'@'localhost'"},
		Foreign:        []string{"1"},
		Summary:        []string{"1\t12"},
	}))
	if diagnostics.Status != "partial" || diagnostics.ActiveQueries != 1 || diagnostics.LongestQuerySec != 12 || diagnostics.ProcesslistStatus != "ok" {
		t.Fatalf("processlist metrics must survive global status failure: %+v", diagnostics)
	}
}

func TestBitrix24MySQLReportsUnavailableWhenConnectionFails(t *testing.T) {
	diagnostics, meta := collectBitrix24MySQLDiagnosticsWithRunner(newBitrix24MySQLScenarioRunner(t, bitrix24MySQLScenario{
		PerformanceErr: mysqlTestError("connection"),
		GrantsErr:      mysqlTestError("connection"),
		ForeignErr:     mysqlTestError("connection"),
		SummaryErr:     mysqlTestError("connection"),
	}))
	if diagnostics.Status != "unavailable" || diagnostics.ProcesslistStatus != "unavailable" {
		t.Fatalf("expected unavailable diagnostics, got %+v", diagnostics)
	}
	if meta.StatusError != "connection" || diagnostics.ProcesslistError != "connection" {
		t.Fatalf("unexpected connection metadata: diagnostics=%+v meta=%+v", diagnostics, meta)
	}
}

func TestNormalizeBitrix24MySQLDiagnosticsKeepsSafeProcesslistFields(t *testing.T) {
	normalized := normalizeAgentBitrix24MySQLDiagnostics(&agentBitrix24MySQLDiagnostics{
		Status:                   "partial",
		ThreadsRunning:           2,
		ThreadsConnected:         5,
		ProcesslistStatus:        "restricted",
		ProcesslistVisibility:    "full",
		ProcessPrivilegeDetected: true,
		ProcessPrivilegeSource:   "show_grants",
		ProcesslistError:         "permission_denied",
		QueryGroupsStatus:        "ok",
	})
	if normalized == nil {
		t.Fatal("expected normalized diagnostics")
	}
	if normalized.Status != "partial" || normalized.ProcesslistStatus != "restricted" || !normalized.ProcessPrivilegeDetected || normalized.ProcessPrivilegeSource != "show_grants" || normalized.QueryGroupsStatus != "ok" {
		t.Fatalf("safe processlist fields were not retained: %+v", normalized)
	}
}

func TestBitrix24MySQLLogDoesNotExposeGrantsOrSQL(t *testing.T) {
	var buffer bytes.Buffer
	logger := log.New(&buffer, "", 0)
	logBitrix24MySQLDiagnostics(logger, &agentBitrix24MySQLDiagnostics{
		Status:                   "ok",
		ProcesslistStatus:        "ok",
		ProcesslistVisibility:    "full",
		ProcessPrivilegeDetected: true,
		ProcessPrivilegeSource:   "show_grants",
		QueryGroupsStatus:        "ok",
	}, bitrix24MySQLDiagnosticsMeta{})
	output := buffer.String()
	for _, forbidden := range []string{"GRANT SELECT", "SELECT SLEEP", "password="} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("unsafe log output contains %q: %s", forbidden, output)
		}
	}
}

type bitrix24MySQLScenario struct {
	Performance    []string
	PerformanceErr error
	Show           []string
	ShowErr        error
	Information    []string
	InformationErr error
	Grants         []string
	GrantsErr      error
	Foreign        []string
	ForeignErr     error
	Summary        []string
	SummaryErr     error
	Groups         []string
	GroupsErr      error
}

func newBitrix24MySQLScenarioRunner(t *testing.T, scenario bitrix24MySQLScenario) bitrix24MySQLQueryRunner {
	t.Helper()
	return func(query string) ([]string, error) {
		switch {
		case strings.Contains(query, "performance_schema.global_status"):
			return scenario.Performance, scenario.PerformanceErr
		case strings.Contains(query, "SHOW GLOBAL STATUS"):
			return scenario.Show, scenario.ShowErr
		case strings.Contains(query, "information_schema.GLOBAL_STATUS"):
			return scenario.Information, scenario.InformationErr
		case strings.Contains(query, "SHOW GRANTS FOR CURRENT_USER"):
			return scenario.Grants, scenario.GrantsErr
		case strings.Contains(query, "WHERE USER NOT IN"):
			return scenario.Foreign, scenario.ForeignErr
		case strings.Contains(query, "GROUP BY 1"):
			return scenario.Groups, scenario.GroupsErr
		case strings.Contains(query, "information_schema.PROCESSLIST"):
			return scenario.Summary, scenario.SummaryErr
		default:
			t.Fatalf("unexpected query: %s", query)
			return nil, errors.New("unexpected query")
		}
	}
}

func mysqlTestError(code string) error {
	return &bitrix24MySQLQueryError{Code: code}
}
