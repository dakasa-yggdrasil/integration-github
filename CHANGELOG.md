# Changelog

All notable changes to integration-github are documented here.

## [Unreleased]

### Added
- `set_container_package_visibility` operation — flip ghcr/container packages to public/private/internal via the GitHub Packages API (`PATCH /{ownerType}/{owner}/packages/container/{name}`). Accepts both Yggdrasil singular (`org`/`user`) and GitHub plural (`orgs`/`users`) ownerTypes — normalized internally.

### Changed
- Adopt `yggdrasil-sdk-go` adapter builder. Transport selectable via `YGGDRASIL_TRANSPORT` (`http` default, `amqp` opt-in). HTTP listener on `ADAPTER_PORT` (default 8081); health probes on `HEALTHCHECK_PORT`.
- `Describe()` returns `transport=http_json` with `/rpc/describe` and `/rpc/execute` endpoints when on HTTP; queues only emitted under AMQP.
- `instance_schema` declares `base_url` (matches kubernetes adapter contract).
- All operations now send `X-GitHub-Api-Version: 2022-11-28` header.
- Type `SimpleStatusResponse` renamed to `AdapterOperationStatusResponse` for consistency.

### Removed
- `controllers/message/consume.go` and `register.go` — obsoleted by SDK.
