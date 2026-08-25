resource "satisfactory_belt" "ingots" {
  class          = "Build_ConveyorBeltMk1_C"
  from_id        = satisfactory_building.smelter.id
  from_connector = 1 # smelter output
  to_id          = satisfactory_building.constructor.id
  to_connector   = 0 # constructor input
}
