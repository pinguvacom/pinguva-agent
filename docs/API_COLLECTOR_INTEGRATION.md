# API Collector integration

Публичная документация для разработчиков клиента.

`API Collector` нужен, чтобы продукт клиента отправлял в Pinguva короткие
события о реальной работе своих API-методов. Это не внешняя проверка URL.
Событие создаётся на стороне приложения клиента: в middleware, SDK, backend
handler или gateway после обработки запроса.

## Что делает клиентское приложение

Разработчик клиента добавляет отправку события в код продукта:

1. API endpoint клиента принял запрос.
2. Приложение обработало бизнес-операцию.
3. Middleware измерил длительность обработки.
4. Middleware определил HTTP status и business error.
5. Middleware отправил короткое событие в Pinguva.

Pinguva не требует передавать тело запроса, тело ответа, персональные данные,
токены клиента или произвольные логи.

## Endpoint

```text
POST https://app.pinguva.com/api/business-api/ingest
Authorization: Bearer bapi_...
Content-Type: application/json
```

Collector token создаётся в интерфейсе Pinguva:

1. Открыть `Интеграции и API`.
2. Открыть `Мониторинг бизнес-API`.
3. Внизу раскрыть `Настройки подключения`.
4. Нажать `Создать токен сборщика`.
5. Скопировать токен сразу после создания.

Полное значение токена показывается один раз. В базе Pinguva хранится только
hash токена и короткая маска. Если токен потерян, его нужно перевыпустить.
После перевыпуска старый токен сразу перестаёт работать, поэтому новый токен
нужно заменить во всех приложениях, сервисах и устройствах, которые отправляют
события в Pinguva.

## Куда добавлять collector token

Токен нужен только той части продукта клиента, которая отправляет события в
Pinguva. Обычно это:

- backend-приложение;
- middleware HTTP-фреймворка;
- API gateway;
- worker, который обрабатывает очередь событий;
- внутренний SDK, через который проходят API-запросы.

Токен передаётся в запросе к Pinguva как Bearer token:

```text
Authorization: Bearer bapi_...
```

Не добавляйте collector token:

- в публичную часть сайта;
- в браузер пользователя;
- в публичный JavaScript;
- в URL query string;
- в логи приложения.

## Формат запроса

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

Можно отправлять до `250` событий одним запросом. Если событий больше,
разбейте их на несколько запросов.

## Поля события

| Поле | Обязательное | Описание |
| --- | --- | --- |
| `endpoint` | Нет | ID или имя метода, настроенного в Pinguva. Если указан, помогает точнее сопоставить событие. |
| `path` | Да | Путь API без query string, например `/api/order.create`. |
| `method` | Да | HTTP-метод: `GET`, `POST`, `PUT`, `PATCH`, `DELETE` и т.д. |
| `statusCode` | Да | HTTP status ответа клиента. |
| `latencyMs` | Да | Длительность обработки запроса в миллисекундах. |
| `caller` | Нет | Понятное имя источника: `mobile-app`, `web`, `partner-crm`, `bitrix24`, `1c`. |
| `sourceIp` / `callerIp` | Нет | IP источника. В текущем безопасном режиме Pinguva его принимает как поле, но не использует в аналитике и не хранит для агрегатов. Лучше передавать `caller` или обезличенный идентификатор. |
| `businessError` | Нет | `true`, если HTTP-ответ успешный, но бизнес-операция завершилась ошибкой. |
| `businessCode` | Нет | Короткий код бизнес-ошибки без персональных данных: `validation_failed`, `payment_rejected`. |
| `occurredAt` | Нет | ISO-8601 время события. Если не передано, Pinguva использует время приёма. |

## Что нельзя отправлять

Не отправляйте в collector:

- тело HTTP-запроса;
- тело HTTP-ответа;
- query string с параметрами;
- пароли;
- access token, refresh token, API key;
- номера телефонов;
- email клиентов;
- ФИО;
- адреса;
- содержимое заказов;
- комментарии CRM;
- произвольные application logs.

