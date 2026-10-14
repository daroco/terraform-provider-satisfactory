# Production lines, in flow order, each a modules/manifold-line. Between
# them, plain splitters fan one line's output to the lines that consume it;
# a splitter has three outputs and this factory never needs more than that
# from one stream. Back-pressure settles uneven demand: a full branch is
# skipped and the surplus continues.
#
# Splitter at yaw 0 (live): 0 = out +x, 1 = in -x, 2 = out +y, 3 = out -y.
# Each distribution splitter stands just past its source line's last merger,
# so the merger's +x output runs straight into the splitter's -x input.

# --- Iron -------------------------------------------------------------------

module "iron_smelters" {
  source           = "../../modules/manifold-line"
  class            = "Build_SmelterMk1_C"
  recipe           = "Recipe_IngotIron_C"
  machines         = local.layout.iron_smelters.machines
  clock_speed      = 0.775 # 93/min from a 120/min node
  origin           = local.origin.iron_smelters
  pitch            = local.cal.smelter.pitch
  row_gap          = local.row_gap
  machine_z        = local.machine_z
  attachment_z     = local.attachment_z
  machine_yaw      = local.cal.smelter.yaw
  inputs_side      = local.cal.smelter.side
  input_connectors = local.cal.smelter.in
  output_connector = local.cal.smelter.out
  belt_class       = var.belt_class
  feeds            = [{ from_id = satisfactory_extractor.miner["iron_a"].id, from_connector = local.miner_out }]
  # First line on the grid: fed by the trunk from the coal plant (power.tf).
}

resource "satisfactory_building" "split_iron" {
  class = "Build_ConveyorAttachmentSplitter_C"
  x     = local.after_x.iron_smelters
  y     = local.merger_y.iron_smelters
  z     = local.attachment_z
}

resource "satisfactory_belt" "iron_to_split" {
  class          = var.belt_class
  from_id        = module.iron_smelters.output_id
  from_connector = module.iron_smelters.output_connector
  to_id          = satisfactory_building.split_iron.id
  to_connector   = 1
}

module "rods" {
  source           = "../../modules/manifold-line"
  class            = "Build_ConstructorMk1_C"
  recipe           = "Recipe_IronRod_C"
  machines         = local.layout.rods.machines
  clock_speed      = 0.82 # 61.5/min
  origin           = local.origin.rods
  pitch            = local.cal.constructor.pitch
  row_gap          = local.row_gap
  machine_z        = local.machine_z
  attachment_z     = local.attachment_z
  machine_yaw      = local.cal.constructor.yaw
  inputs_side      = local.cal.constructor.side
  input_connectors = local.cal.constructor.in
  output_connector = local.cal.constructor.out
  belt_class       = var.belt_class
  feeds            = [{ from_id = satisfactory_building.split_iron.id, from_connector = 0 }]
  power_feed       = { from_id = module.iron_smelters.pole_last_id, from_connector = local.pole_conn }
}

resource "satisfactory_building" "split_rods" {
  class = "Build_ConveyorAttachmentSplitter_C"
  x     = local.after_x.rods
  y     = local.merger_y.rods
  z     = local.attachment_z
}

resource "satisfactory_belt" "rods_to_split" {
  class          = var.belt_class
  from_id        = module.rods.output_id
  from_connector = module.rods.output_connector
  to_id          = satisfactory_building.split_rods.id
  to_connector   = 1
}

module "screws" {
  source           = "../../modules/manifold-line"
  class            = "Build_ConstructorMk1_C"
  recipe           = "Recipe_Screw_C"
  machines         = local.layout.screws.machines
  clock_speed      = 0.8875 # 142/min
  origin           = local.origin.screws
  pitch            = local.cal.constructor.pitch
  row_gap          = local.row_gap
  machine_z        = local.machine_z
  attachment_z     = local.attachment_z
  machine_yaw      = local.cal.constructor.yaw
  inputs_side      = local.cal.constructor.side
  input_connectors = local.cal.constructor.in
  output_connector = local.cal.constructor.out
  belt_class       = var.belt_class
  feeds            = [{ from_id = satisfactory_building.split_rods.id, from_connector = 0 }]
  power_feed       = { from_id = module.rods.pole_last_id, from_connector = local.pole_conn }
}

