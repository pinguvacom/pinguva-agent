# Pinguva Agent 0.2.15 Changes

- Preserve configured HTTP methods when collecting Business API routes.
- Support `ANY` route watches for URLs without an explicit method.
- Read explicitly configured application access-log paths that pass the agent safety filter.
- Send only aggregated route counters; raw log lines and request data remain local.