Если нужно анализировать источник трафика, используйте `caller`:

```json
{
  "caller": "mobile-app"
}
```

Если бизнесу всё же нужен IP, согласуйте это с политикой приватности клиента и
передавайте обезличенное значение, например hash или внутренний source id. Для
SaaS-аналитики Pinguva сырой IP не нужен.

## Как Pinguva сопоставляет событие с методом

Pinguva принимает только события для заранее настроенных методов API.

Сопоставление идёт по порядку:

1. `endpoint` совпал с ID метода в Pinguva.
2. `endpoint` совпал с именем метода в Pinguva.
3. `method + path` совпали с URL ручной проверки.
4. `path` совпал с URL ручной проверки.

Если метод не найден или совпадение неоднозначное, событие игнорируется.
Это защищает от случайного взрыва кардинальности и мусорных данных.

## Пример: cURL

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

## Пример: Node.js / Express

```js
const PINGUVA_INGEST_URL = process.env.PINGUVA_INGEST_URL;
const PINGUVA_COLLECTOR_TOKEN = process.env.PINGUVA_COLLECTOR_TOKEN;

function pinguvaCollector() {
  return function(req, res, next) {
    const startedAt = Date.now();

    res.on("finish", function() {
      if (!PINGUVA_INGEST_URL || !PINGUVA_COLLECTOR_TOKEN) return;

      const event = {
        path: req.route && req.route.path ? String(req.baseUrl || "") + String(req.route.path) : req.path,
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
```

## Пример: PHP

```php
function pinguva_send_event(array $event): void {
    $url = getenv('PINGUVA_INGEST_URL');
    $token = getenv('PINGUVA_COLLECTOR_TOKEN');
    if (!$url || !$token) {
        return;
    }

    $payload = json_encode(['events' => [$event]], JSON_UNESCAPED_SLASHES);

    $ch = curl_init($url);
    curl_setopt_array($ch, [
        CURLOPT_POST => true,
        CURLOPT_RETURNTRANSFER => true,
        CURLOPT_TIMEOUT_MS => 800,
        CURLOPT_HTTPHEADER => [
            'Authorization: Bearer ' . $token,
            'Content-Type: application/json',
        ],
        CURLOPT_POSTFIELDS => $payload,
    ]);
    curl_exec($ch);
    curl_close($ch);
}

$startedAt = microtime(true);

// ... обработка запроса продукта ...

pinguva_send_event([
    'path' => parse_url($_SERVER['REQUEST_URI'] ?? '/', PHP_URL_PATH) ?: '/',
    'method' => $_SERVER['REQUEST_METHOD'] ?? 'GET',
    'statusCode' => http_response_code() ?: 200,
    'latencyMs' => (int) round((microtime(true) - $startedAt) * 1000),
    'caller' => $_SERVER['HTTP_X_CLIENT_APP'] ?? 'web',
    'businessError' => false,
]);
```

## Пример: Python / FastAPI

```python
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
```

## Ответ API

Успешный приём:

```json
{
  "accepted": 1,
  "ignored": 0
}
```

`accepted` — сколько событий принято и сопоставлено с настроенными методами.
`ignored` — сколько событий отброшено из-за неизвестного endpoint, старого
времени, слишком большого payload или невалидных полей.

## Production checklist

Перед запуском у клиента:

- храните collector token только в secret manager или переменных окружения;
- не логируйте collector token;
- не отправляйте событие синхронно так, чтобы оно тормозило бизнес-запрос;
- ставьте короткий timeout `500-1000 ms`;
- добавьте sampling, если endpoint очень нагруженный;
- отправляйте batch событиями, если есть очередь;
- проверяйте, что `path` не содержит query string;
- используйте `caller` вместо сырого IP;
- не отправляйте персональные данные.
