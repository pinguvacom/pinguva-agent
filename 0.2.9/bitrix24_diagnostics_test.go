package main

import (
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
	diagnostics, meta := collectBitrix24MySQLDiagnosticsWithRunner(bitrix24MySQLTestRunner(t, func(query string) ([]string, error) {
		switch {
		case strings.Contains(query, "performance_schema.global_status"):
			return []string{"2\t1"}, nil
		case query == "SHOW GRANTS;":
			return []string{"GRANT PROCESS ON *.* TO 'monitor'@'localhost'"}, nil
		case strings.Contains(query, "GROUP BY 1"):
			return nil, nil
		case strings.Contains(query, "information_schema.PROCESSLIST"):
			return []string{"0\t0"}, nil
		default:
			t.Fatalf("unexpected query: %s", query)
			return nil, nil
		}
	}))
	if diagnostics.Status != "ok" || diagnostics.ThreadsRunning != 2 || diagnostics.ThreadsConnected != 1 {
		t.Fatalf("unexpected diagnostics: %+v", diagnostics)
	}
	if meta.StatusSource != "performance_schema.global_status" || meta.FallbackUsed {
		t.Fatalf("unexpected metadata: %+v", meta)
	}
}

func TestBitrix24MySQLFallsBackToShowGlobalStatus(t *testing.T) {
	diagnostics, meta := collectBitrix24MySQLDiagnosticsWithRunner(bitrix24MySQLTestRunner(t, func(query string) ([]string, error) {
		switch {
		case strings.Contains(query, "performance_schema.global_status"):
			return nil, &bitrix24MySQLQueryError{Code: "compatibility"}
		case strings.Contains(query, "SHOW GLOBAL STATUS"):
			return []string{"Threads_connected\t4", "Threads_running\t1"}, nil
		case query == "SHOW GRANTS;":
			return []string{"GRANT PROCESS ON *.* TO 'monitor'@'localhost'"}, nil
		case strings.Contains(query, "GROUP BY 1"):
			return nil, nil
		case strings.Contains(query, "information_schema.PROCESSLIST"):
			return []string{"0\t0"}, nil
		default:
			t.Fatalf("unexpected query: %s", query)
			return nil, nil
		}
	}))
	if diagnostics.Status != "ok" || diagnostics.ThreadsRunning != 1 || diagnostics.ThreadsConnected != 4 {
		t.Fatalf("unexpected diagnostics: %+v", diagnostics)
	}
	if meta.StatusSource != "show_global_status" || !meta.FallbackUsed {
		t.Fatalf("expected SHOW GLOBAL STATUS fallback, got %+v", meta)
	}
}

func TestBitrix24MySQLFallsBackToInformationSchema(t *testing.T) {
	diagnostics, meta := collectBitrix24MySQLDiagnosticsWithRunner(bitrix24MySQLTestRunner(t, func(query string) ([]string, error) {
		switch {
		case strings.Contains(query, "performance_schema.global_status"), strings.Contains(query, "SHOW GLOBAL STATUS"):
			return nil, &bitrix24MySQLQueryError{Code: "compatibility"}
		case strings.Contains(query, "information_schema.GLOBAL_STATUS"):
			return []string{"3\t2"}, nil
		case query == "SHOW GRANTS;":
			return []string{"GRANT PROCESS ON *.* TO 'monitor'@'localhost'"}, nil
		case strings.Contains(query, "GROUP BY 1"):
			return nil, nil
		case strings.Contains(query, "information_schema.PROCESSLIST"):
			return []string{"0\t0"}, nil
		default:
			t.Fatalf("unexpected query: %s", query)
			return nil, nil
		}
	}))
	if diagnostics.Status != "ok" || diagnostics.ThreadsRunning != 3 || diagnostics.ThreadsConnected != 2 {
		t.Fatalf("unexpected diagnostics: %+v", diagnostics)
	}
	if meta.StatusSource != "information_schema.global_status" || !meta.FallbackUsed {
		t.Fatalf("expected information_schema fallback, got %+v", meta)
	}
}

func TestBitrix24MySQLTreatsEmptyProcesslistAsHealthy(t *testing.T) {
	diagnostics, meta := collectBitrix24MySQLDiagnosticsWithRunner(bitrix24MySQLHealthyRunner(t, "0\t0", nil))
	if diagnostics.Status != "ok" || diagnostics.ActiveQueries != 0 || diagnostics.LongestQuerySec != 0 {
		t.Fatalf("empty processlist must be healthy, got diagnostics=%+v meta=%+v", diagnostics, meta)
	}
}

func TestBitrix24MySQLRecordsLongRunningQuery(t *testing.T) {
	diagnostics, meta := collectBitrix24MySQLDiagnosticsWithRunner(bitrix24MySQLHealthyRunner(t, "1\t2", []string{"crm_contact_case_insensitive_lookup\t1\t2"}))
	if diagnostics.Status != "ok" || diagnostics.ActiveQueries != 1 || diagnostics.LongestQuerySec != 2 || len(diagnostics.TopQueries) != 1 {
		t.Fatalf("long-running query was not collected: diagnostics=%+v meta=%+v", diagnostics, meta)
	}
}

