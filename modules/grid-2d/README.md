# grid-2d

Computes a 2D grid of `{x, y}` positions (centimetres) from a bounding box and
a fixed pitch, for use with a resource's `for_each`. Pure Terraform - no
provider dependency, works with `terraform validate` on its own.

```hcl
module "floor" {
  source  = "../../modules/grid-2d"
  from    = { x = 0, y = 0 }
  to      = { x = 3200, y = 1600 }
  spacing = 800 # one Build_Foundation_8x4_01_C tile
}

resource "satisfactory_foundation" "floor" {
  for_each = module.floor.positions
  class    = "Build_Foundation_8x4_01_C"
  x        = each.value.x
  y        = each.value.y
  z        = local.base_z
}
```

The same module works for spacing out repeated buildings - just pass a wider
`spacing` and feed the output into a `satisfactory_building` resource
instead. See `examples/factory-floor` for both used together.

## Why `for_each` with string keys, not `count`

Cells are keyed by a stable `"ix_iy"` string, not a list index. With `count`,
shrinking or growing a grid renumbers every cell after the change point,
which Terraform reads as "destroy and recreate" for anything downstream of
it - including in-game buildables that would get dismantled and rebuilt for
no reason. With `for_each` and stable keys, changing the bounding box only
touches the cells that actually appear or disappear; every other cell's key
is unchanged, so its `satisfactory_foundation`/`satisfactory_building`
resource is left alone.

## Scope

- 2D only (one `z` level per module call). Stacking multiple floors is the
  same pattern with one more nested loop over a `z` range - not built here,
  since it wasn't needed yet.
- `spacing` is a plain number the module takes on faith - it still has no
  idea how big a building is. But you can now feed it a real measurement
  instead of a guess: `data.satisfactory_buildable_class.<name>.size.x`
  reports a class's clearance footprint, read from the game without placing
  anything. See the provider README for the pattern.
- Footprint-aware auto-packing (given a class, work out how many fit) is now
  possible to build on that data, but is deliberately not done here - it would
  turn a pure-HCL module into something that needs a live game to plan.
