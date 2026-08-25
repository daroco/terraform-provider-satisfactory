resource "satisfactory_power_line" "link" {
  # class defaults to Build_PowerLine_C.
  from_id        = satisfactory_building.smelter.id
  from_connector = 0
  to_id          = satisfactory_building.constructor.id
  to_connector   = 0
}
