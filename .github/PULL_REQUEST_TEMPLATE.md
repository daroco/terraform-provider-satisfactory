<!--
Thanks for contributing! Please fill in the sections below and delete this
comment. See CONTRIBUTING.md for the local checks CI will re-run.
-->

## Description

<!-- What does this change do, and why? -->

Fixes # <!-- issue number, if any -->

## Type of change

- [ ] Bug fix
- [ ] New resource or endpoint
- [ ] Enhancement to an existing resource
- [ ] Documentation only
- [ ] CI / build / tooling

## Checklist

- [ ] `gofmt -l .` reports no files
- [ ] `go vet ./...` and `go test ./...` pass
- [ ] If a resource schema, provider metadata, or an `examples/` file changed,
      I ran `make docs` and committed the regenerated `docs/`
- [ ] If the mod⇄provider contract changed, I updated `api/openapi.yaml`, the
      client, and the mockserver together
- [ ] I updated `CHANGELOG.md` under `Unreleased` for any user-visible change
