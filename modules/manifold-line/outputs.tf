output "machine_ids" {
  value       = { for k, m in satisfactory_building.machine : k => m.id }
  description = "tf_id of each machine, keyed \"0\".. \"count-1\"."
}

output "output_id" {
  value       = var.has_output ? satisfactory_building.merger[tostring(local.n - 1)].id : null
  description = "The buildable carrying the line's merged output (the last merger). Null when has_output is false."
}

output "output_connector" {
  value       = var.has_output ? 1 : null
  description = "Connector index of the merged output on output_id."
}

output "pole_first_id" {
  value       = local.has_poles ? satisfactory_building.pole["0"].id : null
  description = "First power pole; wire the line's supply here when power_feed is not set. Connector 0."
}

output "pole_ids" {
  value       = { for k, p in satisfactory_building.pole : k => p.id }
  description = "tf_id of each pole, keyed like machine_ids. A Mk1 pole has 4 connections; the module uses up to 3 (machine, previous, next), so each has one spare."
}

output "pole_last_id" {
  value       = local.has_poles ? satisfactory_building.pole[tostring(local.n - 1)].id : null
  description = "Last power pole; chain the next line from here. Connector 0."
}

output "extent" {
  value = {
    min_x = var.origin.x
    max_x = var.origin.x + (local.n - 1) * var.pitch
    min_y = min(local.pole_y, local.splitter_y[local.inputs - 1])
    max_y = max(local.pole_y, local.splitter_y[local.inputs - 1])
  }
  description = "Bounding box of everything the line places (origins, not footprints), for laying lines out next to each other."
}
