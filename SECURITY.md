# Security Policy

Pinguva publishes this archive for transparency and security review of the monitoring agent. It does not include the Pinguva SaaS backend, web application, alerting infrastructure, or internal server-side logic.

## Customer security review

- [Agent deployment, network flows and transmitted data (English)](./docs/en/SECURITY_REVIEW.md)
- [Описание для службы ИБ клиента (русский)](./docs/ru/SECURITY_REVIEW.md)
- [Privacy Notes](./PRIVACY.md)

The agent does transmit authenticated technical telemetry and enabled diagnostic aggregates. "No raw customer data" must not be read as "no data leaves the server": hostnames, IP addresses, selected paths and service names can be included. The review guides distinguish local-only secrets and raw content from metadata and aggregate fields sent to Pinguva.

## Reporting a vulnerability

- Do not publish a vulnerability in a GitHub issue, discussion, pull request, or release comment before it is resolved.
- Contact Pinguva through the [official website](https://pinguva.com) and clearly mark the request as a security report.
- Include the affected agent version, a sanitized reproduction, impact, and the smallest useful proof of concept.
- Do not include tokens, webhook URLs, passwords, state files, customer data, production configuration, raw access logs, or unredacted SQL.

## Scope

This policy covers the public agent source code in this repository. Reports about closed Pinguva platform components are welcome through the same private channel, but the relevant code is not included here.

## Русский

Этот архив публикуется для прозрачности и аудита кода агента Pinguva. Backend, SaaS-сервис, инфраструктура уведомлений, веб-приложение и внутренняя серверная логика Pinguva в него не входят.

Для согласования установки с ИБ используйте [описание сетевых потоков и передаваемых полей](./docs/ru/SECURITY_REVIEW.md). Агент отправляет техническую телеметрию и включённые агрегаты; утверждение «ничего не передаёт» неверно. Секреты и сырые журналы остаются локально, но hostname, IP, выбранные пути и названия служб могут входить в отчёт.

Если вы нашли уязвимость:

- не публикуйте её в issue, pull request, discussion или комментарии к релизу до исправления;
- свяжитесь с Pinguva через [официальный сайт](https://pinguva.com) и явно укажите, что это security-обращение;
- приложите версию агента, безопасное описание воспроизведения и оценку влияния;
- не включайте токены, webhook URL, пароли, state-файлы, данные клиентов, рабочие конфигурации, сырые access log или исходный SQL.
