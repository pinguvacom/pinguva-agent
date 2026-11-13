package main

import "strings"

const bitrix24MySQLLockWaitsQuery = `
SELECT
  COUNT(*),
  COUNT(DISTINCT REQUESTING_ENGINE_TRANSACTION_ID),
  COUNT(DISTINCT BLOCKING_ENGINE_TRANSACTION_ID)
FROM performance_schema.data_lock_waits;`

type bitrix24MySQLLockDiagnostics struct {
	Status               string
	WaitCount            int
	BlockingTransactions int
	WaitingTransactions  int
}

// collectBitrix24MySQLLocks is optional by design. data_lock_waits is not
// present on every supported MySQL/MariaDB build, so an unsupported table does
// not downgrade otherwise healthy MySQL diagnostics.
func collectBitrix24MySQLLocks(run bitrix24MySQLQueryRunner) bitrix24MySQLLockDiagnostics {
	out := bitrix24MySQLLockDiagnostics{Status: "unsupported"}
	if run == nil {
		return out
	}
	lines, err := run(bitrix24MySQLLockWaitsQuery)
	if err != nil {
		if bitrix24MySQLDiagnosticErrorCode(err) == "permission_denied" {
			out.Status = "unavailable"
		}
		return out
	}
	if len(lines) != 1 {
		out.Status = "unavailable"
		return out
	}
	parts := strings.Split(lines[0], "\t")
	if len(parts) != 3 {
		out.Status = "unavailable"
		return out
	}
	waits, waitsOK := safeBitrix24Int(parts[0])
	waiting, waitingOK := safeBitrix24Int(parts[1])
	blocking, blockingOK := safeBitrix24Int(parts[2])
	if !waitsOK || !waitingOK || !blockingOK {
		out.Status = "unavailable"
		return out
	}
	out.Status = "ok"
	out.WaitCount = waits
	out.WaitingTransactions = waiting
	out.BlockingTransactions = blocking
	return out
}
