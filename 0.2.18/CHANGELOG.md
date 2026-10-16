# Pinguva Agent 0.2.18 Changes

- Match configured named path parameters such as `:uuid`, `:code`, and `:order_id` to real request path segments.
- Parse common Nginx and Apache access-log layouts, including unquoted requests and JSON access logs.
- Keep raw log lines and request data local; send aggregated counters only.
