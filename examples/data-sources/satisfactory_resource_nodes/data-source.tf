# Every free, non-impure coal node, nearest first, from a point on the map.
data "satisfactory_resource_nodes" "coal" {
  resource = "Desc_Coal_C"
  x        = -77737
  y        = 220338
  z        = -337
}

locals {
  usable_coal = [for n in data.satisfactory_resource_nodes.coal.nodes : n if n.purity != "impure"]
}

output "nearest_coal" {
  value = local.usable_coal[0]
}
