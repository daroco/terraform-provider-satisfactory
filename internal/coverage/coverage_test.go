package coverage

import (
	"strings"
	"testing"

	"github.com/daroco/terraform-provider-satisfactory/internal/api"
)

func catalog() []api.ClassInfo {
	return []api.ClassInfo{
		{Class: "Build_Foundation_8x4_01_C", Mechanism: "foundation", Supported: true, Resource: "satisfactory_foundation"},
		{Class: "Build_Wall_8x4_01_C", Mechanism: "foundation", Supported: true, Resource: "satisfactory_foundation"},
		{Class: "Build_ConstructorMk1_C", Mechanism: "building", Supported: true, Resource: "satisfactory_building"},
		{Class: "Build_ConveyorBeltMk1_C", Mechanism: "belt", Supported: true, Resource: "satisfactory_belt"},
		{Class: "Build_MinerMk1_C", Mechanism: "node_bound", WhyUnsupported: "needs node discovery"},
		{Class: "Build_MinerMk2_C", Mechanism: "node_bound", WhyUnsupported: "needs node discovery"},
		{Class: "Build_RailroadTrack_C", Mechanism: "rail_track", WhyUnsupported: "spline network"},
		{Class: "Build_SmartSplitter_C", Mechanism: "building", Supported: true, Resource: "satisfactory_building",
			SettingsNotModelled: "filter rules"},
	}
}

func TestSummarizeCountsAndPercent(t *testing.T) {
	s := Summarize(catalog())
	if s.Total != 8 || s.Supported != 5 || s.Unsupported != 3 {
		t.Errorf("total/supported/unsupported = %d/%d/%d, want 8/5/3", s.Total, s.Supported, s.Unsupported)
	}
	if got := s.Percent(); got < 62.4 || got > 62.6 {
		t.Errorf("Percent = %.2f, want 62.5", got)
	}
	if Summarize(nil).Percent() != 0 {
		t.Error("empty catalog must not divide by zero")
	}
}

// The report is read to decide what to build next, so gaps are ordered by
// size: the mechanism unblocking the most classes comes first.
func TestSummarizeOrdersSupportedFirstThenLargestGap(t *testing.T) {
	s := Summarize(catalog())
	var order []string
	for _, g := range s.Groups {
		order = append(order, g.Mechanism)
	}
	joined := strings.Join(order, ",")
	sup := strings.Index(joined, "foundation")
	unsup := strings.Index(joined, "node_bound")
	rail := strings.Index(joined, "rail_track")
	if sup > unsup {
		t.Errorf("supported groups should precede unsupported: %v", order)
	}
	if unsup > rail {
		t.Errorf("node_bound (2 classes) should precede rail_track (1): %v", order)
	}
}

func TestReportNamesEveryGapAndItsReason(t *testing.T) {
	out := Report(Summarize(catalog()), false)
	for _, want := range []string{
		"8 placeable classes",
		"can place 5 (62%)",
		"Build_MinerMk1_C",
		"Build_RailroadTrack_C",
		"needs node discovery",
		"spline network",
		"Build_SmartSplitter_C",
		"filter rules",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
	// Supported classes are counted, not listed, unless asked: the gaps are
	// the point.
	if strings.Contains(out, "Build_Wall_8x4_01_C") {
		t.Errorf("supported classes should not be listed by default:\n%s", out)
	}
	if !strings.Contains(Report(Summarize(catalog()), true), "Build_Wall_8x4_01_C") {
		t.Error("-all should list supported classes")
	}
}

func TestReportOmitsEmptySections(t *testing.T) {
	full := []api.ClassInfo{{Class: "Build_X_C", Mechanism: "building", Supported: true, Resource: "satisfactory_building"}}
	out := Report(Summarize(full), false)
	if strings.Contains(out, "Not yet") || strings.Contains(out, "cannot configure") {
		t.Errorf("a fully covered catalog should have no gap sections:\n%s", out)
	}
}
