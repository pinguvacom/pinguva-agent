const PINGUVA_INGEST_URL = process.env.PINGUVA_INGEST_URL;
const PINGUVA_COLLECTOR_TOKEN = process.env.PINGUVA_COLLECTOR_TOKEN;

function pinguvaCollector() {
  return function(req, res, next) {
    const startedAt = Date.now();

    res.on("finish", function() {
      if (!PINGUVA_INGEST_URL || !PINGUVA_COLLECTOR_TOKEN) return;

      const routePath = req.route && req.route.path
        ? String(req.baseUrl || "") + String(req.route.path)
        : req.path;

      const event = {
        path: routePath,
        method: req.method,
        statusCode: res.statusCode,
        latencyMs: Date.now() - startedAt,
        caller: req.get("X-Client-App") || "web",
        businessError: Boolean(res.locals && res.locals.businessError),
        businessCode: res.locals && res.locals.businessCode ? String(res.locals.businessCode) : ""
      };

      fetch(PINGUVA_INGEST_URL, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${PINGUVA_COLLECTOR_TOKEN}`,
          "Content-Type": "application/json"
        },
        body: JSON.stringify({ events: [event] })
      }).catch(function() {
        // Monitoring must never break the customer application.
      });
    });

    next();
  };
}

module.exports = { pinguvaCollector };