func TestBitrix24MySQLKeepsStatusMetricsWhenProcesslistIsRestricted(t *testing.T) {
	diagnostics, meta := collectBitrix24MySQLDiagnosticsWithRunner(bitrix24MySQLTestRunner(t, func(query string) ([]string, error) {
		switch {
		case strings.Contains(query, "performance_schema.global_status"):
			return []string{"2\t5"}, nil
		case query == "SHOW GRANTS;":
			return []string{"GRANT USAGE ON *.* TO 'monitor'@'localhost'"}, nil
		default:
			t.Fatalf("unexpected query after restricted grant: %s", query)
			return nil, nil
		}
	}))
	if diagnostics.Status != "partial" || diagnostics.ThreadsRunning != 2 || diagnostics.ThreadsConnected != 5 {
		t.Fatalf("status metrics must survive restricted processlist: %+v", diagnostics)
	}
	if meta.ProcesslistState != "restricted" || meta.ProcesslistError != "process_privilege_missing" {
		t.Fatalf("expected restricted processlist metadata, got %+v", meta)
	}
}

func TestBitrix24MySQLKeepsProcesslistWhenGlobalStatusIsUnavailable(t *testing.T) {
	diagnostics, meta := collectBitrix24MySQLDiagnosticsWithRunner(bitrix24MySQLTestRunner(t, func(query string) ([]string, error) {
		switch {
		case strings.Contains(strings.ToLower(query), "global_status"), strings.Contains(query, "SHOW GLOBAL STATUS"):
			return nil, &bitrix24MySQLQueryError{Code: "compatibility"}
		case query == "SHOW GRANTS;":
			return []string{"GRANT PROCESS ON *.* TO 'monitor'@'localhost'"}, nil
		case strings.Contains(query, "GROUP BY 1"):
			return nil, nil
		case strings.Contains(query, "information_schema.PROCESSLIST"):
			return []string{"1\t12"}, nil
		default:
			t.Fatalf("unexpected query: %s", query)
			return nil, nil
		}
	}))
	if diagnostics.Status != "partial" || diagnostics.ActiveQueries != 1 || diagnostics.LongestQuerySec != 12 {
		t.Fatalf("processlist metrics must survive global status failure: %+v", diagnostics)
	}
	if meta.StatusSource != "unavailable" || meta.ProcesslistState != "ok" {
		t.Fatalf("unexpected metadata: %+v", meta)
	}
}

func TestBitrix24MySQLReportsUnavailableWhenConnectionFails(t *testing.T) {
	diagnostics, meta := collectBitrix24MySQLDiagnosticsWithRunner(bitrix24MySQLTestRunner(t, func(string) ([]string, error) {
		return nil, &bitrix24MySQLQueryError{Code: "connection"}
	}))
	if diagnostics.Status != "unavailable" {
		t.Fatalf("expected unavailable diagnostics, got %+v", diagnostics)
	}
	if meta.StatusError != "connection" || meta.ProcesslistError != "connection" {
		t.Fatalf("unexpected connection metadata: %+v", meta)
	}
}

func TestNormalizeBitrix24MySQLDiagnosticsKeepsPartialMetrics(t *testing.T) {
	normalized := normalizeAgentBitrix24MySQLDiagnostics(&agentBitrix24MySQLDiagnostics{
		Status:           "partial",
		ThreadsRunning:   2,
		ThreadsConnected: 5,
	})
	if normalized == nil {
		t.Fatal("expected normalized diagnostics")
	}
	if normalized.Status != "partial" || normalized.ThreadsRunning != 2 || normalized.ThreadsConnected != 5 {
		t.Fatalf("partial MySQL diagnostics must retain usable thread metrics, got %+v", normalized)
	}
}

func bitrix24MySQLHealthyRunner(t *testing.T, summary string, groups []string) bitrix24MySQLQueryRunner {
	t.Helper()
	return bitrix24MySQLTestRunner(t, func(query string) ([]string, error) {
		switch {
		case strings.Contains(query, "performance_schema.global_status"):
			return []string{"2\t1"}, nil
		case query == "SHOW GRANTS;":
			return []string{"GRANT PROCESS ON *.* TO 'monitor'@'localhost'"}, nil
		case strings.Contains(query, "GROUP BY 1"):
			return groups, nil
		case strings.Contains(query, "information_schema.PROCESSLIST"):
			return []string{summary}, nil
		default:
			t.Fatalf("unexpected query: %s", query)
			return nil, nil
		}
	})
}

func bitrix24MySQLTestRunner(t *testing.T, run bitrix24MySQLQueryRunner) bitrix24MySQLQueryRunner {
	t.Helper()
	return func(query string) ([]string, error) {
		return run(query)
	}
}
