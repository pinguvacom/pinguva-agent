# API Collector Integration

Public documentation for customer developers.

The API Collector lets a customer's product send short events about real API traffic to Pinguva. It is not an external URL check. Events are created inside the customer's application after the request is processed.

## What The Customer Application Does

1. The customer's API endpoint receives a request.
2. The application processes the business operation.
3. Server-side code measures processing time.
4. The code detects HTTP status and optional business error.
5. The code sends a short event to Pinguva.

Pinguva does not need request bodies, response bodies, personal data, customer tokens or raw application logs.

## Endpoint

```text
POST https://app.pinguva.com/api/business-api/ingest
Authorization: Bearer bapi_...
Content-Type: application/json
```

Create the collector token in Pinguva:

1. Open `Integrations and API`.
2. Open `Business API Monitoring`.
3. Expand `Connection settings` at the bottom.
4. Create a collector token.
5. Copy the token immediately.

The full token is shown only once. Pinguva stores only the token hash and a short masked hint. If the token is lost, rotate it. After rotation, update every application, service and device that sends events to Pinguva.

## Where To Put The Collector Token

Use the token only in the customer's server-side code, for example:

- backend application;
- HTTP framework middleware;
- API gateway;
- worker that processes API events;
- internal SDK used by API handlers.

Send it as a Bearer token:

```text
Authorization: Bearer bapi_...
```

Do not put the token into:

- browser pages;
- public JavaScript;
- URL query strings;
- application logs.

## Request Format

```json
{
  "events": [
    {
      "endpoint": "order.create",
      "path": "/api/order.create",
      "method": "POST",
      "statusCode": 200,
      "latencyMs": 142,
      "caller": "mobile-app",
      "sourceIp": "203.0.113.10",
      "businessError": false,
      "businessCode": ""
    }
  ]
}
```

Send up to `250` events per request. Split larger batches.

## Event Fields

| Field | Required | Description |
| --- | --- | --- |
| `endpoint` | No | Endpoint ID or name configured in Pinguva. |
| `path` | Yes | API path without query string, for example `/api/order.create`. |
| `method` | Yes | HTTP method: `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, etc. |
| `statusCode` | Yes | HTTP response status code. |
| `latencyMs` | Yes | Processing time in milliseconds. |
| `caller` | No | Safe source label: `mobile-app`, `web`, `partner-crm`, `bitrix24`, `1c`. |
| `sourceIp` / `callerIp` | No | Source IP. Pinguva does not need raw IP for standard SaaS analytics. Prefer `caller` or an anonymized source identifier. |
| `businessError` | No | `true` when HTTP status is successful but the business operation failed. |
| `businessCode` | No | Short business error code without personal data, for example `validation_failed`. |
| `occurredAt` | No | ISO-8601 event time. If omitted, Pinguva uses receive time. |

## Do Not Send

Do not send:

- request body;
- response body;
- query strings with parameters;
- passwords;
- access tokens, refresh tokens or API keys;
- phone numbers;
- customer email addresses;
- names;
- addresses;
- order contents;
- CRM comments;
- raw application logs.

Use `caller` for traffic source analytics:

```json
{
  "caller": "mobile-app"
}
```

If IP analytics are required, confirm it with the customer's privacy policy and prefer a hash or internal source ID.

## Matching Events To Endpoints

Pinguva accepts events only for API methods configured in advance.

Matching order:

1. `endpoint` equals the Pinguva endpoint ID.
2. `endpoint` equals the Pinguva endpoint name.
3. `method + path` equals a configured manual check URL.
4. `path` equals a configured manual check URL.

Unknown or ambiguous events are ignored to prevent noisy high-cardinality data.

## cURL Example

```bash
curl -X POST "https://app.pinguva.com/api/business-api/ingest" \
  -H "Authorization: Bearer bapi_your_token" \
  -H "Content-Type: application/json" \
  -d '{
    "events": [
      {
        "path": "/api/order.create",
        "method": "POST",
        "statusCode": 200,
        "latencyMs": 142,
        "caller": "web",
        "businessError": false
      }
    ]
  }'
```

## Principles

- Monitoring must never break the customer application.
- Send events asynchronously when possible.
- Never include secrets or personal data.
- Use short stable labels for sources and business error codes.
