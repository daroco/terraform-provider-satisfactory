package provider

import (
	"context"
	"fmt"
	"math"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/daroco/terraform-provider-satisfactory/internal/api"
	"github.com/daroco/terraform-provider-satisfactory/internal/client"
)

// resourceNodesDataSource lists the world features an extractor can bind to:
// resource nodes, geysers and water volumes. It is how a config finds a node
// for satisfactory_extractor without hard-coding actor paths. Read-only, so
// safe during plan.
type resourceNodesDataSource struct {
	client *client.Client
}

func newResourceNodesDataSource() datasource.DataSource { return &resourceNodesDataSource{} }

type resourceNodeModel struct {
	ID           types.String  `tfsdk:"id"`
	Resource     types.String  `tfsdk:"resource"`
	Purity       types.String  `tfsdk:"purity"`
	Form         types.String  `tfsdk:"form"`
	Type         types.String  `tfsdk:"type"`
	Occupied     types.Bool    `tfsdk:"occupied"`
	OccupantTFID types.String  `tfsdk:"occupant_tf_id"`
	X            types.Float64 `tfsdk:"x"`
	Y            types.Float64 `tfsdk:"y"`
	Z            types.Float64 `tfsdk:"z"`
	Yaw          types.Float64 `tfsdk:"yaw"`
	BoundsMin    *vec3Model    `tfsdk:"bounds_min"`
	BoundsMax    *vec3Model    `tfsdk:"bounds_max"`
}

type resourceNodesModel struct {
	Resource        types.String        `tfsdk:"resource"`
	Purity          types.String        `tfsdk:"purity"`
	X               types.Float64       `tfsdk:"x"`
	Y               types.Float64       `tfsdk:"y"`
	Z               types.Float64       `tfsdk:"z"`
	Radius          types.Float64       `tfsdk:"radius"`
	IncludeOccupied types.Bool          `tfsdk:"include_occupied"`
	Nodes           []resourceNodeModel `tfsdk:"nodes"`
}

func (d *resourceNodesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_resource_nodes"
}

func (d *resourceNodesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Resource nodes, geysers and water volumes an extractor can be placed on. " +
			"Miners and water extractors are placed on a world feature rather than at a coordinate; " +
			"use this to pick one and pass its `id` to `satisfactory_extractor.node_id`. " +
			"When `x`/`y`/`z` are given the list is sorted by distance from that point (ties by id), " +
			"otherwise by id - so `nodes[0]` is a stable choice across plans. Only features the game " +
			"currently has loaded are reported; on a large map that can exclude distant ones.",
		Attributes: map[string]schema.Attribute{
			"resource": schema.StringAttribute{
				Optional:    true,
				Description: "Keep only features of this resource descriptor class, e.g. Desc_OreIron_C or Desc_Water_C.",
			},
			"purity": schema.StringAttribute{
				Optional:    true,
				Description: "Keep only nodes of this purity: impure, normal or pure. Water volumes have no purity.",
			},
			"x": schema.Float64Attribute{
				Optional:    true,
				Description: "Centre to sort (and optionally filter) by, in centimetres. x, y and z go together.",
			},
			"y": schema.Float64Attribute{Optional: true},
			"z": schema.Float64Attribute{Optional: true},
			"radius": schema.Float64Attribute{
				Optional:    true,
				Description: "Keep only features within this many centimetres of x/y/z. Requires x, y and z.",
			},
			"include_occupied": schema.BoolAttribute{
				Optional: true,
				Description: "Also list features that already have an extractor. Set this when selecting nodes by " +
					"index and filter on `occupant_tf_id` instead, so the nodes this config already owns keep " +
					"their position in the list from one plan to the next.",
			},
			"nodes": schema.ListNestedAttribute{
				Computed:    true,
				Description: "Matching features, in contract order.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":             schema.StringAttribute{Computed: true, Description: "Stable id; what satisfactory_extractor.node_id takes."},
						"resource":       schema.StringAttribute{Computed: true, Description: "Resource descriptor class, e.g. Desc_OreIron_C."},
						"purity":         schema.StringAttribute{Computed: true, Description: "impure, normal or pure; empty for water."},
						"form":           schema.StringAttribute{Computed: true, Description: "solid, liquid, gas or heat."},
						"type":           schema.StringAttribute{Computed: true, Description: "node, geyser, water, fracking or fracking_satellite."},
						"occupied":       schema.BoolAttribute{Computed: true, Description: "True when an extractor already sits on it."},
						"occupant_tf_id": schema.StringAttribute{Computed: true, Description: "The Terraform-managed extractor on it, if any; empty otherwise."},
						"x":              schema.Float64Attribute{Computed: true, Description: "Position in whole centimetres."},
						"y":              schema.Float64Attribute{Computed: true, Description: "Position in whole centimetres."},
						"z":              schema.Float64Attribute{Computed: true, Description: "Position in whole centimetres. For water, the volume's centre at surface height."},
						"yaw":            schema.Float64Attribute{Computed: true, Description: "Default rotation an extractor gets here, in degrees."},
						"bounds_min":     vec3DataAttr("Water volumes only: lowest corner of the volume's box."),
						"bounds_max":     vec3DataAttr("Water volumes only: highest corner of the volume's box."),
					},
				},
			},
		},
	}
}

