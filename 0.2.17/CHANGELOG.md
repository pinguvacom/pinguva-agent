# Pinguva Agent 0.2.17 Changes

- Parse access logs where the status appears before the quoted HTTP request.
- Read request duration from timestamp blocks such as `+0500 - 0.146`.
- Keep raw log lines and request data local; send aggregated counters only.
