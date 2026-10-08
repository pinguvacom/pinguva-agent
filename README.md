# Pinguva Agent - Server Monitoring and Bitrix24 Diagnostics

[Pinguva](https://pinguva.com) is a SaaS infrastructure monitoring platform for IT outsourcing teams, web studios, system administrators, product teams, and companies managing multiple servers and customer projects.

The Pinguva platform monitors websites, APIs, TCP services, Ping availability, TLS certificates, domains, and server telemetry through lightweight Linux and Windows agents. Synthetic website, API, TCP, Ping, TLS, and domain checks are platform capabilities. The agent collects local server telemetry and reports it to Pinguva.

For self-hosted Bitrix24 installations, Pinguva Agent provides privacy-safe REST and MySQL diagnostics. It does not send webhook secrets, MySQL passwords, CRM records, raw access logs, source SQL values, configuration-file contents, or HTTP request and response bodies to the Pinguva platform.

## Key capabilities

Pinguva Agent provides the following confirmed capabilities:

- Versioned Linux and Windows agent source snapshots.
- Server telemetry collection for CPU, memory, disk, network, uptime, and Ping.
- Linux Disk I/O, watched-service, and configuration-change telemetry.
- Outbound reporting to the configured Pinguva endpoint. The agent does not open inbound listening ports.
- No remote command execution from the Pinguva platform. The agent may invoke local operating-system utilities only for its own collection and service-management tasks.
- Local Bitrix24 REST checks and self-hosted Bitrix24 load diagnostics on supported Linux installations.
- Local MySQL, MariaDB, and PostgreSQL diagnostics on supported Linux installations.
- Business API route diagnostics from local Linux access logs when routes are configured in Pinguva.
- Public integration guides, troubleshooting runbooks, and API Collector examples.

In the documented production configuration, the agent communicates with Pinguva over outbound HTTPS. Development and test configuration can use a separately configured local HTTP endpoint.

## Self-hosted Bitrix24 monitoring

Pinguva Agent helps technical teams investigate load on self-hosted Bitrix24 by collecting local technical aggregates for REST activity and MySQL. It provides diagnostic data that helps identify likely sources of load and investigate short-lived incidents. It does not claim to determine the exact cause of every incident automatically.

For a compatible, locally configured Bitrix24 installation, version `0.2.12` and newer can collect:

- REST request activity, 5xx counts, and route aggregates without query strings.
- REST method availability and latency for selected profiles.
- MySQL connection and activity counters, including `Threads_running` and active-query metrics when available.
- Long-running query categories, SQL digest aggregates, examined-row counters, no-index markers, and supported lock-wait information.
- Minute-level diagnostic history and load incidents.

The webhook remains only on the customer server. MySQL credentials remain local. Pinguva receives bounded technical aggregates only. The agent does not send CRM records, raw access-log lines, original SQL text or values, HTTP bodies, configuration-file contents, Bitrix24 webhook secrets, or MySQL passwords. A normalized `SELECT` structure is sent only when safe redaction succeeds; otherwise only the digest, category, and counters are sent.

## Business API route monitoring

Starting with `0.2.14`, Pinguva Agent can monitor customer-specific API routes from local Linux access logs after routes are selected in the Pinguva interface.

The agent reports aggregated technical counters only: route, HTTP method, request count, response-code groups, latency when the log format contains request time, and masked traffic sources. It does not send request bodies, response bodies, headers, cookies, query strings, tokens, raw access-log lines, or personal data.

## Security model

- The agent does not accept inbound network connections.
- It reports outward to Pinguva and does not expose a control endpoint for remote commands.
- Bitrix24 webhooks and MySQL credentials are stored and used locally on the customer server.
- The agent does not transmit configuration-file contents. Configuration monitoring reports bounded metadata and a SHA-256 fingerprint rather than file content.
- Bitrix24 diagnostics use aggregates and redacted data. Raw access logs, CRM data, raw SQL values, and HTTP request or response bodies are excluded.
- Business API diagnostics use configured route patterns and aggregated counters. Raw access-log lines, query strings, request bodies, response bodies, headers, cookies, and tokens are excluded.

See [Security Policy](./SECURITY.md) and [Privacy Notes](./PRIVACY.md) for the detailed scope.

## Repository contents

This repository contains:

- Release-specific source snapshots of Pinguva Agent.
- Linux and Windows agent source code in each snapshot.
- Security, privacy, and license documents.
- Bitrix24 local integration documentation.
- Agent troubleshooting runbooks.
- API Collector integration documentation and examples for Node.js, PHP, and FastAPI.
- Database diagnostics documentation for MySQL, MariaDB, and PostgreSQL.
- Business API route monitoring documentation for Linux access-log based diagnostics.

Each version directory is self-contained and includes its own `go.mod`, README, license, security notes, privacy notes, and changelog.

## Source snapshots

- [`0.2.3`](./0.2.3)
- [`0.2.4`](./0.2.4)
- [`0.2.5`](./0.2.5)
- [`0.2.6`](./0.2.6)
- [`0.2.7`](./0.2.7)
- [`0.2.8`](./0.2.8)
- [`0.2.9`](./0.2.9)
- [`0.2.10`](./0.2.10)
- [`0.2.11`](./0.2.11)
- [`0.2.12`](./0.2.12)
- [`0.2.13`](./0.2.13)
- [`0.2.14`](./0.2.14)
- [`0.2.15`](./0.2.15)
- [`0.2.16`](./0.2.16)
- [`0.2.17`](./0.2.17)

## Repository scope

This repository contains the source code of the Pinguva monitoring agent for transparency and security review.

The Pinguva SaaS backend, web application, internal server-side logic, alerting infrastructure, and commercial platform components are not included and remain closed-source.

Pinguva Agent is not the complete Pinguva SaaS platform and this repository does not provide a self-hosted Pinguva backend.

## Current source version

The latest published source snapshot is [`0.2.17`](./0.2.17).

Confirmed additions after `0.2.12` include separate MySQL, MariaDB, and PostgreSQL diagnostics in `0.2.13`, Business API route monitoring from local Linux access logs in `0.2.14`, Excel route methods and safe application log paths in `0.2.15`, automatic access-log discovery in `0.2.16`, and support for status-before-request access-log formats in `0.2.17`. See the release-specific `CHANGELOG.md` files for details.

## Documentation

- [Bitrix24 Local Integration](./docs/en/BITRIX24_LOCAL_INTEGRATION.md)
- [API Collector Integration](./docs/en/API_COLLECTOR_INTEGRATION.md)
- [Agent Troubleshooting Runbook](./docs/en/AGENT_TROUBLESHOOTING_RUNBOOK.md)
- [MySQL, MariaDB and PostgreSQL Diagnostics](./docs/en/DATABASE_DIAGNOSTICS.md)
- [Business API Monitoring with Pinguva Agent](./docs/en/BUSINESS_API_AGENT_INTEGRATION.md)
- [API Collector examples](./examples/business-api-collector/README.md)
- [Documentation index and Russian documentation](./docs/README.md)

## Links

- [Pinguva website](https://pinguva.com)
- [Bitrix24 integration](https://pinguva.com/bitrix24-integration/)
- [Pinguva application](https://monit.pinguva.com)

## Contributing and reporting issues

Read [CONTRIBUTING.md](./CONTRIBUTING.md) before opening a public issue or pull request. Do not put webhook URLs, tokens, passwords, customer data, raw logs, or security-vulnerability details into public reports.

## Russian documentation

Русская документация сохранена в [docs/ru](./docs/ru/). Для обзора всех публичных документов используйте [docs/README.md](./docs/README.md).
