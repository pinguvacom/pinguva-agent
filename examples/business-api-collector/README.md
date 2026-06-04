# Pinguva Business API Collector examples

These examples show how a customer application can send short API events to Pinguva.

The collector sends only technical summaries:

- path;
- method;
- statusCode;
- latencyMs;
- caller;
- businessError;
- businessCode.

Do not send request bodies, response bodies, query strings, personal data,
secrets, authorization tokens or raw application logs.

Full integration guide:

- `../../docs/API_COLLECTOR_INTEGRATION.md`