resource "satisfactory_building" "split_screws" {
  class = "Build_ConveyorAttachmentSplitter_C"
  x     = local.after_x.screws
  y     = local.merger_y.screws
  z     = local.attachment_z
}

resource "satisfactory_belt" "screws_to_split" {
  class          = var.belt_class
  from_id        = module.screws.output_id
  from_connector = module.screws.output_connector
  to_id          = satisfactory_building.split_screws.id
  to_connector   = 1
}

module "plates" {
  source           = "../../modules/manifold-line"
  class            = "Build_ConstructorMk1_C"
  recipe           = "Recipe_IronPlate_C"
  machines         = local.layout.plates.machines
  clock_speed      = 0.525 # 21/min
  origin           = local.origin.plates
  pitch            = local.cal.constructor.pitch
  row_gap          = local.row_gap
  machine_z        = local.machine_z
  attachment_z     = local.attachment_z
  machine_yaw      = local.cal.constructor.yaw
  inputs_side      = local.cal.constructor.side
  input_connectors = local.cal.constructor.in
  output_connector = local.cal.constructor.out
  belt_class       = var.belt_class
  feeds            = [{ from_id = satisfactory_building.split_iron.id, from_connector = 2 }]
  power_feed       = { from_id = module.screws.pole_last_id, from_connector = local.pole_conn }
}

module "rip" {
  source           = "../../modules/manifold-line"
  class            = "Build_AssemblerMk1_C"
  recipe           = "Recipe_IronPlateReinforced_C"
  machines         = local.layout.rip.machines
  clock_speed      = 0.70 # 3.5/min
  origin           = local.origin.rip
  pitch            = local.cal.assembler.pitch
  row_gap          = local.row_gap
  machine_z        = local.machine_z
  attachment_z     = local.attachment_z
  machine_yaw      = local.cal.assembler.yaw
  inputs_side      = local.cal.assembler.side
  input_connectors = local.cal.assembler.in
  output_connector = local.cal.assembler.out
  belt_class       = var.belt_class
  feeds = [
    { from_id = module.plates.output_id, from_connector = module.plates.output_connector },
    { from_id = satisfactory_building.split_screws.id, from_connector = 2 },
  ]
  power_feed = { from_id = module.plates.pole_last_id, from_connector = local.pole_conn }
}

resource "satisfactory_building" "split_rip" {
  class = "Build_ConveyorAttachmentSplitter_C"
  x     = local.after_x.rip
  y     = local.merger_y.rip
  z     = local.attachment_z
}

resource "satisfactory_belt" "rip_to_split" {
  class          = var.belt_class
  from_id        = module.rip.output_id
  from_connector = module.rip.output_connector
  to_id          = satisfactory_building.split_rip.id
  to_connector   = 1
}

module "modular_frames" {
  source           = "../../modules/manifold-line"
  class            = "Build_AssemblerMk1_C"
  recipe           = "Recipe_ModularFrame_C"
  machines         = local.layout.modular_frames.machines
  clock_speed      = 0.50 # 1/min
  origin           = local.origin.modular_frames
  pitch            = local.cal.assembler.pitch
  row_gap          = local.row_gap
  machine_z        = local.machine_z
  attachment_z     = local.attachment_z
  machine_yaw      = local.cal.assembler.yaw
  inputs_side      = local.cal.assembler.side
  input_connectors = local.cal.assembler.in
  output_connector = local.cal.assembler.out
  belt_class       = var.belt_class
  feeds = [
    { from_id = satisfactory_building.split_rip.id, from_connector = 0 },
    { from_id = satisfactory_building.split_rods.id, from_connector = 3 },
  ]
  power_feed = { from_id = module.rip.pole_last_id, from_connector = local.pole_conn }
}

module "rotors" {
  source           = "../../modules/manifold-line"
  class            = "Build_AssemblerMk1_C"
  recipe           = "Recipe_Rotor_C"
  machines         = local.layout.rotors.machines
  clock_speed      = 1.0 # 4/min
  origin           = local.origin.rotors
  pitch            = local.cal.assembler.pitch
  row_gap          = local.row_gap
  machine_z        = local.machine_z
  attachment_z     = local.attachment_z
  machine_yaw      = local.cal.assembler.yaw
  inputs_side      = local.cal.assembler.side
  input_connectors = local.cal.assembler.in
  output_connector = local.cal.assembler.out
  belt_class       = var.belt_class
  feeds = [
    { from_id = satisfactory_building.split_rods.id, from_connector = 2 },
    { from_id = satisfactory_building.split_screws.id, from_connector = 0 },
  ]
  power_feed = { from_id = module.modular_frames.pole_last_id, from_connector = local.pole_conn }
}

