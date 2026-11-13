# Pinguva Agent - security review for customer IT and security teams

This document describes the customer-side agent in the public `0.2.23` source snapshot. It is an agent data-flow description, not a security certification or a description of the closed Pinguva SaaS backend. The [Russian version](../ru/SECURITY_REVIEW.md) has the same scope.

## What is installed

Pinguva Agent runs on the customer's Linux or Windows server. It collects host metrics and sends them to the configured Pinguva server about once per minute. Linux-only integrations can additionally read configured local logs and query database system views. Optional Bitrix24 REST checks use a webhook stored on the customer server. The agent does not open a listening port or accept inbound commands from Pinguva.

The operator installs or updates the binary using a command from the Pinguva server card. Installation, local integration setup, file permissions and firewall policy remain under the customer's control. Installing the agent does not grant Pinguva interactive SSH access to the server.

## Network and protocol flows

| Flow | Direction | Protocol | When | Data exchanged |
| --- | --- | --- | --- | --- |
| Enrollment | Agent to configured `AGENT_SERVER` | HTTP POST `/api/agent/enroll`, HTTPS in production | First enrollment or re-enrollment | Initial telemetry report and temporary enrollment token in the `Authorization: Bearer` header; response contains agent ID and permanent token. |
| Regular report | Agent to configured `AGENT_SERVER` | HTTP POST `/api/agent/report`, HTTPS in production | About every 60 seconds | JSON host telemetry and enabled integration summaries; permanent token in the Bearer header. Response can contain selected routes, log-source hints and a Bitrix24 diagnostics enable flag. It contains no shell command field. |
| Optional diagnostic history | Agent to the same configured server | HTTP POST `/api/agent/v1/bitrix24-diagnostics` or `/api/agent/v1/database-diagnostics`, HTTPS in production | When the corresponding Linux integration and local service are enabled | Bounded diagnostic events and acknowledgment IDs. |
| Optional Bitrix24 REST check | Agent to the customer-configured Bitrix24 portal | HTTPS | Only after local webhook setup | Selected REST checks using the locally stored webhook; Pinguva receives a summary, not the webhook or CRM response body. |
| Optional log and database reads | Agent to local filesystem or database client | Local file I/O, database client/socket as configured | Only for enabled integrations | Local source data is reduced to technical summaries before reporting. Database client configuration can affect how the database connection is made. |
| Binary installation/update | Customer-run install command to the Pinguva distribution endpoint | HTTPS download | Only when the operator runs the command | Installer and agent binary; this is separate from the periodic report. |

For the documented Pinguva deployment, allow outbound TCP 443 to `monit.pinguva.com`. Allow access to a Bitrix24 portal only if REST checks are enabled. The agent's Ping metric also makes a short outbound TCP connection to the configured server host. No inbound firewall rule is required for the agent. Review the actual `AGENT_SERVER` value before approval: the source permits a custom HTTP URL for development and does **not** enforce HTTPS. Customer production configuration must use `https://` and normal certificate validation. The agent does not configure a custom TLS verification bypass.

## Data sent to Pinguva

| Mode | Fields or categories that can leave the customer server |
| --- | --- |
| Base report | Agent/install IDs, display name, hostname, server IP, OS/platform/kernel/architecture, agent version, report time, CPU/load/core use, memory, disks, network and disk I/O rates, uptime, Ping, and a bounded error/status field. |
| Configured Linux observations | Service names and status. For watched configuration profiles: profile key, file path, type, existence, modification time, size and SHA-256 fingerprint, **not** file contents. |
| Configured Bitrix24 | Check status, selected REST method/profile, latency, safe result counts and sanitized error category; normalized access-log route and 5xx aggregates; MySQL activity, digest and lock counters. Some digest records can include a redacted `SELECT` structure and schema name when normalization succeeds. |
| Configured database diagnostics | Database type/version, instance or endpoint metadata, connection and activity counters, wait/lock indicators, technical query groups, digest/hash, operation and database name where available. |
| Configured Business API monitoring | Chosen route name, HTTP method and normalized path; per-minute request and response-category counts, numeric error-code counts, average/maximum latency when logged, and a traffic-source summary. Log-source discovery can also report the resolved **local file path**, source status and discovered safe route names/methods/paths. |

Hostnames, server IPs, file paths, database/schema names, configured route labels and source addresses are metadata, but can still be sensitive in a customer's environment. The current source-summary function abbreviates common IP-looking values; other source strings can pass through (limited to 80 characters). Do not put credentials, customer IDs or personal data into route names, watched file paths or log `source` fields. Review the selected observation scope before enabling it.

