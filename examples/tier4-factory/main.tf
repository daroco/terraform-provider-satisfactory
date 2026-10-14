# Every automatable part through Tier 4, from ore to storage, coal-powered.
#
# Tier 0-2: Iron Ingot, Iron Plate, Iron Rod, Screw, Reinforced Iron Plate,
#           Copper Ingot, Wire, Cable, Concrete, Copper Sheet, Rotor,
#           Modular Frame, Smart Plating
# Tier 3-4: Steel Ingot, Steel Beam, Steel Pipe, Versatile Framework,
#           Encased Industrial Beam, Stator, Motor, Automated Wiring
#
# Nothing here is a coordinate someone copied out of the game. Miners and
# water extractors bind to nodes found with the satisfactory_resource_nodes
# data source, nearest the anchor first; the factory floor is laid out
# relative to the anchor; the coal plant is laid out relative to the lake.
# Move var.anchor and the whole thing moves.
#
# Production math (per minute, all clocks <= 1.0, every belt <= 120/min so
# Mk2 belts carry everything) is in README.md. The layout is a strip of
# production lines (modules/manifold-line), two columns, stacked along +y.
#
# Local dev against the mock: see the mock-stack skill. Live: README.md.

terraform {
  required_providers {
    satisfactory = {
      source = "daroco/satisfactory"
    }
  }
}

provider "satisfactory" {
  # endpoint defaults to http://localhost:8090 (or SATISFACTORY_ENDPOINT).
  # preview = true shows the whole plan as holograms in-game first.
}

variable "anchor" {
  type        = object({ x = number, y = number, z = number })
  description = "Where the factory floor starts (its south-west corner), in centimetres; also the point nodes are picked nearest to. The floor runs about 180 m east and 540 m north of it. Default: open ground east of the Grasslands HUB, clear of examples/factory-hub and of the lake the coal plant uses."
  default     = { x = -74000, y = 224000, z = -337 }
}

variable "floor" {
  type        = bool
  description = "Lay 1 m foundations under every line. Off by default: it is ~1300 tiles, and the API places machines fine without them. Turn on for a factory you will walk around in."
  default     = false
}

variable "bootstrap_biomass_burner" {
  type        = bool
  description = "Place one biomass burner on the grid so the miners can start before the coal plant has coal. Fill it by hand once; after that the plant carries itself. Not needed with the 'No Power' advanced game setting."
  default     = true
}

variable "belt_class" {
  type    = string
  default = "Build_ConveyorBeltMk2_C"
}

# --- Calibration -------------------------------------------------------------
#
# Connector indices and orientations the game does not document. Each entry
# says how it was found. "live" = confirmed in a running game (see
# examples/factory-hub and the mod's implementation notes); "assumed" = the
# natural guess, to be confirmed with the calibration procedure in README.md
# before trusting the result. The module only needs `side` to be right for
# belts to reach; a wrong index fails loudly at apply (422), a wrong side
# produces belts that go around the machine.
locals {
  cal = {
    # inputs on which side of the machine row at yaw 0 (+1 = +y, -1 = -y)
    smelter     = { in = [0], out = 1, side = -1, yaw = 0, pitch = 1000 }    # in/out live; side live (output faces +y)
    constructor = { in = [0], out = 1, side = 1, yaw = 0, pitch = 1000 }     # in live (faces +y); out assumed
    foundry     = { in = [0, 1], out = 2, side = -1, yaw = 0, pitch = 1200 } # live: inputs 0 (+x half) and 1 (-x half) face -y, output 2 faces +y
    assembler   = { in = [1, 2], out = 0, side = -1, yaw = 0, pitch = 1600 } # live: inputs 1 (+x half) and 2 (-x half) face -y, output 0 faces +y
    coal_gen    = { in = [0], side = 1, yaw = 0, pitch = 1200 }              # assumed; belt input only
  }
  miner_out      = 0 # assumed: a miner has one factory connector
  water_pipe_out = 0 # live: the water extractor's only pipe connector
  coal_gen_pipe  = 0 # live: the coal generator's only pipe connector
  # Junction: 0 and 2 are opposite ends and 1 the side branch (live, T junction);
  # the cross adds 3 opposite 1 (assumed). At yaw -90 connector 2 faces +y (the
  # lake) and 1/3 face -x/+x, the two generators it serves.
  junction_conn = { in = 2, outs = [1, 3], yaw = -90 }
  # Storage (live): input 0 faces -y and output 1 faces +y at yaw 0. The sinks
  # sit at the +x end of a merger chain, so yaw -90 turns the input toward it.
  storage   = { in = 0, out = 1, yaw = -90 }
  pole_conn = 0 # live: poles and machines have one power connector

  # Vertical offsets (live, examples/factory-hub): machines sit half a tile
  # above a 1 m foundation's pivot; splitters/mergers 1 m above that.
  base_z       = var.anchor.z
  machine_z    = local.base_z + 50
  attachment_z = local.machine_z + 100
}

