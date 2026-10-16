# Pinguva Agent 0.2.19 Changes

- Detect JSONL, common Nginx/Apache access logs, and multiline PHP/cURL application logs locally.
- Parse only timestamp, route, HTTP status, and duration from PHP/cURL log blocks.
- Match method-less application log records to the configured route and its explicit log path.
- Mark a readable non-empty log with no supported records as unavailable instead of reporting false zero traffic.
- Keep raw log lines, response bodies, headers, tokens, and query values local to the server.
