# A production line as the game is usually built: a row of identical
# machines, a manifold of splitters along each input, a chain of mergers on
# the output, and a power pole per machine. Belts are routed by the game
# between connectors, so this only has to put the attachments where the
# connectors can reach them.
#
# Geometry, for inputs_side = 1 (input rows on +y):
#
#   y = origin.y + 2*row_gap   splitter row for input 1   (assemblers)
#   y = origin.y + 1*row_gap   splitter row for input 0
#   y = origin.y               machines, along +x at `pitch`
#   y = origin.y - 1*row_gap   merger row
#   y = origin.y - 2*row_gap   power poles
#
# Connector maps (confirmed live, see examples/factory-hub):
#   Splitter at yaw 0: 0 = out +x, 1 = in -x, 2 = out +y, 3 = out -y
#   Merger   at yaw 0: 0 = in -x,  1 = out +x, 2 = in +y, 3 = in -y
# A splitter chain runs +x: feed -> s[0].1, s[i].0 -> s[i+1].1, and the
# machine hangs off the side facing it. Same for mergers, reversed.

terraform {
  required_providers {
    satisfactory = {
      source = "daroco/satisfactory"
    }
  }
}

locals {
  n      = var.machines
  inputs = length(var.input_connectors)

  machine_keys = { for i in range(local.n) : tostring(i) => i }
  cell_keys    = { for c in setproduct(range(local.inputs), range(local.n)) : "${c[0]}_${c[1]}" => { k = c[0], i = c[1] } }

  machine_x = { for i in range(local.n) : i => var.origin.x + i * var.pitch }

  # Splitter rows stack outward from the machines, one per input.
  splitter_y = { for k in range(local.inputs) : k => var.origin.y + var.inputs_side * (k + 1) * var.row_gap }
  merger_y   = var.origin.y - var.inputs_side * var.row_gap
  pole_y     = var.origin.y - var.inputs_side * 2 * var.row_gap

  # The side output of a splitter that faces the machines, and the side
  # input of a merger that faces them. Inputs on +y means the machines are
  # at -y of the splitters (connector 3) and at +y of the mergers (2).
  splitter_to_machine = var.inputs_side == 1 ? 3 : 2
  merger_from_machine = var.inputs_side == 1 ? 2 : 3

  has_poles = var.pole_class != ""
}

resource "satisfactory_building" "machine" {
  for_each    = local.machine_keys
  class       = var.class
  recipe      = var.recipe
  clock_speed = var.clock_speed
  x           = local.machine_x[each.value]
  y           = var.origin.y
  z           = var.machine_z
  yaw         = var.machine_yaw
}

# --- Inputs: a splitter per machine per input stream -----------------------

resource "satisfactory_building" "splitter" {
  for_each = local.cell_keys
  class    = var.splitter_class
  x        = local.machine_x[each.value.i]
  y        = local.splitter_y[each.value.k]
  z        = var.attachment_z
}

# feed -> first splitter of each stream (its -x input)
resource "satisfactory_belt" "feed" {
  for_each       = { for k in range(local.inputs) : tostring(k) => k }
  class          = var.belt_class
  from_id        = var.feeds[each.value].from_id
  from_connector = var.feeds[each.value].from_connector
  to_id          = satisfactory_building.splitter["${each.value}_0"].id
  to_connector   = 1
}

# splitter[i] -> splitter[i+1] along the row
resource "satisfactory_belt" "splitter_chain" {
  for_each       = { for key, c in local.cell_keys : key => c if c.i < local.n - 1 }
  class          = var.belt_class
  from_id        = satisfactory_building.splitter[each.key].id
  from_connector = 0
  to_id          = satisfactory_building.splitter["${each.value.k}_${each.value.i + 1}"].id
  to_connector   = 1
}

# splitter[k][i] -> machine[i] input k
resource "satisfactory_belt" "input" {
  for_each       = local.cell_keys
  class          = var.belt_class
  from_id        = satisfactory_building.splitter[each.key].id
  from_connector = local.splitter_to_machine
  to_id          = satisfactory_building.machine[tostring(each.value.i)].id
  to_connector   = var.input_connectors[each.value.k]
}

# --- Output: a merger per machine, chained ---------------------------------

resource "satisfactory_building" "merger" {
  for_each = var.has_output ? local.machine_keys : {}
  class    = var.merger_class
  x        = local.machine_x[each.value]
  y        = local.merger_y
  z        = var.attachment_z
}

# machine[i] -> merger[i] side input
resource "satisfactory_belt" "output" {
  for_each       = var.has_output ? local.machine_keys : {}
  class          = var.belt_class
  from_id        = satisfactory_building.machine[each.key].id
  from_connector = var.output_connector
  to_id          = satisfactory_building.merger[each.key].id
  to_connector   = local.merger_from_machine
}

# merger[i] -> merger[i+1]
resource "satisfactory_belt" "merger_chain" {
  for_each       = var.has_output ? { for key, i in local.machine_keys : key => i if i < local.n - 1 } : {}
  class          = var.belt_class
  from_id        = satisfactory_building.merger[each.key].id
  from_connector = 1
  to_id          = satisfactory_building.merger[tostring(each.value + 1)].id
  to_connector   = 0
}

# last merger -> sink, when given
resource "satisfactory_belt" "sink" {
  count          = var.has_output && var.sink != null ? 1 : 0
  class          = var.belt_class
  from_id        = satisfactory_building.merger[tostring(local.n - 1)].id
  from_connector = 1
  to_id          = var.sink.to_id
  to_connector   = var.sink.to_connector
}

# --- Power: a pole per machine, chained, fed from power_feed ---------------

resource "satisfactory_building" "pole" {
  for_each = local.has_poles ? local.machine_keys : {}
  class    = var.pole_class
  x        = local.machine_x[each.value]
  y        = local.pole_y
  z        = var.machine_z
}

resource "satisfactory_power_line" "machine" {
  for_each       = local.has_poles ? local.machine_keys : {}
  from_id        = satisfactory_building.machine[each.key].id
  from_connector = var.power_connector
  to_id          = satisfactory_building.pole[each.key].id
  to_connector   = 0
}

resource "satisfactory_power_line" "pole_chain" {
  for_each       = local.has_poles ? { for key, i in local.machine_keys : key => i if i < local.n - 1 } : {}
  from_id        = satisfactory_building.pole[each.key].id
  from_connector = 0
  to_id          = satisfactory_building.pole[tostring(each.value + 1)].id
  to_connector   = 0
}

resource "satisfactory_power_line" "feed" {
  count          = local.has_poles && var.power_feed != null ? 1 : 0
  from_id        = var.power_feed.from_id
  from_connector = var.power_feed.from_connector
  to_id          = satisfactory_building.pole["0"].id
  to_connector   = 0
}
