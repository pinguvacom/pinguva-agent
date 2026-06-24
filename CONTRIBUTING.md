# Contributing to Pinguva Agent

Pinguva Agent source snapshots are published for transparency, security review, and evaluation. The Pinguva SaaS backend, web application, alerting infrastructure, and internal server-side logic are closed-source and are not part of this repository.

## Public issues

Public issues are appropriate for reproducible agent defects, documentation corrections, and build or compatibility problems that do not expose sensitive information.

Include:

- The published agent version.
- Operating system and architecture.
- A minimal, sanitized reproduction.
- Expected and actual behavior.

Do not include webhook URLs, API tokens, passwords, customer domains, private IP addresses, raw access logs, CRM data, configuration-file contents, or unredacted SQL.

## Security reports

Do not report vulnerabilities in a public issue. Follow the process in [SECURITY.md](./SECURITY.md) and contact Pinguva through the [official website](https://pinguva.com).

## Pull requests

Documentation improvements and narrowly scoped, reproducible agent fixes may be reviewed case by case. This repository is a release-source archive, so accepted work can be integrated through Pinguva's private development workflow and appear in a later public source snapshot rather than being merged directly as a standalone change.

Before opening a pull request:

- Limit the change to public agent code or documentation.
- Do not modify or attempt to recreate closed platform components.
- Run the relevant Go tests for the affected version directory.
- Do not add secrets, production configuration, customer data, or binary artifacts.
