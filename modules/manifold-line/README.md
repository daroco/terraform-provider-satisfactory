# manifold-line

One production line, the way the game is usually built: a row of identical
machines, a splitter manifold along each input, a merger chain on the output,
and a power pole per machine. Feed it a belt per input and it hands back the
merged output and the ends of its pole chain, so lines compose:

```hcl
module "rods" {
  source           = "../../modules/manifold-line"
  class            = "Build_ConstructorMk1_C"
  recipe           = "Recipe_IronRod_C"
  machines         = 5
  clock_speed      = 0.82
  origin           = { x = 0, y = 0 }
  machine_z        = local.machine_z
  attachment_z     = local.attachment_z
  input_connectors = [0]
  output_connector = 1
  feeds            = [{ from_id = satisfactory_building.ingot_splitter.id, from_connector = 0 }]
  power_feed       = { from_id = module.smelters.pole_last_id, from_connector = 0 }
}

module "screws" {
  source = "../../modules/manifold-line"
  # ...
  feeds      = [{ from_id = module.rods.output_id, from_connector = module.rods.output_connector }]
  power_feed = { from_id = module.rods.pole_last_id, from_connector = 0 }
}
```

## Ratios

An even split with back-pressure settles at demand: a full machine is
skipped and the surplus runs on down the chain. So the manifold works as long
as the feed carries at least the row's total consumption, and the last
machine fills up slower than the first - that is normal.

## Orientation

Machines go along +x. Which side the inputs face depends on the class and
`machine_yaw`; say so with `inputs_side` (see its description). The module
only knows the splitter/merger connector maps confirmed in
`examples/factory-hub`; which machine connector is which (`input_connectors`,
`output_connector`) is yours to pass, because it differs per class and is
found by hand - see "Calibration" in `examples/tier4-factory/README.md`.

With more than one input, the outer splitter rows' belts cross the inner
row. The game routes them; it is not pretty.

## What it does not do

- Check footprints: pick `pitch` from `data.satisfactory_buildable_class`.
- Lifts or multi-level anything.
- Decide power pole spacing beyond "one per machine": a long row at a wide
  pitch may exceed a Mk1 pole's reach - use `modules/pole-chain` to bridge.
