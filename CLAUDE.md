# terraform-provider-satisfactory

Factory-as-code for Satisfactory: a Go Terraform provider that talks to a
companion UE C++ mod ([SatisfactoryTerraform](https://github.com/daroco/SatisfactoryTerraform))
exposing a localhost REST API. `terraform apply` places foundations, machines,
belts and power lines in a running game session.

## Architecture in one paragraph

The provider (`internal/provider`) assigns each resource a UUID (`tf_id`) and
talks to the mod's HTTP API (contract: `api/openapi.yaml` — the single source
of truth; change it first, then client, mock, and mod together — the mod lives
in the SatisfactoryTerraform repo). The mod keeps a `tf_id → actor` registry
persisted in the save game, so plans stay clean across save/load, and in-game
dismantling shows up as drift (404 on Read → state removal → recreate plan).
`internal/mockserver` is an in-memory implementation of the same contract so
everything provider-side is developed and CI-gated without launching the game.
See `docs/architecture.md`.

## Commands

```sh
go build ./... && go vet ./...            # must stay clean
go test ./...                             # unit tests (api, mockserver, client)
TF_ACC=1 go test ./internal/provider/ -v  # acceptance tests (needs terraform in PATH
                                          #   or TF_ACC_TERRAFORM_PATH=<binary>)
go run ./cmd/mockserver                   # fake world on :8090
```

Full dev loop for applying the example against the mock: see the
`mock-stack` skill (`.claude/skills/mock-stack/`).

## Layout

- `api/openapi.yaml` — REST contract (source of truth)
- `internal/api` — wire types shared by client + mock
- `internal/client` — HTTP client; `NotFoundError`/`IsNotFound` is the drift signal
- `internal/mockserver` — in-memory mod API; mirror every contract change here
- `internal/provider` — terraform-plugin-framework provider; 4 resources
- `cmd/mockserver` — runnable mock
- `examples/iron-plate-line` — canonical minimal example; CI applies it against the mock
- `examples/factory-floor` — range/grid placement example (`modules/grid-2d`);
  every `examples/*` directory is applied/planned/destroyed in CI
- `examples/factory-hub` — hub-and-spoke layout (merger/splitter star topology)
- `modules/grid-2d` — reusable local module: bounding box + spacing → a
  `for_each`-ready map of positions. Pure HCL, no provider/mod changes.
- `docs/` — architecture, GitOps-factory design notes

The in-game half (UE C++ mod, its CI, and its self-hosted runner docs) lives
in [daroco/SatisfactoryTerraform](https://github.com/daroco/SatisfactoryTerraform).

## Conventions

- Resource identity is `tf_id` (provider-generated UUID), passed on create,
  persisted by the mod in the save. Never derive identity from position/class.
- Transform units are Unreal centimetres; `yaw` in degrees. Reads/creates
  absorb float32 round-trip noise via `preserveWithinEpsilon`
  (`internal/provider/resource_building.go`) — keep that guard on any new
  transform-carrying resource.
- Class/recipe names are opaque strings validated by the game at apply time
  (e.g. `Build_ConstructorMk1_C`, `Recipe_IronPlate_C`). Do not bake game
  content tables into the provider. (The mock carries one narrow, documented
  exception: a manufacturer-class prefix list that only shapes mock responses
  so CI reproduces the real mod's manufacturer-vs-not behavior.)
- Only `recipe` and `clock_speed` update in place; everything else
  `RequiresReplace`. Both fields are manufacturer-only: the mod omits them
  entirely for other classes and 422s PATCHes to them.
- Adding a resource or endpoint: follow the `add-resource` skill
  (`.claude/skills/add-resource/`).
- Error contract: 404 unknown tf_id, 409 duplicate tf_id, 422 validation.
  Delete is idempotent client-side (404 on DELETE is success).

## CI

- `provider-ci` (hosted): build, vet, tests, acceptance tests, and
  apply/plan(-detailed-exitcode)/destroy of every example against the mock.
  This must stay green; it is the merge gate.

## Status

v0.1.0-level functionality is complete and verified live end-to-end against a
running game session: all 4 resources, zero-drift plans across save/reload.
Next: registry release (GoReleaser + tfplugindocs + GPG signing), import
polish, GitOps mode (see docs/gitops-factory.md).
