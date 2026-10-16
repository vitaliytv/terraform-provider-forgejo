package provider

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v3"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &tagProtectionResource{}
	_ resource.ResourceWithConfigure   = &tagProtectionResource{}
	_ resource.ResourceWithImportState = &tagProtectionResource{}
)

type tagProtectionResource struct{ client *forgejo.Client }

type tagProtectionResourceModel struct {
	ID                 types.Int64  `tfsdk:"id"`
	RepositoryID       types.Int64  `tfsdk:"repository_id"`
	NamePattern        types.String `tfsdk:"name_pattern"`
	WhitelistUsernames types.Set    `tfsdk:"whitelist_usernames"`
	WhitelistTeams     types.Set    `tfsdk:"whitelist_teams"`
	CreatedAt          types.String `tfsdk:"created_at"`
	UpdatedAt          types.String `tfsdk:"updated_at"`
}

// NewTagProtectionResource returns a repository tag protection resource.
func NewTagProtectionResource() resource.Resource { return &tagProtectionResource{} }

func (r *tagProtectionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tag_protection"
}

func (r *tagProtectionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage a repository tag protection rule. The Forgejo API requires at least one whitelisted user or team when creating a rule. Whitelists may subsequently be cleared by an update; empty whitelists grant no exceptions through this rule. Whitelist entries do not grant repository permissions: listed users and teams must already have repository access. Destroying this resource deletes the protection rule without deleting any tags or the repository.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description:   "Numeric identifier of the tag protection rule.",
				Computed:      true,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"repository_id": schema.Int64Attribute{
				Description:   "Numeric identifier of the repository. Changing this forces a new resource to be created.",
				Required:      true,
				Validators:    []validator.Int64{int64validator.AtLeast(1)},
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"name_pattern": schema.StringAttribute{
				Description: "Pattern matching tag names to protect, for example v*.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					stringvalidator.RegexMatches(regexp.MustCompile(`^\S(?:.*\S)?$`), "must not have leading or trailing whitespace"),
				},
			},
			"whitelist_usernames": schema.SetAttribute{
				Description: "Users with repository access allowed to create or modify matching tags.",
				Optional:    true, Computed: true, ElementType: types.StringType,
				Default: setdefault.StaticValue(types.SetValueMust(types.StringType, nil)),
			},
			"whitelist_teams": schema.SetAttribute{
				Description: "Organization teams with repository access allowed to create or modify matching tags. The Forgejo API only returns whitelisted teams that have access to the repository.",
				Optional:    true, Computed: true, ElementType: types.StringType,
				Default: setdefault.StaticValue(types.SetValueMust(types.StringType, nil)),
			},
			"created_at": schema.StringAttribute{Description: "Creation time of the rule.", Computed: true},
			"updated_at": schema.StringAttribute{Description: "Last update time of the rule.", Computed: true},
		},
	}
}

func (r *tagProtectionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*forgejo.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *forgejo.Client, got: %T.", req.ProviderData))
		return
	}
	r.client = client
}

