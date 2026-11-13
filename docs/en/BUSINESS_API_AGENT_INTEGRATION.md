# Business API Monitoring with Pinguva Agent

Use this integration when a customer has custom API routes and needs quick load,
error and latency visibility without changing the customer's backend code.

## Requirements

- A Linux server with Pinguva Agent `0.2.19` or newer.
- A standard Nginx or Apache access log at one of these paths:
  - `/var/log/nginx/access.log`;
  - `/var/log/nginx/api.access.log`;
  - `/var/log/apache2/access.log`;
  - `/var/log/httpd/access_log`.
- Pinguva `Enterprise` plan.

## Setup

1. Open `Integrations and API`.
2. Open `Business API Monitoring`.
3. Expand `Connection settings`.
4. Select a Linux server.
5. Click `Add route` and fill in the route form.
6. In `Route URL or path`, enter a full URL or only its path. For example,
   `https://api.example.com/api/orders/:id` is stored as `/api/orders/:value`.
   The domain and query string are not stored or sent.
7. Select the HTTP method, optionally enter the application log path, configure
   notification rules, and click `Add route`.

Select `ANY` when the route should match every HTTP method.

The log path is optional. Agent `0.2.19` first checks standard paths and parses
active `access_log` directives in Nginx and `CustomLog` or `TransferLog`
directives in Apache. Use a manual path only when the application log is not
declared in the web server configuration. The file remains local to the server.

The agent also supports logs where duration is inside the timestamp and the
status appears before the request, for example:

```text
203.0.113.10 - - [08/Oct/2026:21:40:58 +0500 - 0.146] 200 "POST /api/customer.segment HTTP/1.1" 272 "-" "GuzzleHttp/7" "-"
```

Version `0.2.19` also matches named path parameters, such as
`/api/orders/:order_id`, against any single real path segment. UUIDs, numeric
identifiers, and arbitrary customer codes therefore do not require separate
routes. The agent also recognizes common Nginx and Apache layouts with quoted
or unquoted requests, ISO timestamps, and JSON access logs with `request` or
`request_method`/`request_uri`, `status`, and duration fields. A non-standard
log file must be entered explicitly in the route settings.

### Application logs and format detection

The agent detects common log formats locally. It supports standard Nginx/Apache
lines, JSONL, and multiline PHP/cURL dumps with fields such as:

```text
Код ответа - 200
[url] => https://api.example.com/api/orders
[http_code] => 200
[total_time] => 0.843471
```

For a multiline application log, enter its absolute `Log path` for the specific
route. If the block does not contain an HTTP method, the agent matches it to the
configured route method and the explicit file only. If a readable file contains
no supported records, Pinguva reports an unrecognized format instead of showing
false zero traffic.

Starting with agent `0.2.20`, each configured file has its own source status:
file missing, read permission denied, or format unrecognized. A readable default
access log cannot hide a bad application-log path. Check the path on the selected
agent server, not on the Pinguva platform server:

### Outbound request JSON logs (agent 0.2.21+)

Agent 0.2.21 supports consecutive pretty-printed JSON objects as well as JSONL.
The supported fields are `time` (server-local time or ISO time with a zone),
`method`, `url`, `httpCode`, and `totalTime` (seconds). The `name` and
`requestUrl` fields are not needed for aggregation: the configured route matches
the `url` template. Actual request URLs, UUIDs, and query parameters are not sent.

```json
{
  "time": "13.11.2026 10:23:44",
  "name": "getPoints",
  "method": "GET",
  "url": "https://api.example.com/api/consumers/:uuid",
  "httpCode": 200,
  "totalTime": 0.146
}
```

An `httpCode` of `0` is not an HTTP response; it is counted as unexpected, not
successful. Connection failures should ideally have a separate technical flag.

Configure each route with an absolute **local file path on the agent server**,
for example `/home/bitrix/www/ilab/monitoring/request.json` for regular calls
and `/home/bitrix/www/ilab/monitoring/sms.json` for SMS. Confirm the real disk
path on that server; a website URL does not establish the local path. Several
routes can use the same `request.json`; method and path select their records.
Configure the actual GET/POST method and a path containing `:uuid` or `:value`.
The agent service user needs read access. The collector reads at most the last
32 MiB of each file and only reports the last completed minute, without
backfilling historical records.

Do not enter a public HTTPS URL in a manual route's Log file field. Use the
source setup below for URLs; the agent still reads files locally and never
fetches logs from websites. Deny web access to
`/ilab/monitoring/`, especially when `requestUrl` contains real identifiers or
other request parameters.

### Add a source and select routes (agent 0.2.22+)

