package provider

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v3"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &teamRepositoryResource{}
	_ resource.ResourceWithConfigure   = &teamRepositoryResource{}
	_ resource.ResourceWithImportState = &teamRepositoryResource{}
)

type teamRepositoryResource struct {
	client *forgejo.Client
}

type teamRepositoryResourceModel struct {
	ID           types.String `tfsdk:"id"`
	TeamID       types.Int64  `tfsdk:"team_id"`
	RepositoryID types.Int64  `tfsdk:"repository_id"`
}

// NewTeamRepositoryResource returns a resource for an explicit team/repository association.
func NewTeamRepositoryResource() resource.Resource {
	return &teamRepositoryResource{}
}

func (r *teamRepositoryResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team_repository"
}

func (r *teamRepositoryResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Assign a repository to an organization team. The team and repository must belong to the same organization. The team must have `includes_all_repositories = false`; this resource manages an explicit association. Permissions are configured on the team. Destroying this resource removes the association without deleting the team or repository.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Association identifier in team_id/repository_id format.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"team_id": schema.Int64Attribute{
				Description:   "Numeric identifier of the organization team. Changing this forces a new resource to be created.",
				Required:      true,
				Validators:    []validator.Int64{int64validator.AtLeast(1)},
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"repository_id": schema.Int64Attribute{
				Description:   "Numeric identifier of the repository. Changing this forces a new resource to be created.",
				Required:      true,
				Validators:    []validator.Int64{int64validator.AtLeast(1)},
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
		},
	}
}

func (r *teamRepositoryResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *teamRepositoryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data teamRepositoryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	team, _, err := r.client.GetTeam(data.TeamID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read team", err.Error())
		return
	}
	if team.IncludesAllRepositories {
		resp.Diagnostics.AddError("Team includes all repositories", "Set includes_all_repositories = false on the team before managing explicit repository associations.")
		return
	}
	repo, _, err := r.client.GetRepoByID(data.RepositoryID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read repository", err.Error())
		return
	}
	if team.Organization == nil || repo.Owner == nil || team.Organization.ID != repo.Owner.ID {
		resp.Diagnostics.AddError("Organization mismatch", "The team and repository must belong to the same organization.")
		return
	}
	if _, err := r.client.AddTeamRepository(data.TeamID.ValueInt64(), repo.Owner.UserName, repo.Name); err != nil {
		resp.Diagnostics.AddError("Unable to assign repository to team", err.Error())
		return
	}
	data.ID = types.StringValue(fmt.Sprintf("%d/%d", data.TeamID.ValueInt64(), data.RepositoryID.ValueInt64()))
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *teamRepositoryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data teamRepositoryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	team, teamResult, teamErr := r.client.GetTeam(data.TeamID.ValueInt64())
	if teamErr != nil {
		if teamResult != nil && teamResult.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read team", teamErr.Error())
		return
	}
	if team.IncludesAllRepositories {
		resp.Diagnostics.AddError("Team includes all repositories", "Set includes_all_repositories = false on the team before managing explicit repository associations. Implicit access cannot be imported or managed as a separate association.")
		return
	}
	found, result, err := teamHasRepository(r.client, data.TeamID.ValueInt64(), data.RepositoryID.ValueInt64())
	if err != nil {
		if result != nil && result.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read team repository association", err.Error())
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *teamRepositoryResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// Both configurable attributes require replacement.
}

func (r *teamRepositoryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data teamRepositoryResourceModel
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
		resp.Diagnostics.AddError("Missing repository owner", "The repository API response did not include an owner.")
		return
	}
	result, err = r.client.RemoveTeamRepository(data.TeamID.ValueInt64(), repo.Owner.UserName, repo.Name)
	if err != nil && (result == nil || result.StatusCode != http.StatusNotFound) {
		resp.Diagnostics.AddError("Unable to remove repository from team", err.Error())
	}
}

func (r *teamRepositoryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import identifier", "Expected positive numeric IDs in team_id/repository_id format.")
		return
	}
	teamID, teamErr := strconv.ParseInt(parts[0], 10, 64)
	repositoryID, repoErr := strconv.ParseInt(parts[1], 10, 64)
	if teamErr != nil || repoErr != nil || teamID <= 0 || repositoryID <= 0 {
		resp.Diagnostics.AddError("Invalid import identifier", "Expected positive numeric IDs in team_id/repository_id format.")
		return
	}
	data := teamRepositoryResourceModel{
		ID:           types.StringValue(fmt.Sprintf("%d/%d", teamID, repositoryID)),
		TeamID:       types.Int64Value(teamID),
		RepositoryID: types.Int64Value(repositoryID),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// teamHasRepository follows pagination so associations beyond the first page remain managed.
func teamHasRepository(client *forgejo.Client, teamID, repositoryID int64) (bool, *forgejo.Response, error) {
	page := 1
	for {
		repositories, result, err := client.ListTeamRepositories(teamID, forgejo.ListTeamRepositoriesOptions{ListOptions: forgejo.ListOptions{Page: page, PageSize: 50}})
		if err != nil {
			return false, result, err
		}
		for _, repository := range repositories {
			if repository != nil && repository.ID == repositoryID {
				return true, result, nil
			}
		}
		if result == nil || result.NextPage <= page {
			return false, result, nil
		}
		page = result.NextPage
	}
}
