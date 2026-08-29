resource "satisfactory_pipeline" "water_to_refinery" {
  class          = "Build_Pipeline_C"
  from_id        = satisfactory_building.water_extractor.id
  from_connector = 0 # extractor fluid output
  to_id          = satisfactory_building.refinery.id
  to_connector   = 0 # refinery fluid input
}
