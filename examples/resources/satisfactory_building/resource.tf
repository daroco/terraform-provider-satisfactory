resource "satisfactory_building" "smelter" {
  class  = "Build_SmelterMk1_C"
  x      = 200
  y      = 0
  z      = 20200
  recipe = "Recipe_IngotIron_C"
  # clock_speed defaults to 1.0 on create for manufacturer classes.
  clock_speed = 1.0
}