In Integrations & API, select the Linux server and paste a log URL such as
`https://example.com/ilab/monitoring/request.json` into Outbound request logs,
or enter an absolute local file path. The URL is only a path hint: Pinguva
stores its pathname without the domain or parameters and never fetches the
file over HTTP. The agent checks standard web roots (`/home/bitrix/www`,
`/var/www/html`, `/srv/www`, `/usr/share/nginx/html`). If the file is elsewhere,
enter its absolute local path. Customer file names may differ; supported source
files have a `.json`, `.log`, or `.txt` extension inside a `monitoring`, `log`,
or `logs` directory.

After one or two agent reports, the UI shows the source status and the safe
names, methods, and route templates found in the last 4 MiB of the JSON log.
Records should include `name`, `method`, `url`, `httpCode`, and `totalTime` for
full diagnostics. Check the routes to monitor and save the selection. Only
selected routes become watched routes and appear in that server's API panel
after the next report. A missing or unreadable file is reported as `missing`
or `permission_denied`; it does not affect other servers or sources.

To replace a technical log name such as `getPoints` with a human-readable
label, open `Edit` on the configured route below the source list and change
`Route name`. The label appears in the server's API panel; the route method,
path, log file, and original `name` in the customer log are unchanged. Saving
the source checkboxes again does not reset this label.

The source list refreshes automatically while the first check is pending.
After changing a path or permissions, use `Refresh sources`; this does not
discard an unfinished route form.

Removing a source removes only routes selected through it, not manually
configured routes. Entering a URL does not make a public log safe: deny web
access to the log on the customer server.

Check access without printing the file contents:

```bash
sudo -u pinguva-agent test -r /absolute/path/to/app/log/request_orders.txt \
  && echo readable || echo missing_or_denied
sudo stat -c 'modified=%y size=%s' /absolute/path/to/app/log/request_orders.txt
```

After fixing the path, the agent collects the last completed minute. It does
not import log entries from before the route was configured. Make a new request
and allow 1-2 minutes for the next report. Email alerts also require the route's
email switch, a triggered alert rule, and the workspace email channel.

Only aggregates are sent. Response bodies, headers, tokens, UUIDs, URL
parameters, and raw log lines remain on the customer server.

### If the access log is unavailable

The agent reads the log as the `pinguva-agent` service user, not as `root`. Check
which account runs the service and whether it can read the configured file:

```bash
sudo systemctl show pinguva-agent -p User -p Group
sudo -u pinguva-agent head -c 1 /var/log/nginx/access.log >/dev/null \
  && echo readable || echo denied
```

If the file is owned by `bitrix:adm` with mode `0640`, grant the agent only the
required access with ACL:

```bash
sudo setfacl -m u:pinguva-agent:rx /var/log/nginx
sudo setfacl -m u:pinguva-agent:r /var/log/nginx/access.log
sudo systemctl restart pinguva-agent
```

For an application log inside a protected directory, grant `pinguva-agent`
execute access to each parent directory and read access only to the configured
file:

```bash
sudo setfacl -m u:pinguva-agent:--x /opt/app /opt/app/log
sudo setfacl -m u:pinguva-agent:r /opt/app/log/request_orders.txt
```

Replace the paths with the actual ones and verify access as the service user. Do
not grant the agent access to an entire directory when one file is configured.

Use the actual Apache directory and log file when Apache is installed. Do not use
`chmod 644`; that would expose the access log to every local user. After the
change, inspect the agent log:

```bash
sudo journalctl -u pinguva-agent -n 50 --no-pager \
  | grep -E 'access log|business API|report sent'
```

`access log unavailable` means that the agent could not open a suitable file.
`requests: 0` means that the file was opened, but no request for the selected
route was found in the current window. If log rotation creates a new file, keep
the ACL in the `logrotate` configuration or add `pinguva-agent` to the log-reading
group. Group membership grants access to other files in that group, so a
file-specific ACL is usually safer.

Example:

```text
GET /api/orders
POST /api/orders/:order_id/status
GET /api/loyalty/services/consumers/:uuid
POST /api/leads/actions
```

You can paste a full URL. Pinguva stores only the path, without the domain or
query string.

## Data sent to Pinguva

The agent sends only aggregated technical counters:

- route;
- HTTP method;
- request count;
- successful responses;
- business rejects `400`, `409`, `422`;
- client and server errors;
- average and maximum latency when the access-log format contains request time;
- masked traffic sources.

## Data not sent

The agent does not send:

- request bodies;
- response bodies;
- HTTP headers;
- cookies;
- query strings;
- tokens;
- raw access logs;
- personal data.

## Latency note

Nginx and Apache do not always write request processing time to the default
access log. If request time is absent, Pinguva still shows requests and errors,
but latency remains unavailable until the log format is adjusted.
