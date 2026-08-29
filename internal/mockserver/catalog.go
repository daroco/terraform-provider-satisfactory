package mockserver

import (
	"net/http"

	"github.com/daroco/terraform-provider-satisfactory/internal/api"
)

// sampleCatalog is a representative slice of the game's class catalog: at
// least one class per mechanism the mod reports, so the coverage report and
// its tests exercise every branch. The real list has a few hundred entries
// and comes from the game's asset registry; this one is hand-picked and makes
// no claim to completeness.
func sampleCatalog() []api.ClassInfo {
	return []api.ClassInfo{
		{Class: "Build_Foundation_8x4_01_C", DisplayName: "Foundation 8m x 4m", Mechanism: "foundation", Supported: true, Resource: "satisfactory_foundation"},
		{Class: "Build_Wall_8x4_01_C", DisplayName: "Wall 8m x 4m", Mechanism: "foundation", Supported: true, Resource: "satisfactory_foundation"},
		{Class: "Build_Ramp_8x4_01_C", DisplayName: "Ramp 8m x 4m", Mechanism: "foundation", Supported: true, Resource: "satisfactory_foundation"},
		{Class: "Build_ConstructorMk1_C", DisplayName: "Constructor", Mechanism: "building", Supported: true, Resource: "satisfactory_building"},
		{Class: "Build_SmelterMk1_C", DisplayName: "Smelter", Mechanism: "building", Supported: true, Resource: "satisfactory_building"},
		{Class: "Build_StorageContainerMk1_C", DisplayName: "Storage Container", Mechanism: "building", Supported: true, Resource: "satisfactory_building"},
		{Class: "Build_ConveyorAttachmentSplitterSmart_C", DisplayName: "Smart Splitter", Mechanism: "building", Supported: true, Resource: "satisfactory_building",
			SettingsNotModelled: "per-output item filter rules"},
		{Class: "Build_StandaloneWidgetSign_Small_C", DisplayName: "Small Sign", Mechanism: "building", Supported: true, Resource: "satisfactory_building",
			SettingsNotModelled: "text, icon and colours"},
		{Class: "Build_ConveyorBeltMk1_C", DisplayName: "Conveyor Belt Mk.1", Mechanism: "belt", Supported: true, Resource: "satisfactory_belt"},
		{Class: "Build_PowerLine_C", DisplayName: "Power Line", Mechanism: "power_line", Supported: true, Resource: "satisfactory_power_line"},
		{Class: "Build_Pipeline_C", DisplayName: "Pipeline", Mechanism: "pipeline", Supported: true, Resource: "satisfactory_pipeline"},
		{Class: "Build_PipeHyper_C", DisplayName: "Hypertube", Mechanism: "hypertube", Supported: true, Resource: "satisfactory_hypertube"},
		{Class: "Build_MinerMk1_C", DisplayName: "Miner Mk.1", Mechanism: "node_bound", Supported: false,
			WhyUnsupported: "placed on a resource node or geyser, not at a coordinate; needs a way to discover and reference world features"},
		{Class: "Build_GeneratorGeoThermal_C", DisplayName: "Geothermal Generator", Mechanism: "node_bound", Supported: false,
			WhyUnsupported: "placed on a resource node or geyser, not at a coordinate; needs a way to discover and reference world features"},
		{Class: "Build_RailroadTrack_C", DisplayName: "Railway", Mechanism: "rail_track", Supported: false,
			WhyUnsupported: "a spline network with switches and signals bound to positions along it; no resource models that yet"},
		{Class: "Build_TrainStation_C", DisplayName: "Train Station", Mechanism: "rail", Supported: false,
			WhyUnsupported: "attaches to track rather than standing alone"},
	}
}

func (s *Server) listClasses(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, sampleCatalog())
}
