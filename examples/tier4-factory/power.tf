# Coal power on the shore of the nearest lake, and the wires that bring it,
# and every miner, onto the factory grid.
#
# 8 x Build_GeneratorCoal_C = 600 MW for a factory that draws ~480 MW
# nominal (less in practice: draw scales with clock^1.32). Each generator
# burns 15 coal and 45 m3 of water a minute: 120 coal from one Mk2-on-normal
# node, 360 m3 from four water extractors (120 each). One extractor feeds
# two generators through a cross junction.
#
# The plant stands at lake level (+50 cm) so the extractors' 10 m head lift
# covers it with no pipeline pump; if a lake's shore is higher where you
# are, set var.plant_needs_pump and a Mk1 pump is added per feed.

variable "plant_needs_pump" {
  type    = bool
  default = false
}

locals {
  gens       = 8
  extractors = 4
  gen_pitch  = local.cal.coal_gen.pitch

  # Shore row just south (-y) of the lake's box, centred on it.
  plant_z      = local.water.z + 50
  plant_origin = { x = local.water.x - (local.gens - 1) * local.gen_pitch / 2, y = local.water.bounds_min.y - 3000 }
  plant_att_z  = local.plant_z + 100

  # Extractors spread along the lake's x, at its surface.
  extractor_x = { for i in range(local.extractors) : tostring(i) => local.water.x + (i - (local.extractors - 1) / 2) * 2200 }
  # A junction per extractor, between the water and the generator row,
  # above the two generators it serves.
  junction = { for i in range(local.extractors) : tostring(i) => {
    x = local.plant_origin.x + (2 * i + 0.5) * local.gen_pitch
    y = local.plant_origin.y + (local.cal.coal_gen.side == 1 ? 3 : -3) * local.row_gap
  } }
}

resource "satisfactory_extractor" "water" {
  for_each = local.extractor_x
  class    = "Build_WaterPump_C"
  node_id  = local.water.id
  x        = each.value
  y        = local.water.y
}

module "generators" {
  source           = "../../modules/manifold-line"
  class            = "Build_GeneratorCoal_C"
  machines         = local.gens
  origin           = local.plant_origin
  pitch            = local.gen_pitch
  row_gap          = local.row_gap
  machine_z        = local.plant_z
  attachment_z     = local.plant_att_z
  machine_yaw      = local.cal.coal_gen.yaw
  inputs_side      = local.cal.coal_gen.side
  input_connectors = local.cal.coal_gen.in
  has_output       = false
  belt_class       = var.belt_class
  feeds            = [{ from_id = satisfactory_extractor.miner["coal_b"].id, from_connector = local.miner_out }]
}

# --- Water: extractor -> junction -> two generators -------------------------

resource "satisfactory_building" "junction" {
  for_each = local.junction
  class    = "Build_PipelineJunction_Cross_C"
  x        = each.value.x
  y        = each.value.y
  z        = local.plant_att_z
}

resource "satisfactory_building" "pump" {
  for_each = var.plant_needs_pump ? local.junction : {}
  class    = "Build_PipelinePump_C"
  x        = each.value.x
  y        = (each.value.y + local.water.y) / 2
  z        = local.plant_att_z
}

# extractor -> (pump ->) junction
resource "satisfactory_pipeline" "supply" {
  for_each       = local.extractor_x
  class          = "Build_Pipeline_C"
  from_id        = satisfactory_extractor.water[each.key].id
  from_connector = local.water_pipe_out
  to_id          = var.plant_needs_pump ? satisfactory_building.pump[each.key].id : satisfactory_building.junction[each.key].id
  to_connector   = var.plant_needs_pump ? 0 : local.junction_conn.in
}

resource "satisfactory_pipeline" "pumped" {
  for_each       = var.plant_needs_pump ? local.extractor_x : {}
  class          = "Build_Pipeline_C"
  from_id        = satisfactory_building.pump[each.key].id
  from_connector = 1
  to_id          = satisfactory_building.junction[each.key].id
  to_connector   = local.junction_conn.in
}

# junction -> generators 2i and 2i+1
resource "satisfactory_pipeline" "to_generator" {
  for_each       = { for i in range(local.gens) : tostring(i) => i }
  class          = "Build_Pipeline_C"
  from_id        = satisfactory_building.junction[tostring(floor(each.value / 2))].id
  from_connector = local.junction_conn.outs[each.value % 2]
  to_id          = module.generators.machine_ids[each.key]
  to_connector   = local.coal_gen_pipe
}

# --- Wires -------------------------------------------------------------------

# Water extractors draw 20 MW each; the plant's own poles carry them (pole
# 0 is full: generator, next, coal miner chain, trunk).
resource "satisfactory_power_line" "water" {
  for_each       = local.extractor_x
  from_id        = satisfactory_extractor.water[each.key].id
  from_connector = local.pole_conn
  to_id          = module.generators.pole_ids[tostring(tonumber(each.key) + 1)]
  to_connector   = local.pole_conn
}

# Plant -> factory: a trunk of poles from the last generator pole to the
# first production line.
module "trunk" {
  source = "../../modules/pole-chain"
  from   = { x = local.plant_origin.x + local.gens * local.gen_pitch, y = local.plant_origin.y - local.cal.coal_gen.side * 2 * local.row_gap, z = local.plant_z }
  to     = { x = local.origin.iron_smelters.x - local.row_gap, y = local.origin.iron_smelters.y, z = local.machine_z }
  feed   = { from_id = module.generators.pole_last_id, from_connector = local.pole_conn }
  sink   = { to_id = module.iron_smelters.pole_first_id, to_connector = local.pole_conn }
}

# Each miner gets a chain of poles from its node to the first pole of the
# line it feeds. Chains start from the NODE's position (known at plan time),
# not the miner's computed one.
locals {
  miner_power = {
    iron_a    = { line = module.iron_smelters.pole_first_id, to = local.origin.iron_smelters }
    iron_b    = { line = module.foundries.pole_first_id, to = local.origin.foundries }
    coal_a    = { line = module.beams.pole_first_id, to = local.origin.beams }
    copper    = { line = module.copper_smelters.pole_first_id, to = local.origin.copper_smelters }
    limestone = { line = module.concrete.pole_first_id, to = local.origin.concrete }
    coal_b    = { line = module.generators.pole_first_id, to = local.plant_origin }
  }
}

module "miner_power" {
  source   = "../../modules/pole-chain"
  for_each = local.miner_power
  from     = { x = local.nodes[each.key].x, y = local.nodes[each.key].y, z = local.nodes[each.key].z }
  to       = { x = each.value.to.x - local.row_gap, y = each.value.to.y - local.row_gap, z = each.key == "coal_b" ? local.plant_z : local.machine_z }
  feed     = { from_id = satisfactory_extractor.miner[each.key].id, from_connector = local.pole_conn }
  sink     = { to_id = each.value.line, to_connector = local.pole_conn }
}

output "power" {
  value = {
    generators       = local.gens
    capacity_mw      = local.gens * 75
    water_extractors = local.extractors
    trunk_poles      = module.trunk.pole_count
  }
}