resource "satisfactory_building" "split_rotors" {
  class = "Build_ConveyorAttachmentSplitter_C"
  x     = local.after_x.rotors
  y     = local.merger_y.rotors
  z     = local.attachment_z
}

resource "satisfactory_belt" "rotors_to_split" {
  class          = var.belt_class
  from_id        = module.rotors.output_id
  from_connector = module.rotors.output_connector
  to_id          = satisfactory_building.split_rotors.id
  to_connector   = 1
}

module "smart_plating" {
  source           = "../../modules/manifold-line"
  class            = "Build_AssemblerMk1_C"
  recipe           = "Recipe_SpaceElevatorPart_1_C"
  machines         = local.layout.smart_plating.machines
  clock_speed      = 1.0 # 2/min
  origin           = local.origin.smart_plating
  pitch            = local.cal.assembler.pitch
  row_gap          = local.row_gap
  machine_z        = local.machine_z
  attachment_z     = local.attachment_z
  machine_yaw      = local.cal.assembler.yaw
  inputs_side      = local.cal.assembler.side
  input_connectors = local.cal.assembler.in
  output_connector = local.cal.assembler.out
  belt_class       = var.belt_class
  feeds = [
    { from_id = satisfactory_building.split_rip.id, from_connector = 2 },
    { from_id = satisfactory_building.split_rotors.id, from_connector = 2 },
  ]
  sink       = { to_id = satisfactory_building.store["smart_plating"].id, to_connector = local.storage.in }
  power_feed = { from_id = module.rotors.pole_last_id, from_connector = local.pole_conn }
}

# --- Copper -----------------------------------------------------------------

module "copper_smelters" {
  source           = "../../modules/manifold-line"
  class            = "Build_SmelterMk1_C"
  recipe           = "Recipe_IngotCopper_C"
  machines         = local.layout.copper_smelters.machines
  clock_speed      = 0.978 # 88/min
  origin           = local.origin.copper_smelters
  pitch            = local.cal.smelter.pitch
  row_gap          = local.row_gap
  machine_z        = local.machine_z
  attachment_z     = local.attachment_z
  machine_yaw      = local.cal.smelter.yaw
  inputs_side      = local.cal.smelter.side
  input_connectors = local.cal.smelter.in
  output_connector = local.cal.smelter.out
  belt_class       = var.belt_class
  feeds            = [{ from_id = satisfactory_extractor.miner["copper"].id, from_connector = local.miner_out }]
  power_feed       = { from_id = module.smart_plating.pole_last_id, from_connector = local.pole_conn }
}

resource "satisfactory_building" "split_copper" {
  class = "Build_ConveyorAttachmentSplitter_C"
  x     = local.after_x.copper_smelters
  y     = local.merger_y.copper_smelters
  z     = local.attachment_z
}

resource "satisfactory_belt" "copper_to_split" {
  class          = var.belt_class
  from_id        = module.copper_smelters.output_id
  from_connector = module.copper_smelters.output_connector
  to_id          = satisfactory_building.split_copper.id
  to_connector   = 1
}

module "wire" {
  source           = "../../modules/manifold-line"
  class            = "Build_ConstructorMk1_C"
  recipe           = "Recipe_Wire_C"
  machines         = local.layout.wire.machines
  clock_speed      = 0.907 # 136/min
  origin           = local.origin.wire
  pitch            = local.cal.constructor.pitch
  row_gap          = local.row_gap
  machine_z        = local.machine_z
  attachment_z     = local.attachment_z
  machine_yaw      = local.cal.constructor.yaw
  inputs_side      = local.cal.constructor.side
  input_connectors = local.cal.constructor.in
  output_connector = local.cal.constructor.out
  belt_class       = var.belt_class
  feeds            = [{ from_id = satisfactory_building.split_copper.id, from_connector = 0 }]
  power_feed       = { from_id = module.copper_smelters.pole_last_id, from_connector = local.pole_conn }
}

resource "satisfactory_building" "split_wire" {
  class = "Build_ConveyorAttachmentSplitter_C"
  x     = local.after_x.wire
  y     = local.merger_y.wire
  z     = local.attachment_z
}