func vec3DataAttr(desc string) schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Computed:    true,
		Description: desc,
		Attributes: map[string]schema.Attribute{
			"x": schema.Float64Attribute{Computed: true},
			"y": schema.Float64Attribute{Computed: true},
			"z": schema.Float64Attribute{Computed: true},
		},
	}
}

func (d *resourceNodesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
		return
	}
	d.client = c
}

func (d *resourceNodesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg resourceNodesModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	hasX, hasY, hasZ := !cfg.X.IsNull(), !cfg.Y.IsNull(), !cfg.Z.IsNull()
	if (hasX || hasY || hasZ) && (!hasX || !hasY || !hasZ) {
		resp.Diagnostics.AddError("Incomplete centre", "x, y and z must be given together.")
		return
	}
	if !cfg.Radius.IsNull() && !hasX {
		resp.Diagnostics.AddError("radius without a centre", "radius needs x, y and z to measure from.")
		return
	}
	switch cfg.Purity.ValueString() {
	case "", "impure", "normal", "pure":
	default:
		resp.Diagnostics.AddError("Invalid purity", fmt.Sprintf("purity must be impure, normal or pure, not %q.", cfg.Purity.ValueString()))
		return
	}

	query := api.ResourceNodeQuery{
		Resource:        cfg.Resource.ValueString(),
		Purity:          cfg.Purity.ValueString(),
		IncludeOccupied: cfg.IncludeOccupied.ValueBool(),
	}
	if hasX {
		query.Center = &api.Vec3{X: cfg.X.ValueFloat64(), Y: cfg.Y.ValueFloat64(), Z: cfg.Z.ValueFloat64()}
	}
	if !cfg.Radius.IsNull() {
		r := cfg.Radius.ValueFloat64()
		query.Radius = &r
	}
	nodes, err := d.client.ListResourceNodes(ctx, query)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list resource nodes", err.Error())
		return
	}

	cfg.Nodes = make([]resourceNodeModel, 0, len(nodes))
	for _, n := range nodes {
		m := resourceNodeModel{
			ID:           types.StringValue(n.ID),
			Resource:     types.StringValue(n.Resource),
			Purity:       types.StringValue(n.Purity),
			Form:         types.StringValue(n.Form),
			Type:         types.StringValue(n.Type),
			Occupied:     types.BoolValue(n.Occupied),
			OccupantTFID: types.StringValue(n.OccupantTFID),
			X:            types.Float64Value(wholeCm(n.X)),
			Y:            types.Float64Value(wholeCm(n.Y)),
			Z:            types.Float64Value(wholeCm(n.Z)),
			Yaw:          types.Float64Value(n.Yaw),
		}
		if n.BoundsMin != nil {
			m.BoundsMin = &vec3Model{X: types.Float64Value(wholeCm(n.BoundsMin.X)), Y: types.Float64Value(wholeCm(n.BoundsMin.Y)), Z: types.Float64Value(wholeCm(n.BoundsMin.Z))}
		}
		if n.BoundsMax != nil {
			m.BoundsMax = &vec3Model{X: types.Float64Value(wholeCm(n.BoundsMax.X)), Y: types.Float64Value(wholeCm(n.BoundsMax.Y)), Z: types.Float64Value(wholeCm(n.BoundsMax.Z))}
		}
		cfg.Nodes = append(cfg.Nodes, m)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}

// wholeCm rounds a world coordinate to a whole centimetre.
//
// Positions come out of the game as arbitrary doubles (-75539.999099824796).
// Terraform holds a number computed in config, such as `node.x - 600`, at
// 512-bit precision but receives the provider's planned copy of it as a
// double, and it compares the two by their shortest decimal text, which
// differs between the two precisions for any double that is not a short
// decimal. Every such plan fails with "planned value does not match config
// value" (seen live on examples/tier4-factory). Integers print the same at
// any precision, and nothing in the game is placed to a fraction of a
// centimetre, so the data source only ever hands out whole centimetres.
func wholeCm(v float64) float64 { return math.Round(v) }
