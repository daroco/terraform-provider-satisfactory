# pole-chain

A straight line of power poles from one point to another, wired in series,
with optional wires into the first pole and out of the last. For bringing
a miner on a distant node, or a coal plant on the shore, onto the factory
grid.

```hcl
module "iron_a_power" {
  source = "../../modules/pole-chain"
  # From the NODE's position (known at plan time), not the extractor's
  # computed x/y/z: the number of poles depends on the distance, and a
  # for_each cannot depend on a value that is only known after apply.
  from   = { x = local.iron_a.x, y = local.iron_a.y, z = local.iron_a.z }
  to     = { x = local.anchor.x, y = local.anchor.y, z = local.machine_z }
  feed   = { from_id = satisfactory_extractor.iron_a.id, from_connector = 0 }
  sink   = { to_id = satisfactory_building.trunk.id, to_connector = 0 }
}
```

`max_span` defaults to 70 m, roughly a Mk1 pole's reach. The API does not
enforce wire length, so a longer span still applies; it just would not have
been buildable by hand.

Poles are interpolated in a straight line including z, so a chain down a
hillside steps with the terrain only as far as its two ends do. Nothing here
reads the ground.
