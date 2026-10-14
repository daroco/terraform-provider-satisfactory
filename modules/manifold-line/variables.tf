variable "class" {
  type        = string
  description = "Machine class placed `machines` times, e.g. Build_ConstructorMk1_C."
}

variable "recipe" {
  type        = string
  default     = ""
  description = "Recipe class for every machine, e.g. Recipe_Screw_C. Empty for classes without one (generators)."
}

variable "machines" {
  type        = number
  description = "How many machines in the row."
  validation {
    condition     = var.machines >= 1 && floor(var.machines) == var.machines
    error_message = "machines must be a whole number >= 1."
  }
}

variable "clock_speed" {
  type        = number
  default     = null
  description = "Clock speed for every machine (1.0 = 100%). Null leaves the game's default."
}

variable "origin" {
  type        = object({ x = number, y = number })
  description = "Where the first machine stands, in centimetres. The row extends along +x."
}

variable "pitch" {
  type        = number
  default     = 1000
  description = "Distance between machine origins along x, in centimetres. Use at least the class's footprint along x (data.satisfactory_buildable_class.size.x)."
}

variable "row_gap" {
  type        = number
  default     = 800
  description = "Distance between the machine row and each splitter/merger/pole row, in centimetres. One 8 m tile by default."
}

variable "machine_z" {
  type        = number
  description = "Z of the machines' origins."
}

variable "attachment_z" {
  type        = number
  description = "Z of the splitters and mergers (their pivot sits lower than a machine's; see examples/factory-hub)."
}

variable "machine_yaw" {
  type        = number
  default     = 0
  description = "Yaw for every machine, in degrees. Pick it so the inputs face the splitter rows (see inputs_side)."
}

variable "inputs_side" {
  type        = number
  default     = 1
  description = <<-EOT
    Which side of the machine row the input (splitter) rows are on: +1 for +y,
    -1 for -y. The output (merger) row goes on the opposite side. At yaw 0 a
    constructor's input faces +y (inputs_side = 1); a smelter's output faces
    +y, so its inputs are on -y (inputs_side = -1). See examples/factory-hub
    for how these were found.
  EOT
  validation {
    condition     = var.inputs_side == 1 || var.inputs_side == -1
    error_message = "inputs_side must be 1 or -1."
  }
}

variable "input_connectors" {
  type        = list(number)
  description = "The machine's input connector index for each input stream, in the order of `feeds`. One entry per input (a constructor has one, an assembler two)."
}

variable "output_connector" {
  type        = number
  default     = null
  description = "The machine's output connector index. Null (with has_output = false) for machines that only consume."
}

variable "feeds" {
  type        = list(object({ from_id = string, from_connector = number }))
  description = "What feeds each input stream: the buildable and connector a belt runs from into the first splitter of that stream. Same length and order as input_connectors."
}

variable "has_output" {
  type        = bool
  default     = true
  description = "Whether to build the merger row. False for consumers like coal generators."
}

variable "sink" {
  type        = object({ to_id = string, to_connector = number })
  default     = null
  description = "Where the merged output goes (e.g. a storage container's input). Null leaves it on the last merger's output; see output_id/output_connector."
}

variable "belt_class" {
  type        = string
  default     = "Build_ConveyorBeltMk2_C"
  description = "Belt class for every belt in the line."
}

variable "splitter_class" {
  type        = string
  default     = "Build_ConveyorAttachmentSplitter_C"
}

variable "merger_class" {
  type        = string
  default     = "Build_ConveyorAttachmentMerger_C"
}

variable "pole_class" {
  type        = string
  default     = "Build_PowerPoleMk1_C"
  description = "Power pole class; one pole per machine. Empty string places no poles (and no wires)."
}

variable "power_feed" {
  type        = object({ from_id = string, from_connector = number })
  default     = null
  description = "What powers the line: a wire runs from here to the first pole. Null leaves the first pole unfed; wire it from pole_first_id."
}

variable "power_connector" {
  type        = number
  default     = 0
  description = "The machine's power connector index. Almost always 0."
}