resource "satisfactory_belt" "wire_to_split" {
  class          = var.belt_class
  from_id        = module.wire.output_id
  from_connector = module.wire.output_connector
  to_id          = satisfactory_building.split_wire.id
  to_connector   = 1
}

module "cable" {
  source           = "../../modules/manifold-line"
  class            = "Build_ConstructorMk1_C"
  recipe           = "Recipe_Cable_C"
  machines         = local.layout.cable.machines
  clock_speed      = 0.833 # 50/min
  origin           = local.origin.cable
  pitch            = local.cal.constructor.pitch
  row_gap          = local.row_gap
  machine_z        = local.machine_z
  attachment_z     = local.attachment_z
  machine_yaw      = local.cal.constructor.yaw
  inputs_side      = local.cal.constructor.side
  input_connectors = local.cal.constructor.in
  output_connector = local.cal.constructor.out
  belt_class       = var.belt_class
  feeds            = [{ from_id = satisfactory_building.split_wire.id, from_connector = 0 }]
  power_feed       = { from_id = module.wire.pole_last_id, from_connector = local.pole_conn }
}

module "copper_sheet" {
  source           = "../../modules/manifold-line"
  class            = "Build_ConstructorMk1_C"
  recipe           = "Recipe_CopperSheet_C"
  machines         = local.layout.copper_sheet.machines
  clock_speed      = 1.0 # 10/min
  origin           = local.origin.copper_sheet
  pitch            = local.cal.constructor.pitch
  row_gap          = local.row_gap
  machine_z        = local.machine_z
  attachment_z     = local.attachment_z
  machine_yaw      = local.cal.constructor.yaw
  inputs_side      = local.cal.constructor.side
  input_connectors = local.cal.constructor.in
  output_connector = local.cal.constructor.out
  belt_class       = var.belt_class
  feeds            = [{ from_id = satisfactory_building.split_copper.id, from_connector = 2 }]
  sink             = { to_id = satisfactory_building.store["copper_sheet"].id, to_connector = local.storage.in }
  power_feed       = { from_id = module.cable.pole_last_id, from_connector = local.pole_conn }
}

# --- Steel ------------------------------------------------------------------

module "foundries" {
  source           = "../../modules/manifold-line"
  class            = "Build_FoundryMk1_C"
  recipe           = "Recipe_IngotSteel_C"
  machines         = local.layout.foundries.machines
  clock_speed      = 0.728 # 98.25/min
  origin           = local.origin.foundries
  pitch            = local.cal.foundry.pitch
  row_gap          = local.row_gap
  machine_z        = local.machine_z
  attachment_z     = local.attachment_z
  machine_yaw      = local.cal.foundry.yaw
  inputs_side      = local.cal.foundry.side
  input_connectors = local.cal.foundry.in
  output_connector = local.cal.foundry.out
  belt_class       = var.belt_class
  feeds = [
    { from_id = satisfactory_extractor.miner["iron_b"].id, from_connector = local.miner_out },
    { from_id = satisfactory_extractor.miner["coal_a"].id, from_connector = local.miner_out },
  ]
  power_feed = { from_id = module.copper_sheet.pole_last_id, from_connector = local.pole_conn }
}

resource "satisfactory_building" "split_steel" {
  class = "Build_ConveyorAttachmentSplitter_C"
  x     = local.after_x.foundries
  y     = local.merger_y.foundries
  z     = local.attachment_z
}

resource "satisfactory_belt" "steel_to_split" {
  class          = var.belt_class
  from_id        = module.foundries.output_id
  from_connector = module.foundries.output_connector
  to_id          = satisfactory_building.split_steel.id
  to_connector   = 1
}

module "beams" {
  source           = "../../modules/manifold-line"
  class            = "Build_ConstructorMk1_C"
  recipe           = "Recipe_SteelBeam_C"
  machines         = local.layout.beams.machines
  clock_speed      = 0.65 # 19.5/min
  origin           = local.origin.beams
  pitch            = local.cal.constructor.pitch
  row_gap          = local.row_gap
  machine_z        = local.machine_z
  attachment_z     = local.attachment_z
  machine_yaw      = local.cal.constructor.yaw
  inputs_side      = local.cal.constructor.side
  input_connectors = local.cal.constructor.in
  output_connector = local.cal.constructor.out
  belt_class       = var.belt_class
  feeds            = [{ from_id = satisfactory_building.split_steel.id, from_connector = 0 }]
  power_feed       = { from_id = module.foundries.pole_last_id, from_connector = local.pole_conn }
}