# --- Nodes -------------------------------------------------------------------
#
# Every query includes occupied nodes and then keeps the ones that are free
# OR already ours: once a miner stands on a node that node must keep its
# place in the list, or the next plan would pick the next free node and
# replace the miner. Impure nodes are skipped (a Mk2 on impure is only 60/min).

data "satisfactory_resource_nodes" "iron" {
  resource         = "Desc_OreIron_C"
  x                = var.anchor.x
  y                = var.anchor.y
  z                = var.anchor.z
  include_occupied = true
}

data "satisfactory_resource_nodes" "copper" {
  resource         = "Desc_OreCopper_C"
  x                = var.anchor.x
  y                = var.anchor.y
  z                = var.anchor.z
  include_occupied = true
}

data "satisfactory_resource_nodes" "limestone" {
  resource         = "Desc_Stone_C"
  x                = var.anchor.x
  y                = var.anchor.y
  z                = var.anchor.z
  include_occupied = true
}

data "satisfactory_resource_nodes" "coal" {
  resource         = "Desc_Coal_C"
  x                = var.anchor.x
  y                = var.anchor.y
  z                = var.anchor.z
  include_occupied = true
}

data "satisfactory_resource_nodes" "water" {
  resource = "Desc_Water_C"
  x        = var.anchor.x
  y        = var.anchor.y
  z        = var.anchor.z
}

locals {
  usable = { for name, ds in {
    iron      = data.satisfactory_resource_nodes.iron
    copper    = data.satisfactory_resource_nodes.copper
    limestone = data.satisfactory_resource_nodes.limestone
    coal      = data.satisfactory_resource_nodes.coal
    } : name => [for n in ds.nodes : n if n.purity != "impure" && (!n.occupied || n.occupant_tf_id != "")]
  }
  nodes = {
    iron_a    = local.usable.iron[0]      # iron smelters (93/min)
    iron_b    = local.usable.iron[1]      # foundries (98.25/min)
    copper    = local.usable.copper[0]    # copper smelters (88/min)
    limestone = local.usable.limestone[0] # concrete (45/min)
    coal_a    = local.usable.coal[0]      # foundries (98.25/min)
    coal_b    = local.usable.coal[1]      # coal plant (120/min)
  }
  water = data.satisfactory_resource_nodes.water.nodes[0]

  # A Mk1 on a pure node and a Mk2 on a normal node both give 120/min, the
  # Mk2 belt's limit. A Mk2 on pure would make 240 and waste half.
  miner_class = { pure = "Build_MinerMk1_C", normal = "Build_MinerMk2_C" }
}

resource "satisfactory_extractor" "miner" {
  for_each = local.nodes
  class    = local.miner_class[each.value.purity]
  node_id  = each.value.id
}

# --- Layout ------------------------------------------------------------------
#
# Lines stack along +y in two columns. A slot is five rows (the most a line
# needs: two splitter rows, machines, mergers, poles) plus one row of air.
# Within a slot the line's origin is offset so that whichever rows it puts
# on -y still start at the slot's bottom edge.

