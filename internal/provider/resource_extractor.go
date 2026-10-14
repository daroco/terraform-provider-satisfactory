package provider

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/daroco/terraform-provider-satisfactory/internal/api"
	"github.com/daroco/terraform-provider-satisfactory/internal/client"
)

// extractorResource places miners, water extractors and the like. Unlike a
// building it has no coordinates of its own: it is bound to a world feature
// (node_id, from the satisfactory_resource_nodes data source) and the game
// decides where on that feature it stands. The transform it got is reported
// back as computed attributes.
type extractorResource struct {
	client *client.Client
}

func newExtractorResource() resource.Resource { return &extractorResource{} }

type extractorModel struct {
	ID         types.String  `tfsdk:"id"`
	Class      types.String  `tfsdk:"class"`
	NodeID     types.String  `tfsdk:"node_id"`
	X          types.Float64 `tfsdk:"x"`
	Y          types.Float64 `tfsdk:"y"`
	Z          types.Float64 `tfsdk:"z"`
	Yaw        types.Float64 `tfsdk:"yaw"`
	ClockSpeed types.Float64 `tfsdk:"clock_speed"`
}

func (r *extractorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_extractor"
}

func (r *extractorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A miner, water extractor or other resource extractor, placed on a world feature " +
			"(resource node, geyser or water volume) rather than at a coordinate. Find the feature with the " +
			"`satisfactory_resource_nodes` data source. One extractor per node; the node stays occupied until " +
			"this is destroyed.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Terraform-assigned stable ID, persisted in the save game by the mod.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"class": schema.StringAttribute{
				Required:      true,
				Description:   "Extractor class name, e.g. Build_MinerMk1_C, Build_MinerMk2_C or Build_WaterPump_C.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"node_id": schema.StringAttribute{
				Required:      true,
				Description:   "Id of the feature to bind to, from `satisfactory_resource_nodes.nodes[*].id`.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"x": schema.Float64Attribute{
				Optional: true,
				Computed: true,
				Description: "World X in centimetres. Settable only for water volumes, which are areas: " +
					"pick a point on the surface within the volume's bounds. For a node the game decides " +
					"and this reports where it landed.",
				PlanModifiers: []planmodifier.Float64{float64planmodifier.RequiresReplace(), float64planmodifier.UseStateForUnknown()},
			},
			"y": schema.Float64Attribute{
				Optional:      true,
				Computed:      true,
				Description:   "World Y in centimetres. See x.",
				PlanModifiers: []planmodifier.Float64{float64planmodifier.RequiresReplace(), float64planmodifier.UseStateForUnknown()},
			},
			"z": schema.Float64Attribute{
				Computed:      true,
				Description:   "World Z in centimetres, as placed by the game.",
				PlanModifiers: []planmodifier.Float64{float64planmodifier.UseStateForUnknown()},
			},
			"yaw": schema.Float64Attribute{
				Optional:      true,
				Computed:      true,
				Description:   "Rotation in degrees. Omitted, the feature's own placement rotation is used and reported here.",
				PlanModifiers: []planmodifier.Float64{float64planmodifier.RequiresReplace(), float64planmodifier.UseStateForUnknown()},
			},
			"clock_speed": schema.Float64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Clock speed as a fraction (1.0 = 100%). Changeable in place. Defaults to 1.0 on create.",
			},
		},
	}
}

func (r *extractorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, resp)
}

// placement carries only what the config set; the mod fills the rest from
// the feature. Unknown values (computed from something not yet applied)
// are treated as unset.
func (m extractorModel) placement() *api.Placement {
	p := &api.Placement{}
	set := false
	if !m.X.IsNull() && !m.X.IsUnknown() {
		v := m.X.ValueFloat64()
		p.X, set = &v, true
	}
	if !m.Y.IsNull() && !m.Y.IsUnknown() {
		v := m.Y.ValueFloat64()
		p.Y, set = &v, true
	}
	if !m.Yaw.IsNull() && !m.Yaw.IsUnknown() {
		v := m.Yaw.ValueFloat64()
		p.Yaw, set = &v, true
	}
	if !set {
		return nil
	}
	return p
}

