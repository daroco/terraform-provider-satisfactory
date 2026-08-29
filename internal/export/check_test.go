package export

import (
	"strings"
	"testing"

	"github.com/daroco/terraform-provider-satisfactory/internal/api"
)

func levels(findings []Finding) map[Level]int {
	m := map[Level]int{}
	for _, f := range findings {
		m[f.Level]++
	}
	return m
}

func find(findings []Finding, substr string) *Finding {
	for i := range findings {
		if strings.Contains(findings[i].Message, substr) {
			return &findings[i]
		}
	}
	return nil
}

func healthy() []api.WorldBuildable {
	return []api.WorldBuildable{
		{Index: 0, Class: "Build_Foundation_8x4_01_C", Lightweight: true},
		{Index: 1, Class: "Build_SmelterMk1_C"},
		{Index: 2, Class: "Build_ConstructorMk1_C"},
		{Index: 3, Class: "Build_ConveyorBeltMk1_C", Connects: &api.WorldConnection{
			From: api.WorldEndpoint{Index: 1, Connector: 1},
			To:   api.WorldEndpoint{Index: 2, Connector: 0},
		}},
	}
}

func TestCheckPassesAHealthyWorld(t *testing.T) {
	findings := Check(healthy(), []api.Player{{Name: "pioneer"}})
	if Failed(findings) {
		t.Errorf("healthy world reported failures: %+v", findings)
	}
	if levels(findings)[LevelOK] == 0 {
		t.Error("a passing check should still say what it ruled out")
	}
}

// The failure the endpoint exists to prevent: enumerate actors only and every
// export silently loses its floor.
func TestCheckWarnsWhenNothingIsLightweight(t *testing.T) {
	items := healthy()
	items[0].Lightweight = false
	findings := Check(items, nil)
	f := find(findings, "no lightweight buildables")
	if f == nil {
		t.Fatalf("missing warning: %+v", findings)
	}
	if f.Level != LevelWarn {
		t.Errorf("level = %v; a floorless region is possible, so this cannot be fatal", f.Level)
	}
	if !strings.Contains(f.Detail, "missing its floor") {
		t.Errorf("the detail should say what breaks: %q", f.Detail)
	}
}

func TestCheckFailsWhenNoBeltResolves(t *testing.T) {
	items := healthy()
	items[3].Connects = nil
	findings := Check(items, nil)
	if !Failed(findings) {
		t.Errorf("belts present but none resolved should be fatal: %+v", findings)
	}
}

func TestCheckFailsOnEndpointOutsideTheResponse(t *testing.T) {
	items := healthy()
	items[3].Connects.To.Index = 99
	if findings := Check(items, nil); !Failed(findings) {
		t.Errorf("an out-of-range endpoint index must be fatal: %+v", findings)
	}
}

func TestCheckFailsOnSelfReference(t *testing.T) {
	items := healthy()
	items[3].Connects.To.Index = items[3].Connects.From.Index
	if findings := Check(items, nil); !Failed(findings) {
		t.Errorf("a belt joining a buildable to itself must be fatal: %+v", findings)
	}
}

func TestCheckFailsOnDuplicateIndices(t *testing.T) {
	items := healthy()
	items[2].Index = 1
	if findings := Check(items, nil); !Failed(findings) {
		t.Errorf("duplicate indices break every reference and must be fatal: %+v", findings)
	}
}

func TestCheckFailsOnEmptyRegion(t *testing.T) {
	if findings := Check(nil, nil); !Failed(findings) {
		t.Errorf("an empty world listing must be fatal: %+v", findings)
	}
}

// Belts are the only thing exercising the graph; without them the run proves
// less than it appears to, and should say so.
func TestCheckWarnsWhenThereAreNoConnections(t *testing.T) {
	items := healthy()[:3]
	findings := Check(items, []api.Player{{}})
	if Failed(findings) {
		t.Errorf("a region with no belts is not broken: %+v", findings)
	}
	if find(findings, "no belts or wires") == nil {
		t.Errorf("should say the graph went untested: %+v", findings)
	}
}
