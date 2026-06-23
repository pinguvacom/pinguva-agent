# Pinguva Agent 0.2.5

## Что изменилось

- Исправлена необязательная проверка доступности метода Bitrix24:
  `method_discovery` теперь вызывает `method.get` с обязательным параметром
  `name=user.get`.
- Профиль `method_discovery` остаётся выключенным по умолчанию и включается
  только вручную.
- Профиль `scope` в интерфейсе и отчётах называется «Проверка выданных
  REST-прав», чтобы не создавать впечатление, что Bitrix24 требует отдельное
  право webhook.

## REST-профили

- `basic` — `user.current`;
- `scope` — `scope`, проверка выданных REST-прав;
- `crm_deals` — `crm.item.list` для сделок;
- `crm_leads` — `crm.item.list` для лидов;
- `crm_contacts` — `crm.item.list` для контактов;
- `crm_statuses` — `crm.status.list` для стадий;
- `method_discovery` — `method.get` с `name=user.get`, вручную при необходимости.

## Безопасность

- Webhook URL Bitrix24 хранится только локально на сервере клиента.
- Webhook URL не передается в Pinguva и не показывается в веб-интерфейсе.
- Тела ответов Bitrix24, CRM-данные и пользовательские данные не отправляются.
- Ошибки перед отправкой проходят redaction: webhook secret, auth/access/refresh
  token и похожие параметры заменяются на `[redacted]`.