func (m extractorModel) toAPI() api.Buildable {
	return api.Buildable{
		TFID:       m.ID.ValueString(),
		Class:      m.Class.ValueString(),
		NodeID:     m.NodeID.ValueString(),
		Placement:  m.placement(),
		ClockSpeed: m.ClockSpeed.ValueFloat64(),
	}
}

func (m extractorModel) toPreview() api.Preview {
	return api.Preview{
		Class:     m.Class.ValueString(),
		NodeID:    m.NodeID.ValueString(),
		Placement: m.placement(),
	}
}

func extractorFromAPI(b api.Buildable) extractorModel {
	return extractorModel{
		ID:         types.StringValue(b.TFID),
		Class:      types.StringValue(b.Class),
		NodeID:     types.StringValue(b.NodeID),
		X:          types.Float64Value(b.Transform.X),
		Y:          types.Float64Value(b.Transform.Y),
		Z:          types.Float64Value(b.Transform.Z),
		Yaw:        types.Float64Value(b.Transform.Yaw),
		ClockSpeed: types.Float64Value(b.ClockSpeed),
	}
}

// ModifyPlan shows the planned extractor as a hologram on its node when
// previewing is on. See preview.go.
func (r *extractorResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil || !r.client.PreviewEnabled() || req.Plan.Raw.IsNull() {
		return
	}
	var plan extractorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || plan.Class.IsUnknown() || plan.NodeID.IsUnknown() {
		return
	}
	if !req.State.Raw.IsNull() {
		var state extractorModel
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
		// Only a create or a replace puts something new in the world.
		if state.Class.Equal(plan.Class) && state.NodeID.Equal(plan.NodeID) &&
			state.X.Equal(plan.X) && state.Y.Equal(plan.Y) && state.Yaw.Equal(plan.Yaw) {
			return
		}
	}
	what := fmt.Sprintf("%s on node %s", plan.Class.ValueString(), plan.NodeID.ValueString())
	previewPlacement(ctx, r.client, what, plan.toPreview(), &resp.Diagnostics)
}

func (r *extractorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan extractorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = types.StringValue(uuid.NewString())
	if plan.ClockSpeed.IsUnknown() || plan.ClockSpeed.IsNull() {
		// Applied here rather than as a schema Default for the same reason
		// as satisfactory_building: the response is the source of truth.
		plan.ClockSpeed = types.Float64Value(1.0)
	}
	clearPreview(ctx, r.client, plan.toPreview())
	created, err := r.client.CreateBuildable(ctx, plan.toAPI())
	if err != nil {
		resp.Diagnostics.AddError("Failed to place extractor", err.Error())
		return
	}
	next := extractorFromAPI(created)
	next.X = preserveWithinEpsilon(plan.X, next.X)
	next.Y = preserveWithinEpsilon(plan.Y, next.Y)
	next.Yaw = preserveWithinEpsilon(plan.Yaw, next.Yaw)
	resp.Diagnostics.Append(resp.State.Set(ctx, next)...)
}

func (r *extractorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state extractorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	b, err := r.client.GetBuildable(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read extractor", err.Error())
		return
	}
	next := extractorFromAPI(b)
	next.X = preserveWithinEpsilon(state.X, next.X)
	next.Y = preserveWithinEpsilon(state.Y, next.Y)
	next.Z = preserveWithinEpsilon(state.Z, next.Z)
	next.Yaw = preserveWithinEpsilon(state.Yaw, next.Yaw)
	resp.Diagnostics.Append(resp.State.Set(ctx, next)...)
}

func (r *extractorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan extractorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	clock := plan.ClockSpeed.ValueFloat64()
	updated, err := r.client.PatchBuildable(ctx, plan.ID.ValueString(), api.BuildablePatch{ClockSpeed: &clock})
	if err != nil {
		resp.Diagnostics.AddError("Failed to update extractor", err.Error())
		return
	}
	next := extractorFromAPI(updated)
	next.X = preserveWithinEpsilon(plan.X, next.X)
	next.Y = preserveWithinEpsilon(plan.Y, next.Y)
	next.Z = preserveWithinEpsilon(plan.Z, next.Z)
	next.Yaw = preserveWithinEpsilon(plan.Yaw, next.Yaw)
	resp.Diagnostics.Append(resp.State.Set(ctx, next)...)
}

func (r *extractorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state extractorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteBuildable(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to dismantle extractor", err.Error())
	}
}

func (r *extractorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
