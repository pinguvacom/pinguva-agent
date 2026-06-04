import os
import time
import httpx
from fastapi import FastAPI, Request

app = FastAPI()
PINGUVA_INGEST_URL = os.getenv("PINGUVA_INGEST_URL")
PINGUVA_COLLECTOR_TOKEN = os.getenv("PINGUVA_COLLECTOR_TOKEN")

@app.middleware("http")
async def pinguva_collector(request: Request, call_next):
    started_at = time.monotonic()
    response = await call_next(request)

    if PINGUVA_INGEST_URL and PINGUVA_COLLECTOR_TOKEN:
        event = {
            "path": request.url.path,
            "method": request.method,
            "statusCode": response.status_code,
            "latencyMs": int((time.monotonic() - started_at) * 1000),
            "caller": request.headers.get("X-Client-App", "web"),
            "businessError": False,
        }
        try:
            async with httpx.AsyncClient(timeout=0.8) as client:
                await client.post(
                    PINGUVA_INGEST_URL,
                    headers={"Authorization": f"Bearer {PINGUVA_COLLECTOR_TOKEN}"},
                    json={"events": [event]},
                )
        except Exception:
            # Monitoring must never break the customer application.
            pass

    return response