locals {
  row_gap      = 800
  slot_pitch   = 6 * local.row_gap
  column_pitch = 12000

  # name => { class key, column, slot, machines, inputs }
  layout = {
    # column 0: iron
    iron_smelters  = { cal = "smelter", col = 0, slot = 0, machines = 4 }
    rods           = { cal = "constructor", col = 0, slot = 1, machines = 5 }
    screws         = { cal = "constructor", col = 0, slot = 2, machines = 4 }
    plates         = { cal = "constructor", col = 0, slot = 3, machines = 2 }
    rip            = { cal = "assembler", col = 0, slot = 4, machines = 1 }
    modular_frames = { cal = "assembler", col = 0, slot = 5, machines = 1 }
    rotors         = { cal = "assembler", col = 0, slot = 6, machines = 1 }
    smart_plating  = { cal = "assembler", col = 0, slot = 7, machines = 1 }
    # column 0: copper
    copper_smelters = { cal = "smelter", col = 0, slot = 8, machines = 3 }
    wire            = { cal = "constructor", col = 0, slot = 9, machines = 5 }
    cable           = { cal = "constructor", col = 0, slot = 10, machines = 2 }
    # column 1: steel and the Tier 4 assemblies
    copper_sheet     = { cal = "constructor", col = 1, slot = 0, machines = 1 }
    foundries        = { cal = "foundry", col = 1, slot = 1, machines = 3 }
    beams            = { cal = "constructor", col = 1, slot = 2, machines = 2 }
    pipes            = { cal = "constructor", col = 1, slot = 3, machines = 1 }
    stators          = { cal = "assembler", col = 1, slot = 4, machines = 1 }
    motors           = { cal = "assembler", col = 1, slot = 5, machines = 1 }
    automated_wiring = { cal = "assembler", col = 1, slot = 6, machines = 1 }
    concrete         = { cal = "constructor", col = 1, slot = 7, machines = 1 }
    eib              = { cal = "assembler", col = 1, slot = 8, machines = 1 }
    versatile_frame  = { cal = "assembler", col = 1, slot = 9, machines = 1 }
  }

  origin = { for name, l in local.layout : name => {
    x = var.anchor.x + l.col * local.column_pitch
    y = var.anchor.y + l.slot * local.slot_pitch + (local.cal[l.cal].side == 1 ? 2 * local.row_gap : length(local.cal[l.cal].in) * local.row_gap)
  } }
  # Where a line's merger row is, and where its last machine stands: the
  # distribution splitter / storage for its output goes just past that.
  merger_y = { for name, l in local.layout : name => local.origin[name].y - local.cal[l.cal].side * local.row_gap }
  after_x  = { for name, l in local.layout : name => local.origin[name].x + (l.machines - 1) * local.cal[l.cal].pitch + 1200 }
}

# --- Floor (optional) --------------------------------------------------------

module "floor" {
  source   = "../../modules/grid-2d"
  for_each = var.floor ? toset(["0", "1"]) : toset([])
  from     = { x = var.anchor.x - local.row_gap, y = var.anchor.y - 2 * local.row_gap }
  to       = { x = var.anchor.x + 8 * local.row_gap, y = var.anchor.y + 11 * local.slot_pitch }
  spacing  = local.row_gap
}

resource "satisfactory_foundation" "floor" {
  for_each = var.floor ? merge([for col in ["0", "1"] : {
    for k, p in module.floor[col].positions : "${col}_${k}" => { x = p.x + tonumber(col) * local.column_pitch, y = p.y }
  }]...) : {}
  class = "Build_Foundation_8x1_01_C"
  x     = each.value.x
  y     = each.value.y
  z     = local.base_z
}

# --- Bootstrap power ---------------------------------------------------------

resource "satisfactory_building" "biomass_burner" {
  count = var.bootstrap_biomass_burner ? 1 : 0
  class = "Build_GeneratorBiomass_Automated_C"
  x     = var.anchor.x - 2000
  y     = var.anchor.y
  z     = local.machine_z
}

resource "satisfactory_power_line" "biomass_burner" {
  count          = var.bootstrap_biomass_burner ? 1 : 0
  from_id        = satisfactory_building.biomass_burner[0].id
  from_connector = local.pole_conn
  to_id          = module.iron_smelters.pole_first_id
  to_connector   = local.pole_conn
}

output "nodes" {
  value       = { for k, n in local.nodes : k => { id = n.id, purity = n.purity, miner = local.miner_class[n.purity] } }
  description = "Which nodes the factory sits on."
}

output "water" {
  value = { id = local.water.id, x = local.water.x, y = local.water.y, z = local.water.z }
}