resource "satisfactory_building" "split_beams" {
  class = "Build_ConveyorAttachmentSplitter_C"
  x     = local.after_x.beams
  y     = local.merger_y.beams
  z     = local.attachment_z
}

resource "satisfactory_belt" "beams_to_split" {
  class          = var.belt_class
  from_id        = module.beams.output_id
  from_connector = module.beams.output_connector
  to_id          = satisfactory_building.split_beams.id
  to_connector   = 1
}

module "pipes" {
  source           = "../../modules/manifold-line"
  class            = "Build_ConstructorMk1_C"
  recipe           = "Recipe_SteelPipe_C"
  machines         = local.layout.pipes.machines
  clock_speed      = 0.675 # 13.5/min
  origin           = local.origin.pipes
  pitch            = local.cal.constructor.pitch
  row_gap          = local.row_gap
  machine_z        = local.machine_z
  attachment_z     = local.attachment_z
  machine_yaw      = local.cal.constructor.yaw
  inputs_side      = local.cal.constructor.side
  input_connectors = local.cal.constructor.in
  output_connector = local.cal.constructor.out
  belt_class       = var.belt_class
  feeds            = [{ from_id = satisfactory_building.split_steel.id, from_connector = 2 }]
  power_feed       = { from_id = module.beams.pole_last_id, from_connector = local.pole_conn }
}

module "stators" {
  source           = "../../modules/manifold-line"
  class            = "Build_AssemblerMk1_C"
  recipe           = "Recipe_Stator_C"
  machines         = local.layout.stators.machines
  clock_speed      = 0.90 # 4.5/min
  origin           = local.origin.stators
  pitch            = local.cal.assembler.pitch
  row_gap          = local.row_gap
  machine_z        = local.machine_z
  attachment_z     = local.attachment_z
  machine_yaw      = local.cal.assembler.yaw
  inputs_side      = local.cal.assembler.side
  input_connectors = local.cal.assembler.in
  output_connector = local.cal.assembler.out
  belt_class       = var.belt_class
  feeds = [
    { from_id = module.pipes.output_id, from_connector = module.pipes.output_connector },
    { from_id = satisfactory_building.split_wire.id, from_connector = 2 },
  ]
  power_feed = { from_id = module.pipes.pole_last_id, from_connector = local.pole_conn }
}

resource "satisfactory_building" "split_stators" {
  class = "Build_ConveyorAttachmentSplitter_C"
  x     = local.after_x.stators
  y     = local.merger_y.stators
  z     = local.attachment_z
}

resource "satisfactory_belt" "stators_to_split" {
  class          = var.belt_class
  from_id        = module.stators.output_id
  from_connector = module.stators.output_connector
  to_id          = satisfactory_building.split_stators.id
  to_connector   = 1
}

module "motors" {
  source           = "../../modules/manifold-line"
  class            = "Build_AssemblerMk1_C"
  recipe           = "Recipe_Motor_C"
  machines         = local.layout.motors.machines
  clock_speed      = 0.20 # 1/min
  origin           = local.origin.motors
  pitch            = local.cal.assembler.pitch
  row_gap          = local.row_gap
  machine_z        = local.machine_z
  attachment_z     = local.attachment_z
  machine_yaw      = local.cal.assembler.yaw
  inputs_side      = local.cal.assembler.side
  input_connectors = local.cal.assembler.in
  output_connector = local.cal.assembler.out
  belt_class       = var.belt_class
  feeds = [
    { from_id = satisfactory_building.split_rotors.id, from_connector = 0 },
    { from_id = satisfactory_building.split_stators.id, from_connector = 0 },
  ]
  sink       = { to_id = satisfactory_building.store["motors"].id, to_connector = local.storage.in }
  power_feed = { from_id = module.stators.pole_last_id, from_connector = local.pole_conn }
}

