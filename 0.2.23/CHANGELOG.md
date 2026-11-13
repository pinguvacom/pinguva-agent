# Pinguva Agent 0.2.23 Changes

- Add per-minute counts by HTTP error code for selected Business API routes.
- Send only the numeric HTTP code and aggregate count. The agent does not send individual requests, URL parameters, raw log lines, headers, or bodies.
- Keep the existing route counters and log parsers compatible with older server versions.
