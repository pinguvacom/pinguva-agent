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
