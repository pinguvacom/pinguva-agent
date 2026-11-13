# Pinguva Agent 0.2.21 Changes

- Parse consecutive pretty-printed JSON objects in local outbound API logs, in addition to existing JSONL, access logs, and PHP/cURL logs.
- Read `httpCode` and `totalTime` from application records; continue matching routes by the normalized `url` template and HTTP method.
- Permit explicitly configured local `request.json` and `sms.json` files under `/ilab/monitoring/` while rejecting unrelated JSON paths.
- Never include the actual `requestUrl`, raw log lines, or request parameters in diagnostic reports.
