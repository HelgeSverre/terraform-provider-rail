package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/HelgeSverre/terraform-provider-rail/internal/rail"
)

var (
	_ datasource.DataSourceWithConfigure = (*topologyDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*traceDataSource)(nil)
)

func configureClient(req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) *rail.Client {
	if req.ProviderData == nil {
		return nil
	}
	client, ok := req.ProviderData.(*rail.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("got %T", req.ProviderData))
	}
	return client
}

// rail_topology

func newTopologyDataSource() datasource.DataSource { return &topologyDataSource{} }

type topologyDataSource struct{ client *rail.Client }

type topologyModel struct {
	ID        types.String `tfsdk:"id"`
	Instance  types.String `tfsdk:"instance"`
	PublicURL types.String `tfsdk:"public_url"`
	Upstreams types.List   `tfsdk:"upstreams"`
}

func (d *topologyDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_topology"
}

func (d *topologyDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The configured tier and its direct upstreams, for audit and capacity-planning purposes.",
		Attributes: map[string]schema.Attribute{
			"id":         schema.StringAttribute{Computed: true, MarkdownDescription: "The tier's per-process identity."},
			"instance":   schema.StringAttribute{Computed: true, MarkdownDescription: "The tier's name."},
			"public_url": schema.StringAttribute{Computed: true, MarkdownDescription: "Base URL links are issued under."},
			"upstreams": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Vendor identifiers, including nested tiers as `rail+<url>`.",
			},
		},
	}
}

func (d *topologyDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req, resp)
}

func (d *topologyDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	t, err := d.client.Topology(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Could not read topology", err.Error())
		return
	}
	upstreams, diags := types.ListValueFrom(ctx, types.StringType, t.Upstreams)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, topologyModel{
		ID:        types.StringValue(t.ID),
		Instance:  types.StringValue(t.Instance),
		PublicURL: types.StringValue(t.PublicURL),
		Upstreams: upstreams,
	})...)
}

// rail_trace

func newTraceDataSource() datasource.DataSource { return &traceDataSource{} }

type traceDataSource struct{ client *rail.Client }

type traceModel struct {
	LinkID    types.String `tfsdk:"link_id"`
	Redirects types.Int64  `tfsdk:"redirects"`
	Headroom  types.Int64  `tfsdk:"headroom"`
	GaveUp    types.Bool   `tfsdk:"gave_up"`
	Hops      []hopModel   `tfsdk:"hops"`
}

type hopModel struct {
	URL       types.String `tfsdk:"url"`
	Status    types.Int64  `tfsdk:"status"`
	LatencyMs types.Int64  `tfsdk:"latency_ms"`
	Instance  types.String `tfsdk:"instance"`
	Error     types.String `tfsdk:"error"`
}

func (d *traceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_trace"
}

func (d *traceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Resolves a link hop by hop, as a browser would. Use it in a `check` block " +
			"to assert that the redirect budget is not exceeded.",
		Attributes: map[string]schema.Attribute{
			"link_id":   schema.StringAttribute{Required: true, MarkdownDescription: "Id of the link to trace."},
			"redirects": schema.Int64Attribute{Computed: true, MarkdownDescription: "Redirects in the chain."},
			"headroom": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Redirects remaining before browsers give up (`ERR_TOO_MANY_REDIRECTS`).",
			},
			"gave_up": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether a browser would have given up."},
			"hops": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Every URL requested, in order.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"url":        schema.StringAttribute{Computed: true, MarkdownDescription: "URL requested."},
						"status":     schema.Int64Attribute{Computed: true, MarkdownDescription: "HTTP status; null if the request failed."},
						"latency_ms": schema.Int64Attribute{Computed: true, MarkdownDescription: "Time to response."},
						"instance":   schema.StringAttribute{Computed: true, MarkdownDescription: "RAIL tier that answered, if any."},
						"error":      schema.StringAttribute{Computed: true, MarkdownDescription: "Why the hop failed, if it did."},
					},
				},
			},
		},
	}
}

func (d *traceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req, resp)
}

func (d *traceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var model traceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	t, err := d.client.Trace(ctx, model.LinkID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Could not trace link", err.Error())
		return
	}
	model.Redirects = types.Int64Value(t.Redirects)
	model.Headroom = types.Int64Value(t.Headroom)
	model.GaveUp = types.BoolValue(t.GaveUp)
	model.Hops = make([]hopModel, 0, len(t.Hops))
	for _, h := range t.Hops {
		model.Hops = append(model.Hops, hopModel{
			URL:       types.StringValue(h.URL),
			Status:    types.Int64PointerValue(h.Status),
			LatencyMs: types.Int64Value(h.LatencyMs),
			Instance:  types.StringPointerValue(h.Instance),
			Error:     types.StringPointerValue(h.Error),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}