module "automated_wiring" {
  source           = "../../modules/manifold-line"
  class            = "Build_AssemblerMk1_C"
  recipe           = "Recipe_SpaceElevatorPart_3_C"
  machines         = local.layout.automated_wiring.machines
  clock_speed      = 1.0 # 2.5/min
  origin           = local.origin.automated_wiring
  pitch            = local.cal.assembler.pitch
  row_gap          = local.row_gap
  machine_z        = local.machine_z
  attachment_z     = local.attachment_z
  machine_yaw      = local.cal.assembler.yaw
  inputs_side      = local.cal.assembler.side
  input_connectors = local.cal.assembler.in
  output_connector = local.cal.assembler.out
  belt_class       = var.belt_class
  feeds = [
    { from_id = satisfactory_building.split_stators.id, from_connector = 2 },
    { from_id = module.cable.output_id, from_connector = module.cable.output_connector },
  ]
  sink       = { to_id = satisfactory_building.store["automated_wiring"].id, to_connector = local.storage.in }
  power_feed = { from_id = module.motors.pole_last_id, from_connector = local.pole_conn }
}

module "concrete" {
  source           = "../../modules/manifold-line"
  class            = "Build_ConstructorMk1_C"
  recipe           = "Recipe_Concrete_C"
  machines         = local.layout.concrete.machines
  clock_speed      = 1.0 # 15/min
  origin           = local.origin.concrete
  pitch            = local.cal.constructor.pitch
  row_gap          = local.row_gap
  machine_z        = local.machine_z
  attachment_z     = local.attachment_z
  machine_yaw      = local.cal.constructor.yaw
  inputs_side      = local.cal.constructor.side
  input_connectors = local.cal.constructor.in
  output_connector = local.cal.constructor.out
  belt_class       = var.belt_class
  feeds            = [{ from_id = satisfactory_extractor.miner["limestone"].id, from_connector = local.miner_out }]
  power_feed       = { from_id = module.automated_wiring.pole_last_id, from_connector = local.pole_conn }
}

module "eib" {
  source           = "../../modules/manifold-line"
  class            = "Build_AssemblerMk1_C"
  recipe           = "Recipe_EncasedIndustrialBeam_C"
  machines         = local.layout.eib.machines
  clock_speed      = 0.4167 # 2.5/min
  origin           = local.origin.eib
  pitch            = local.cal.assembler.pitch
  row_gap          = local.row_gap
  machine_z        = local.machine_z
  attachment_z     = local.attachment_z
  machine_yaw      = local.cal.assembler.yaw
  inputs_side      = local.cal.assembler.side
  input_connectors = local.cal.assembler.in
  output_connector = local.cal.assembler.out
  belt_class       = var.belt_class
  feeds = [
    { from_id = satisfactory_building.split_beams.id, from_connector = 2 },
    { from_id = module.concrete.output_id, from_connector = module.concrete.output_connector },
  ]
  sink       = { to_id = satisfactory_building.store["eib"].id, to_connector = local.storage.in }
  power_feed = { from_id = module.concrete.pole_last_id, from_connector = local.pole_conn }
}

module "versatile_frame" {
  source           = "../../modules/manifold-line"
  class            = "Build_AssemblerMk1_C"
  recipe           = "Recipe_SpaceElevatorPart_2_C"
  machines         = local.layout.versatile_frame.machines
  clock_speed      = 0.40 # 2/min
  origin           = local.origin.versatile_frame
  pitch            = local.cal.assembler.pitch
  row_gap          = local.row_gap
  machine_z        = local.machine_z
  attachment_z     = local.attachment_z
  machine_yaw      = local.cal.assembler.yaw
  inputs_side      = local.cal.assembler.side
  input_connectors = local.cal.assembler.in
  output_connector = local.cal.assembler.out
  belt_class       = var.belt_class
  feeds = [
    { from_id = module.modular_frames.output_id, from_connector = module.modular_frames.output_connector },
    { from_id = satisfactory_building.split_beams.id, from_connector = 0 },
  ]
  sink       = { to_id = satisfactory_building.store["versatile_frame"].id, to_connector = local.storage.in }
  power_feed = { from_id = module.eib.pole_last_id, from_connector = local.pole_conn }
}

# --- Sinks ------------------------------------------------------------------

resource "satisfactory_building" "store" {
  for_each = toset(["smart_plating", "copper_sheet", "motors", "automated_wiring", "eib", "versatile_frame"])
  class    = "Build_StorageContainerMk1_C"
  x        = local.after_x[each.key]
  y        = local.merger_y[each.key]
  z        = local.machine_z
  yaw      = local.storage.yaw
}
