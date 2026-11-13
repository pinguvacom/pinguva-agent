# Pinguva Agent 0.2.22 Changes

- Discover named API routes from an explicitly configured, local JSON application log on a selected Linux agent.
- Accept a log URL only as a local web-path hint; the agent never fetches the log over HTTP. An absolute local path remains available for custom layouts.
- Report bounded source status and sanitized route name, method, and template so users can select which routes to monitor.
- Keep existing manual access-log routes and other agents unchanged; only selected routes produce diagnostic aggregates.
- Never send actual request URLs, raw log lines, request bodies, or response bodies to Pinguva.
