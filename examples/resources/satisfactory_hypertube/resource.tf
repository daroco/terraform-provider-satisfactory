resource "satisfactory_hypertube" "hub_to_outpost" {
  # class defaults to Build_PipeHyper_C
  from_id        = satisfactory_building.entrance_hub.id
  from_connector = 0
  to_id          = satisfactory_building.entrance_outpost.id
  to_connector   = 0
}
