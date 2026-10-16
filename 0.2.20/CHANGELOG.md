# Pinguva Agent 0.2.20 Changes

- Report source availability for each configured Business API route.
- Distinguish a missing log file, denied read access, and an unsupported non-empty log format.
- Continue collecting readable routes when another route's explicit log file is unavailable.
- Send only bounded route status and aggregate counters. Raw log lines and request or response bodies stay on the agent server.