func (r *tagProtectionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data tagProtectionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	repo, diags := getRepositoryByID(ctx, r.client, data.RepositoryID.ValueInt64())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if repo.Owner == nil {
		resp.Diagnostics.AddError("Missing repository owner", "The API response did not include a repository owner.")
		return
	}
	opts := forgejo.CreateTagProtectionOption{NamePattern: data.NamePattern.ValueString()}
	resp.Diagnostics.Append(data.WhitelistUsernames.ElementsAs(ctx, &opts.WhitelistUsernames, false)...)
	resp.Diagnostics.Append(data.WhitelistTeams.ElementsAs(ctx, &opts.WhitelistTeams, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(opts.WhitelistUsernames) == 0 && len(opts.WhitelistTeams) == 0 {
		resp.Diagnostics.AddError("Empty whitelist on tag protection creation", "The Forgejo API requires at least one whitelisted user or team when creating a rule. To manage a rule with empty whitelists, clear them in a subsequent update or import an existing rule. A team with no members can be used to grant no user exceptions on creation.")
		return
	}
	rule, _, err := r.client.CreateTagProtection(repo.Owner.UserName, repo.Name, opts)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create tag protection", err.Error())
		return
	}
	resp.Diagnostics.Append(data.from(ctx, rule)...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	}
}

func (r *tagProtectionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data tagProtectionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	repo, result, err := r.client.GetRepoByID(data.RepositoryID.ValueInt64())
	if err != nil {
		if result != nil && result.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read repository", err.Error())
		return
	}
	if repo.Owner == nil {
		resp.Diagnostics.AddError("Missing repository owner", "The API response did not include a repository owner.")
		return
	}
	rule, result, err := r.client.GetTagProtection(repo.Owner.UserName, repo.Name, data.ID.ValueInt64())
	if err != nil {
		if result != nil && result.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read tag protection", err.Error())
		return
	}
	resp.Diagnostics.Append(data.from(ctx, rule)...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	}
}

func (r *tagProtectionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data tagProtectionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	repo, diags := getRepositoryByID(ctx, r.client, data.RepositoryID.ValueInt64())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if repo.Owner == nil {
		resp.Diagnostics.AddError("Missing repository owner", "The API response did not include a repository owner.")
		return
	}
	opts := forgejo.EditTagProtectionOption{NamePattern: data.NamePattern.ValueStringPointer()}
	resp.Diagnostics.Append(data.WhitelistUsernames.ElementsAs(ctx, &opts.WhitelistUsernames, false)...)
	resp.Diagnostics.Append(data.WhitelistTeams.ElementsAs(ctx, &opts.WhitelistTeams, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rule, _, err := r.client.EditTagProtection(repo.Owner.UserName, repo.Name, data.ID.ValueInt64(), opts)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update tag protection", err.Error())
		return
	}
	resp.Diagnostics.Append(data.from(ctx, rule)...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	}
}

func (r *tagProtectionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data tagProtectionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	repo, result, err := r.client.GetRepoByID(data.RepositoryID.ValueInt64())
	if err != nil {
		if result != nil && result.StatusCode == http.StatusNotFound {
			return
		}
		resp.Diagnostics.AddError("Unable to read repository", err.Error())
		return
	}
	if repo.Owner == nil {
		resp.Diagnostics.AddError("Missing repository owner", "The API response did not include a repository owner.")
		return
	}
	result, err = r.client.DeleteTagProtection(repo.Owner.UserName, repo.Name, data.ID.ValueInt64())
	if err != nil && (result == nil || result.StatusCode != http.StatusNotFound) {
		resp.Diagnostics.AddError("Unable to delete tag protection", err.Error())
	}
}

func (r *tagProtectionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import identifier", "Expected positive numeric IDs in repository_id/protection_id format.")
		return
	}
	repositoryID, repoErr := strconv.ParseInt(parts[0], 10, 64)
	id, idErr := strconv.ParseInt(parts[1], 10, 64)
	if repoErr != nil || idErr != nil || repositoryID <= 0 || id <= 0 {
		resp.Diagnostics.AddError("Invalid import identifier", "Expected positive numeric IDs in repository_id/protection_id format.")
		return
	}
	data := tagProtectionResourceModel{
		ID: types.Int64Value(id), RepositoryID: types.Int64Value(repositoryID),
		WhitelistUsernames: types.SetNull(types.StringType), WhitelistTeams: types.SetNull(types.StringType),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (m *tagProtectionResourceModel) from(ctx context.Context, rule *forgejo.TagProtection) diag.Diagnostics {
	var diags diag.Diagnostics
	if rule == nil {
		diags.AddError("Missing tag protection", "The API response did not include the tag protection rule.")
		return diags
	}
	m.ID = types.Int64Value(rule.ID)
	m.NamePattern = types.StringValue(rule.NamePattern)
	m.CreatedAt = types.StringValue(rule.Created.Format(time.RFC3339))
	m.UpdatedAt = types.StringValue(rule.Updated.Format(time.RFC3339))
	var d diag.Diagnostics
	m.WhitelistUsernames, d = types.SetValueFrom(ctx, types.StringType, append([]string{}, rule.WhitelistUsernames...))
	diags.Append(d...)
	m.WhitelistTeams, d = types.SetValueFrom(ctx, types.StringType, append([]string{}, rule.WhitelistTeams...))
	diags.Append(d...)
	return diags
}
