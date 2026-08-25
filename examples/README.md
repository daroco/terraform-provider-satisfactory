# Examples

This directory holds two kinds of examples.

## Documentation snippets (used by tfplugindocs)

`make docs` embeds these single-purpose files into the generated registry docs.
The generator only looks in these fixed locations; every other `*.tf` here is
ignored by it.

- `provider/provider.tf` — the provider configuration on the index page.
- `resources/<resource name>/resource.tf` — the example on each resource page.
- `resources/<resource name>/import.sh` — the `terraform import` example.

## Runnable end-to-end configs

These are complete, applyable configurations. CI applies, plans, and destroys
each of them against the in-memory mock (they are the directories with a
`main.tf`).

- `iron-plate-line/` — the canonical minimal factory: a foundation pad, a
  smelter feeding a constructor by belt, and a power line.
- `factory-floor/` — tiling a bounding box into a grid with the `grid-2d`
  module.
- `factory-hub/` — a hub-and-spoke merger/splitter star topology.
