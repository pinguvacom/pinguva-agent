# Business API Monitoring with Pinguva Agent

Use this integration when a customer has custom API routes and needs quick load,
error and latency visibility without changing the customer's backend code.

## Requirements

- A Linux server with Pinguva Agent `0.2.17` or newer.
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
5. Paste API routes, one route per line.
6. Click `Save routes`.

If you paste only a URL, the route matches any HTTP method. When pasting rows
from Excel, use four tab-separated columns:

```text
Name<TAB>URL<TAB>Method<TAB>Log path
Order creation<TAB>https://api.example.com/api/orders<TAB>POST<TAB>/opt/app/log/request_orders.txt
```

The log path is optional. Agent `0.2.17` first checks standard paths and parses
active `access_log` directives in Nginx and `CustomLog` or `TransferLog`
directives in Apache. Use a manual path only when the application log is not
declared in the web server configuration. The file remains local to the server.

The agent also supports logs where duration is inside the timestamp and the
status appears before the request, for example:

```text
203.0.113.10 - - [08/Oct/2026:21:40:58 +0500 - 0.146] 200 "POST /api/customer.segment HTTP/1.1" 272 "-" "GuzzleHttp/7" "-"
```

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
