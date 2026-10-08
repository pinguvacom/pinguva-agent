# Business API Monitoring with Pinguva Agent

Use this integration when a customer has custom API routes and needs quick load,
error and latency visibility without changing the customer's backend code.

## Requirements

- A Linux server with Pinguva Agent `0.2.14` or newer.
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
