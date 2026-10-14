# Find a node first; extractors are placed on a world feature, not at a point.
data "satisfactory_resource_nodes" "iron" {
  resource = "Desc_OreIron_C"
  purity   = "normal"
  x        = -77737
  y        = 220338
  z        = -337
}

resource "satisfactory_extractor" "iron" {
  class   = "Build_MinerMk2_C"
  node_id = data.satisfactory_resource_nodes.iron.nodes[0].id
  # clock_speed defaults to 1.0 on create and updates in place.
  clock_speed = 1.0
}

# Water volumes are areas: say where on the surface the extractor goes.
data "satisfactory_resource_nodes" "water" {
  resource = "Desc_Water_C"
  x        = -77737
  y        = 220338
  z        = -337
}

resource "satisfactory_extractor" "water" {
  class   = "Build_WaterPump_C"
  node_id = data.satisfactory_resource_nodes.water.nodes[0].id
  x       = data.satisfactory_resource_nodes.water.nodes[0].x + 1000
  y       = data.satisfactory_resource_nodes.water.nodes[0].y
}
