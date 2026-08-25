# Contributing

Thanks for your interest in improving `terraform-provider-satisfactory`.

## Ground rules

- The mod⇄provider REST contract in [`api/openapi.yaml`](api/openapi.yaml) is
  the single source of truth. Change it first, then the client
  (`internal/client`), the in-memory mock (`internal/mockserver`), and the
  companion mod ([daroco/SatisfactoryTerraform](https://github.com/daroco/SatisfactoryTerraform))
  together.
- Resource identity is the provider-generated `tf_id` (a UUID). Never derive
  identity from a buildable's position or class.
- Only `recipe` and `clock_speed` update in place; every other attribute is
  `RequiresReplace`.
- Keep everything provider-side developable without the game: the mockserver
  mirrors the contract so `go test ./...` and CI never launch Satisfactory.

## Development environment

You need Go (see the version in [`go.mod`](go.mod)) and, for the acceptance
tests and doc generation, the Terraform CLI on your `PATH`.

```sh
make build      # go build ./...
make test       # go vet + unit tests
make fmt        # gofmt -s -w
make lint       # golangci-lint run
make docs       # regenerate docs/ with tfplugindocs (needs terraform)
```

Before opening a pull request, make sure the following all pass:

```sh
gofmt -l .                 # no output
go vet ./...
go test ./...
```

If you touched a resource schema, its provider metadata, or an example under
`examples/`, regenerate the registry docs and commit the result:

```sh
make docs
```

## Running the provider by hand

Develop against the in-memory mock instead of a live game (no Satisfactory
install required):

```sh
go run ./cmd/mockserver &                  # fake world on :8090
go build -o /tmp/terraform-provider-satisfactory .

cat > /tmp/dev.tfrc <<'EOF'
provider_installation {
  dev_overrides { "daroco/satisfactory" = "/tmp" }
  direct {}
}
EOF

cd examples/iron-plate-line
TF_CLI_CONFIG_FILE=/tmp/dev.tfrc terraform apply
```

The acceptance tests exercise the resources end to end against that same mock
and need a Terraform binary:

```sh
TF_ACC=1 go test ./internal/provider/ -v
```

## Adding a resource or endpoint

Follow the `add-resource` skill in `.claude/skills/add-resource/`: it walks
the openapi → client → mock → provider → docs change in the right order.

## Pull requests

- Keep commits focused and logically grouped, with a clear subject line.
- CI (`provider-ci`) must stay green: build, `go vet`, `gofmt`,
  `golangci-lint`, unit tests, acceptance tests, and an apply/plan/destroy of
  every runnable example against the mock. It is the merge gate.
- Update [`CHANGELOG.md`](CHANGELOG.md) under the `Unreleased` heading for any
  user-visible change.

## Reporting bugs and requesting features

Use the issue templates. For security-sensitive reports, see
[`SECURITY.md`](SECURITY.md) instead of opening a public issue.