## Data kept local

The agent's reporting code does not send Bitrix24 webhook URLs, database passwords, OAuth/API tokens, CRM records, original SQL values, raw access-log lines, complete configuration files, HTTP request/response bodies, cookies or authorization headers. The agent parses local logs and reduces matching records to aggregates. It strips URL query parameters and normalizes dynamic route segments before reporting. Error details in the UI are counts by interval and HTTP code, not individual request records or exact per-request timestamps.

The agent token is different: it **is transmitted** in the HTTPS `Authorization: Bearer` header to authenticate the report. It is not a field inside the telemetry JSON. Protect it as a credential and do not share headers, state files or packet captures publicly. On Linux the agent writes its state file with `0600` permissions; the standard location is `/var/lib/pinguva-agent/state.json`. On Windows the default is `%ProgramData%\Pinguva\agent\state.json`; inspect and restrict its Windows ACL separately, because Unix permission bits do not establish a Windows ACL policy. A Bitrix24 webhook is stored in `/etc/pinguva-agent/bitrix24.json` after explicit local setup. The optional `/root/.my.cnf` is used only when it passes the agent's strict ownership and `0600` checks; otherwise the collector falls back to its configured local connection path.

The exclusions above describe the intended implemented collectors. They are not a claim that arbitrary customer-defined labels or filesystem paths are automatically free of sensitive information. The customer should keep raw logs private and grant the agent only the access required for selected integrations.

## Illustrative wire format

The examples below are invented and deliberately incomplete. They show protocol shape, not a full production payload or real customer data. Never include a live Bearer token in an approval ticket.

```http
POST /api/agent/report HTTP/1.1
Host: monit.pinguva.com
Content-Type: application/json
Authorization: Bearer <REDACTED_AGENT_TOKEN>

{"agentId":"agt_example","hostname":"example-host","ip":"192.0.2.10","os":"linux","version":"0.2.23","cpuPercent":12.5,"memoryPercent":38.0,"reportedAt":"2026-01-01T00:00:00Z"}
```

An enabled Business API route can add an aggregate such as:

```json
{
  "businessApiDiagnostics": [{
    "bucketStart": "2026-01-01T00:00:00Z",
    "bucketSeconds": 60,
    "routes": [{
      "name": "Example operation",
      "method": "POST",
      "path": "/api/orders/:value",
      "requests": 12,
      "serverErrors": 2,
      "errorStatusCodes": [{"code": 503, "count": 2}],
      "latencyAvailable": true,
      "averageLatencyMs": 146,
      "maxLatencyMs": 1250
    }]
  }]
}
```

The regular report response may contain configuration, for example `businessApiEndpointWatches` with route patterns and `bitrix24LoadDiagnosticsEnabled`. It is a configuration response to the agent's outbound report, not a remote shell session.

## Customer verification checklist

1. Approve the agent version and inspect its source under [`0.2.23`](../../0.2.23/), especially `main.go`, `business_api_diagnostics.go`, `bitrix24_load_diagnostics.go` and `database_diagnostics.go`.
2. Confirm the configured server URL uses HTTPS, the required destination is allowlisted, and no inbound agent port is exposed.
3. Approve optional Bitrix24 webhook permissions, database privileges, log paths and watched configuration profiles separately. Leave unused integrations disabled.
4. Check local service identity and listening sockets without exposing credentials:

   ```bash
   sudo systemctl show pinguva-agent -p User -p Group
   sudo ss -ltnp | grep pinguva-agent || echo 'No agent listening socket'
   sudo stat -c '%a %U:%G %n' /var/lib/pinguva-agent/state.json
   ```

5. Redact all tokens, headers, hostnames, IPs, file paths and customer data before sharing logs or screenshots with a third party. Do not publish packet captures or customer review documents.

On Windows, the default service name is `PinguvaAgent` (it can be changed at setup). The following checks show service identity and state-file ACL without reading the token:

```powershell
Get-CimInstance Win32_Service -Filter "Name='PinguvaAgent'" | Select-Object State, StartName, ProcessId
Get-Acl "$env:ProgramData\Pinguva\agent\state.json" | Format-List Path, AccessToString
```

This public repository does not establish SaaS-side retention, storage region, access control, backups or incident-response commitments. Request those controls from Pinguva separately through the [official website](https://pinguva.com). For vulnerability reporting, see [SECURITY.md](../../SECURITY.md).
