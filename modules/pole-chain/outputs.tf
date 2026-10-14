output "first_id" {
  value       = satisfactory_building.pole["0"].id
  description = "The pole at `from`. Connector 0."
}

output "last_id" {
  value       = satisfactory_building.pole[tostring(local.spans)].id
  description = "The pole at `to`. Connector 0."
}

output "pole_count" {
  value = local.spans + 1
}
