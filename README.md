# integration-github

`integration-github` is the GitHub runtime and governance plugin for Yggdrasil. It
exposes GitHub operations through the generic `describe/execute` contract expected by
`yggdrasil-core`.

This plugin is the `github / operations / api` entry in the Yggdrasil plugin catalog.

## Operations

- `dispatch_workflow`
- `create_repository`
- `upsert_environment`
- `grant_team_repository_access`

The adapter exposes `describe` and `execute` over RabbitMQ. The core resolves the configured
`integration_instance`, forwards the request here, and the adapter calls the GitHub API.

This repository keeps its own local protocol types. The public wire contract is
documented in [/Users/dakasa/projects/yggdrasil-core/docs/contracts](/Users/dakasa/projects/yggdrasil-core/docs/contracts), not imported from `yggdrasil-core/model`.

## Repository shape

- [/Users/dakasa/projects/integration-github/main.go](/Users/dakasa/projects/integration-github/main.go): worker bootstrap
- [/Users/dakasa/projects/integration-github/controllers/message](/Users/dakasa/projects/integration-github/controllers/message): RabbitMQ RPC handlers
- [/Users/dakasa/projects/integration-github/internal/adapter/spec.go](/Users/dakasa/projects/integration-github/internal/adapter/spec.go): adapter contract and GitHub dispatch logic
- [/Users/dakasa/projects/integration-github/internal/adapter/spec_test.go](/Users/dakasa/projects/integration-github/internal/adapter/spec_test.go): adapter tests
- [/Users/dakasa/projects/integration-github/examples](/Users/dakasa/projects/integration-github/examples): example manifests for the core

## Queues

- `yggdrasil.adapter.github.describe`
- `yggdrasil.adapter.github.execute`

## Auth model

The adapter accepts the GitHub token in either place:

- `request.auth.token`: preferred during the transition from `yggdrasil-api`
- `integration_instance.spec.credentials.token`: good for service-managed credentials later

If both are present, the caller token wins.

## Config

Supported `integration_instance.spec.config` fields:

- `default_owner`
- `default_ref`
- `default_workflow`
- `default_visibility`
- `api_base_url`

`api_base_url` defaults to `https://api.github.com`, but it can point to GitHub Enterprise later.

## Environment

- `BROKER_URL`: RabbitMQ connection string used by the worker itself

## Running

```bash
go run .
```

## Validation

```bash
go mod tidy
go test ./...
```
