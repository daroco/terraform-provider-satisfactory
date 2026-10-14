variable "from" {
  type        = object({ x = number, y = number, z = number })
  description = "Where the first pole stands, in centimetres."
}

variable "to" {
  type        = object({ x = number, y = number, z = number })
  description = "Where the last pole stands."
}

variable "max_span" {
  type        = number
  default     = 7000
  description = "Longest wire between two poles, in centimetres. A Mk1 pole's wire reaches about 70 m in-game; the API does not enforce it, so this is about the result looking and behaving like a built one."
  validation {
    condition     = var.max_span > 0
    error_message = "max_span must be positive."
  }
}

variable "pole_class" {
  type    = string
  default = "Build_PowerPoleMk1_C"
}

variable "feed" {
  type        = object({ from_id = string, from_connector = number })
  default     = null
  description = "Optional: wire this buildable's power connector to the first pole."
}

variable "sink" {
  type        = object({ to_id = string, to_connector = number })
  default     = null
  description = "Optional: wire the last pole to this buildable's power connector."
}
