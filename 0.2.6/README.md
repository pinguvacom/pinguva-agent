# Pinguva Agent 0.2.6

## Русский

Версия `0.2.6` добавляет безопасную локальную диагностику нагрузки для
коробочного Bitrix24. Это тот же Linux-агент Pinguva: отдельный агент не
устанавливается.

После подключения Bitrix24 системный таймер раз в минуту локально собирает
ограниченную сводку из access log и MySQL. Обычный агент отправляет в Pinguva
только эту сводку.

В Pinguva передаются:

- количество динамических запросов и ошибок `5xx` за последние 15 минут;
- нормализованные пути без query string, например `/api/accruedpoints`;
- источники трафика с маскированным последним сегментом IP;
- `Threads_running`, число активных запросов и максимальная длительность
  текущего MySQL-запроса;
- фиксированные категории запросов, без текста SQL.

В Pinguva **не** передаются:

- сырые строки access log;
- параметры URL, cookies и заголовки `Authorization`;
- Bitrix24 webhook URL, токены, тела HTTP-запросов и ответов;
- CRM-записи, телефоны, email, комментарии и пользовательские поля;
- текст SQL, UUID и любые значения SQL-запросов.

Агент не принимает входящие подключения, работает только через исходящий
HTTPS и не выполняет удалённые команды. Это можно проверить по исходникам.

Подключение Bitrix24 выполняется командой из UI Pinguva. Она обновляет уже
установленный агент, затем просит вставить webhook скрытым вводом:

```bash
curl -fsSL "https://monit.pinguva.com/install/bitrix24.sh" | sudo bash -s -- --base-url "https://crm.example.kz"
```

Проверка локальной диагностики:

```bash
sudo systemctl status pinguva-bitrix24-diagnostics.timer --no-pager
sudo journalctl -u pinguva-bitrix24-diagnostics.service -n 50 --no-pager
```

Полная инструкция:

- [Русская документация Bitrix24](../docs/ru/BITRIX24_LOCAL_INTEGRATION.md)
- [English Bitrix24 documentation](../docs/en/BITRIX24_LOCAL_INTEGRATION.md)

Проверка исходников:

```bash
go test .
go build .
```

## English

Version `0.2.6` adds safe local load diagnostics for self-hosted Bitrix24. It
uses the existing Pinguva Linux agent; no separate agent is installed.

After Bitrix24 setup, a system timer creates a bounded local access-log and
MySQL summary once a minute. The regular agent sends only that summary to
Pinguva.

Pinguva receives:

- dynamic request and `5xx` counts for the last 15 minutes;
- normalized paths without query strings, for example `/api/accruedpoints`;
- traffic sources with the final IP segment masked;
- `Threads_running`, active-query count and longest current MySQL query;
- fixed query categories, without SQL text.

Pinguva does **not** receive:

- raw access-log lines;
- URL parameters, cookies or `Authorization` headers;
- Bitrix24 webhook URLs, tokens, HTTP request or response bodies;
- CRM records, phone numbers, email addresses, comments or custom fields;
- SQL text, UUIDs or SQL values.

The agent accepts no inbound connections, uses outbound HTTPS only and does
not execute remote commands. This can be verified in the source code.

Use the command generated in the Pinguva UI to connect Bitrix24. It updates
the existing agent and asks for the webhook through hidden terminal input:

```bash
curl -fsSL "https://monit.pinguva.com/install/bitrix24.sh" | sudo bash -s -- --base-url "https://crm.example.kz"
```

Check local diagnostics:

```bash
sudo systemctl status pinguva-bitrix24-diagnostics.timer --no-pager
sudo journalctl -u pinguva-bitrix24-diagnostics.service -n 50 --no-pager
```

Full documentation:

- [Russian Bitrix24 documentation](../docs/ru/BITRIX24_LOCAL_INTEGRATION.md)
- [English Bitrix24 documentation](../docs/en/BITRIX24_LOCAL_INTEGRATION.md)

Verify the source archive:

```bash
go test .
go build .
```
