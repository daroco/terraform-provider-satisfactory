---
name: mock-stack
description: Run the full local dev loop - mockserver + locally built provider + terraform apply of an example - without launching the game. Use when asked to run, demo, or manually verify the provider, or to reproduce a provider-ci failure locally.
---

# Mock stack dev loop

Everything runs against `internal/mockserver`; no game needed.

## What this loop cannot tell you

It exercises the provider, not the mod. A clean run here means the provider is
self-consistent - it does **not** mean the change works in a game. The mock is
a Go map; the real implementation lives inside Unreal, and the gap has hidden
every serious bug in this project:

- The game converts foundations to lightweight instances **synchronously**
  during spawn, and recycles instance slots on removal. The mock has no such
  concept, so it cannot reproduce leaked instances, stale references, or
  identity collisions - and a leaked instance is invisible to the API anyway.
- Transforms round-trip through float32 in the game and not in the mock, so
  quantization drift never appears here.
- Wherever the mock is more lenient than the mod, this loop goes **green on
  configs that fail live**. Keeping the two aligned is a hard requirement, not
  a nicety - see the `add-resource` skill.

Use this loop for fast iteration and CI parity. For anything the mod
implements, follow it with a live pass (`live-verify` skill, mod repo).

```sh
# 1. Build both binaries (provider binary name matters for dev_overrides)
go build -o /tmp/satisfacto/terraform-provider-satisfactory .
go build -o /tmp/satisfacto/mockserver ./cmd/mockserver

# 2. Start the fake world and wait for health
/tmp/satisfacto/mockserver -addr :8090 &
until curl -fsS http://localhost:8090/api/v1/health; do sleep 1; done

# 3. Point terraform at the local provider build (skips init/registry)
cat > /tmp/satisfacto/dev.tfrc <<EOF
provider_installation {
  dev_overrides { "daroco/satisfactory" = "/tmp/satisfacto" }
  direct {}
}
EOF
export TF_CLI_CONFIG_FILE=/tmp/satisfacto/dev.tfrc

# 4. Apply / verify / destroy
cd examples/iron-plate-line
terraform get       # only needed for examples with local `module` blocks
                     # (e.g. factory-floor) - installs them without touching
                     # providers/backend
terraform apply -auto-approve
terraform plan -detailed-exitcode   # exit 0 = clean, 2 = drift, 1 = error
terraform destroy -auto-approve
```

Notes:

- With `dev_overrides` do NOT run `terraform init` - it actively fails
  (queries the registry for daroco/satisfactory, which doesn't exist there).
  Use `terraform get` instead if the example has local `module` blocks; it
  only installs modules, no provider/backend involved. Remove any stale
  `.terraform.lock.hcl` if terraform complains.
- If no `terraform` binary is on PATH, download one from
  releases.hashicorp.com (any 1.5+ works) and put it on PATH; acceptance tests
  accept `TF_ACC_TERRAFORM_PATH=<binary>` instead.
- To poke the API directly: `curl localhost:8090/api/v1/buildables`; the
  contract is `api/openapi.yaml`.
- To simulate in-game dismantling (drift): `curl -X DELETE
  localhost:8090/api/v1/buildables/<tf_id>` then `terraform plan` — it must
  propose recreation.
- Never commit `terraform.tfstate*` or lockfiles from example runs
  (gitignored, but clean up anyway).
- This mirrors the "Example applies against mockserver" step in
  `.github/workflows/provider-ci.yml`; keep the two in sync.
- `iron-plate-line` is the simplest walkthrough; every directory under
  `examples/` is applied/planned/destroyed the same way in CI (a bash loop
  over `examples/*/`), so the same steps 1-4 work unchanged for any of them
  — swap the `cd` target. See `examples/factory-floor` for the grid/range
  placement pattern (`modules/grid-2d`).
