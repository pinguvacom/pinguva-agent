# Security Policy

## Русский

Этот каталог публикуется для прозрачности и аудита агента `0.2.13`. Backend,
веб-приложение и коммерческая платформа Pinguva сюда не входят.

Локальная диагностика использует только фиксированные read-only запросы. Она
не открывает сетевой listener, не принимает команды от backend и не передаёт
секреты СУБД. Конфиги и очередь создаются с root-only правами.

Если вы нашли уязвимость, передайте описание и шаги воспроизведения команде
Pinguva через закрытый официальный канал. Не прикладывайте рабочие токены,
пароли и конфигурации к публичным issue.

## English

This directory is published for transparency and security review of agent
`0.2.13`. It does not include the Pinguva backend, web application or commercial
platform.

Local diagnostics use fixed read-only queries only. They open no network
listener, accept no backend commands and transmit no database credentials.
Configuration and queue files use root-only permissions.

Report vulnerabilities to the Pinguva team through a private official channel.
Do not attach live tokens, passwords or configurations to public issues.
