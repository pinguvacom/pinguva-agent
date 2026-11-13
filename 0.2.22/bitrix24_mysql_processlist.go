package main

import "strings"

const bitrix24MySQLForeignSessionCountQuery = `
SELECT COUNT(*)
FROM information_schema.PROCESSLIST
WHERE USER NOT IN (
  SUBSTRING_INDEX(CURRENT_USER(), '@', 1),
  SUBSTRING_INDEX(USER(), '@', 1)
);`

// bitrix24MySQLProcesslistCollection separates privilege hints from the actual
// PROCESSLIST queries. Successful reads are authoritative; grant parsing never
// prevents collection of safe aggregates.
type bitrix24MySQLProcesslistCollection struct {
	Status               string
	Visibility           string
	PrivilegeDetected    bool
	PrivilegeSource      string
	Error                string
	QueryGroupsStatus    string
	QueryGroupsError     string
	ActiveQueries        int
	LongestQuerySec      int64
	TopQueries           []agentBitrix24QueryFingerprint
	GrantsError          string
	VisibilityCheckError string
}

func collectBitrix24MySQLProcesslist(run bitrix24MySQLQueryRunner) bitrix24MySQLProcesslistCollection {
	result := bitrix24MySQLProcesslistCollection{
		Status:            "unavailable",
		Visibility:        "unknown",
		PrivilegeSource:   "functional_check",
		QueryGroupsStatus: "unavailable",
	}

	grants, grantsErr := run("SHOW GRANTS FOR CURRENT_USER();")
	if grantsErr != nil {
		result.GrantsError = bitrix24MySQLDiagnosticErrorCode(grantsErr)
	} else if bitrix24MySQLHasProcessPrivilege(grants) {
		result.PrivilegeDetected = true
		result.PrivilegeSource = "show_grants"
		result.Visibility = "full"
	}

	foreignSessions, foreignKnown := 0, false
	foreignLines, foreignErr := run(bitrix24MySQLForeignSessionCountQuery)
	if foreignErr != nil {
		result.VisibilityCheckError = bitrix24MySQLDiagnosticErrorCode(foreignErr)
	} else if count, ok := parseBitrix24MySQLCount(foreignLines); ok {
		foreignSessions, foreignKnown = count, true
	} else {
		result.VisibilityCheckError = "invalid_result"
	}
	if !result.PrivilegeDetected && foreignKnown && foreignSessions > 0 {
		result.PrivilegeDetected = true
		result.PrivilegeSource = "foreign_sessions_visible"
		result.Visibility = "full"
	}

	// Do not infer a missing PROCESS privilege from grants or from an empty
	// process list. The summary query is the first authoritative read result.
	summaryLines, summaryErr := run(bitrix24MySQLProcesslistSummaryQuery)
	if summaryErr != nil {
		result.Error = bitrix24MySQLDiagnosticErrorCode(summaryErr)
		if result.Error == "permission_denied" {
			result.Status = "restricted"
		}
		return result
	}
	active, longest, ok := parseBitrix24MySQLProcesslistSummary(summaryLines)
	if !ok {
		result.Error = "invalid_result"
		return result
	}
	result.Status = "ok"
	result.ActiveQueries = active
	result.LongestQuerySec = longest

	// An empty result is a valid empty aggregate and remains query_groups=ok.
	queryLines, queryErr := run(bitrix24MySQLProcesslistGroupsQuery)
	if queryErr != nil {
		result.QueryGroupsError = bitrix24MySQLDiagnosticErrorCode(queryErr)
		return result
	}
	result.QueryGroupsStatus = "ok"
	result.TopQueries = parseBitrix24QueryFingerprints(queryLines)
	if result.TopQueries == nil {
		result.TopQueries = []agentBitrix24QueryFingerprint{}
	}
	return result
}

func parseBitrix24MySQLGlobalPrivileges(grants []string) map[string]struct{} {
	privileges := make(map[string]struct{})
	for _, grant := range grants {
		normalized := strings.Join(strings.Fields(strings.ToUpper(grant)), " ")
		if !strings.HasPrefix(normalized, "GRANT ") {
			continue
		}
		onIndex := strings.Index(normalized, " ON ")
		if onIndex <= len("GRANT ") {
			continue
		}
		tail := strings.Fields(normalized[onIndex+len(" ON "):])
		if len(tail) == 0 || strings.ReplaceAll(tail[0], "`", "") != "*.*" {
			continue
		}
		for _, raw := range strings.Split(normalized[len("GRANT "):onIndex], ",") {
			privilege := strings.TrimSpace(raw)
			if privilege != "" {
				privileges[privilege] = struct{}{}
			}
		}
	}
	return privileges
}

func bitrix24MySQLHasProcessPrivilege(grants []string) bool {
	privileges := parseBitrix24MySQLGlobalPrivileges(grants)
	_, hasProcess := privileges["PROCESS"]
	if hasProcess {
		return true
	}
	_, hasAll := privileges["ALL PRIVILEGES"]
	return hasAll
}

func parseBitrix24MySQLCount(lines []string) (int, bool) {
	if len(lines) != 1 {
		return 0, false
	}
	return safeBitrix24Int(lines[0])
}

func bitrix24MySQLDiagnosticErrorCode(err error) string {
	switch bitrix24MySQLQueryErrorCode(err) {
	case "permission":
		return "permission_denied"
	case "connection", "timeout", "compatibility", "invalid_result", "client_not_found":
		return bitrix24MySQLQueryErrorCode(err)
	default:
		return "query_failed"
	}
}

func normalizeBitrix24MySQLProcesslistStatus(value string) string {
	switch strings.TrimSpace(value) {
	case "ok", "restricted", "unavailable":
		return strings.TrimSpace(value)
	default:
		return "unavailable"
	}
}

func normalizeBitrix24MySQLProcesslistVisibility(value string) string {
	switch strings.TrimSpace(value) {
	case "full", "unknown":
		return strings.TrimSpace(value)
	default:
		return "unknown"
	}
}

func normalizeBitrix24MySQLQueryGroupsStatus(value string) string {
	switch strings.TrimSpace(value) {
	case "ok", "unavailable":
		return strings.TrimSpace(value)
	default:
		return "unavailable"
	}
}

func normalizeBitrix24MySQLPrivilegeSource(value string) string {
	switch strings.TrimSpace(value) {
	case "show_grants", "foreign_sessions_visible", "functional_check":
		return strings.TrimSpace(value)
	default:
		return "functional_check"
	}
}

func normalizeBitrix24MySQLDiagnosticError(value string) string {
	switch strings.TrimSpace(value) {
	case "permission_denied", "connection", "timeout", "compatibility", "invalid_result", "query_failed", "client_not_found":
		return strings.TrimSpace(value)
	default:
		return ""
	}
}
