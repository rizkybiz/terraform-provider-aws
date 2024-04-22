// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package rekognition

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rekognition"
	awstypes "github.com/aws/aws-sdk-go-v2/service/rekognition/types"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-provider-aws/internal/create"
	"github.com/hashicorp/terraform-provider-aws/internal/enum"
	"github.com/hashicorp/terraform-provider-aws/internal/errs"
	"github.com/hashicorp/terraform-provider-aws/internal/framework"
	"github.com/hashicorp/terraform-provider-aws/internal/framework/flex"
	fwtypes "github.com/hashicorp/terraform-provider-aws/internal/framework/types"
	"github.com/hashicorp/terraform-provider-aws/internal/tfresource"
	"github.com/hashicorp/terraform-provider-aws/names"
)

// @FrameworkResource("aws_rekognition_dataset", name="Dataset")
func newResourceDataset(_ context.Context) (resource.ResourceWithConfigure, error) {
	r := &resourceDataset{}
	r.SetDefaultCreateTimeout(30 * time.Minute)
	r.SetDefaultUpdateTimeout(30 * time.Minute)
	r.SetDefaultDeleteTimeout(30 * time.Minute)

	return r, nil
}

const (
	ResNameDataset = "Dataset"
)

type resourceDataset struct {
	framework.ResourceWithConfigure
	framework.WithTimeouts
}

func (r *resourceDataset) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "aws_rekognition_dataset"
}

func (r *resourceDataset) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"arn": framework.ARNAttributeComputedOnly(),
			"project_arn": schema.StringAttribute{
				Required:   true,
				CustomType: fwtypes.ARNType,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 2048),
				},
				Description: "The ARN of the Amazon Rekognition Custom Labels project to which you want to assign the dataset.",
			},
			"type": schema.StringAttribute{
				Required:    true,
				CustomType:  fwtypes.StringEnumType[awstypes.DatasetType](),
				Description: "The type of the dataset. Specify TRAIN to create a training dataset. Specify TEST to create a test dataset.",
			},
		},
	}
}

func (r *resourceDataset) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {

	conn := r.Meta().RekognitionClient(ctx)

	var plan resourceDatasetData
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := &rekognition.CreateDatasetInput{
		DatasetType: plan.DatasetType.ValueEnum(),
		ProjectArn:  aws.String(plan.ProjectArn.String()),
	}

	// The only defining information we have at this point is the ProjectArn
	out, err := conn.CreateDataset(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError(
			create.ProblemStandardMessage(names.Rekognition, create.ErrActionCreating, ResNameDataset, plan.ProjectArn.String(), err),
			err.Error(),
		)
		return
	}
	if out == nil || out.DatasetArn == nil {
		resp.Diagnostics.AddError(
			create.ProblemStandardMessage(names.Rekognition, create.ErrActionCreating, ResNameDataset, plan.ProjectArn.String(), nil),
			errors.New("empty output").Error(),
		)
		return
	}

	plan.ARN = flex.StringToFramework(ctx, out.DatasetArn)

	createTimeout := r.CreateTimeout(ctx, plan.Timeouts)
	_, err = waitDatasetCreated(ctx, conn, plan.ARN.ValueString(), createTimeout)
	if err != nil {
		resp.Diagnostics.AddError(
			create.ProblemStandardMessage(names.Rekognition, create.ErrActionWaitingForCreation, ResNameDataset, plan.ARN.String(), err),
			err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *resourceDataset) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// TIP: ==== RESOURCE READ ====
	// Generally, the Read function should do the following things. Make
	// sure there is a good reason if you don't do one of these.
	//
	// 1. Get a client connection to the relevant service
	// 2. Fetch the state
	// 3. Get the resource from AWS
	// 4. Remove resource from state if it is not found
	// 5. Set the arguments and attributes
	// 6. Set the state

	// TIP: -- 1. Get a client connection to the relevant service
	conn := r.Meta().RekognitionClient(ctx)

	var state resourceDatasetData
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// TIP: -- 3. Get the resource from AWS using an API Get, List, or Describe-
	// type function, or, better yet, using a finder.
	out, err := findDatasetByID(ctx, conn, state.ID.ValueString())
	// TIP: -- 4. Remove resource from state if it is not found
	if tfresource.NotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(
			create.ProblemStandardMessage(names.Rekognition, create.ErrActionSetting, ResNameDataset, state.ID.String(), err),
			err.Error(),
		)
		return
	}

	// TIP: -- 5. Set the arguments and attributes
	//
	// For simple data types (i.e., schema.StringAttribute, schema.BoolAttribute,
	// schema.Int64Attribute, and schema.Float64Attribue), simply setting the
	// appropriate data struct field is sufficient. The flex package implements
	// helpers for converting between Go and Plugin-Framework types seamlessly. No
	// error or nil checking is necessary.
	//
	// However, there are some situations where more handling is needed such as
	// complex data types (e.g., schema.ListAttribute, schema.SetAttribute). In
	// these cases the flatten function may have a diagnostics return value, which
	// should be appended to resp.Diagnostics.
	state.ARN = flex.StringToFramework(ctx, out.Arn)
	state.ID = flex.StringToFramework(ctx, out.DatasetId)
	state.Name = flex.StringToFramework(ctx, out.DatasetName)
	state.Type = flex.StringToFramework(ctx, out.DatasetType)

	// TIP: Setting a complex type.
	complexArgument, d := flattenComplexArgument(ctx, out.ComplexArgument)
	resp.Diagnostics.Append(d...)
	state.ComplexArgument = complexArgument

	// TIP: -- 6. Set the state
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *resourceDataset) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// TIP: ==== RESOURCE UPDATE ====
	// Not all resources have Update functions. There are a few reasons:
	// a. The AWS API does not support changing a resource
	// b. All arguments have RequiresReplace() plan modifiers
	// c. The AWS API uses a create call to modify an existing resource
	//
	// In the cases of a. and b., the resource will not have an update method
	// defined. In the case of c., Update and Create can be refactored to call
	// the same underlying function.
	//
	// The rest of the time, there should be an Update function and it should
	// do the following things. Make sure there is a good reason if you don't
	// do one of these.
	//
	// 1. Get a client connection to the relevant service
	// 2. Fetch the plan and state
	// 3. Populate a modify input structure and check for changes
	// 4. Call the AWS modify/update function
	// 5. Use a waiter to wait for update to complete
	// 6. Save the request plan to response state
	// TIP: -- 1. Get a client connection to the relevant service
	conn := r.Meta().RekognitionClient(ctx)

	// TIP: -- 2. Fetch the plan
	var plan, state resourceDatasetData
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// TIP: -- 3. Populate a modify input structure and check for changes
	if !plan.Name.Equal(state.Name) ||
		!plan.Description.Equal(state.Description) ||
		!plan.ComplexArgument.Equal(state.ComplexArgument) ||
		!plan.Type.Equal(state.Type) {

		in := &rekognition.UpdateDatasetInput{
			// TIP: Mandatory or fields that will always be present can be set when
			// you create the Input structure. (Replace these with real fields.)
			DatasetId:   aws.String(plan.ID.ValueString()),
			DatasetName: aws.String(plan.Name.ValueString()),
			DatasetType: aws.String(plan.Type.ValueString()),
		}

		if !plan.Description.IsNull() {
			// TIP: Optional fields should be set based on whether or not they are
			// used.
			in.Description = aws.String(plan.Description.ValueString())
		}
		if !plan.ComplexArgument.IsNull() {
			// TIP: Use an expander to assign a complex argument. The elements must be
			// deserialized into the appropriate struct before being passed to the expander.
			var tfList []complexArgumentData
			resp.Diagnostics.Append(plan.ComplexArgument.ElementsAs(ctx, &tfList, false)...)
			if resp.Diagnostics.HasError() {
				return
			}

			in.ComplexArgument = expandComplexArgument(tfList)
		}

		// TIP: -- 4. Call the AWS modify/update function
		out, err := conn.UpdateDataset(ctx, in)
		if err != nil {
			resp.Diagnostics.AddError(
				create.ProblemStandardMessage(names.Rekognition, create.ErrActionUpdating, ResNameDataset, plan.ID.String(), err),
				err.Error(),
			)
			return
		}
		if out == nil || out.Dataset == nil {
			resp.Diagnostics.AddError(
				create.ProblemStandardMessage(names.Rekognition, create.ErrActionUpdating, ResNameDataset, plan.ID.String(), nil),
				errors.New("empty output").Error(),
			)
			return
		}

		// TIP: Using the output from the update function, re-set any computed attributes
		plan.ARN = flex.StringToFramework(ctx, out.Dataset.Arn)
		plan.ID = flex.StringToFramework(ctx, out.Dataset.DatasetId)
	}

	// TIP: -- 5. Use a waiter to wait for update to complete
	updateTimeout := r.UpdateTimeout(ctx, plan.Timeouts)
	_, err := waitDatasetUpdated(ctx, conn, plan.ID.ValueString(), updateTimeout)
	if err != nil {
		resp.Diagnostics.AddError(
			create.ProblemStandardMessage(names.Rekognition, create.ErrActionWaitingForUpdate, ResNameDataset, plan.ID.String(), err),
			err.Error(),
		)
		return
	}

	// TIP: -- 6. Save the request plan to response state
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *resourceDataset) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	conn := r.Meta().RekognitionClient(ctx)

	// TIP: -- 2. Fetch the state
	var state resourceDatasetData
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// TIP: -- 3. Populate a delete input structure
	in := &rekognition.DeleteDatasetInput{
		DatasetArn: aws.String(state.ARN.String()),
	}
	_, err := conn.DeleteDataset(ctx, in)

	if err != nil {
		if errs.IsA[*awstypes.ResourceNotFoundException](err) {
			return
		}
		resp.Diagnostics.AddError(
			create.ProblemStandardMessage(names.Rekognition, create.ErrActionDeleting, ResNameDataset, state.ID.String(), err),
			err.Error(),
		)
		return
	}

	deleteTimeout := r.DeleteTimeout(ctx, state.Timeouts)
	_, err = waitDatasetDeleted(ctx, conn, state.ARN.ValueString(), deleteTimeout)
	if err != nil {
		resp.Diagnostics.AddError(
			create.ProblemStandardMessage(names.Rekognition, create.ErrActionWaitingForDeletion, ResNameDataset, state.ID.String(), err),
			err.Error(),
		)
		return
	}
}

// TIP: ==== TERRAFORM IMPORTING ====
// If Read can get all the information it needs from the Identifier
// (i.e., path.Root("id")), you can use the PassthroughID importer. Otherwise,
// you'll need a custom import function.
//
// See more:
// https://developer.hashicorp.com/terraform/plugin/framework/resources/import
func (r *resourceDataset) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

const (
	statusChangePending           = awstypes.DatasetStatusUpdateInProgress
	statusDeleting                = awstypes.DatasetStatusDeleteInProgress
	statusCreated                 = awstypes.DatasetStatusCreateComplete
	statusCreationPending         = awstypes.DatasetStatusCreateInProgress
	statusUpdated                 = awstypes.DatasetStatusUpdateComplete
	datasetStatusUpdateInProgress = awstypes.DatasetStatusUpdateInProgress
)

func waitDatasetCreated(ctx context.Context, conn *rekognition.Client, id string, timeout time.Duration) (*awstypes.DatasetDescription, error) {
	stateConf := &retry.StateChangeConf{
		Pending:                   enum.Slice(statusCreationPending),
		Target:                    enum.Slice(statusCreated),
		Refresh:                   statusDataset(ctx, conn, id),
		Timeout:                   timeout,
		NotFoundChecks:            20,
		ContinuousTargetOccurence: 2,
	}

	outputRaw, err := stateConf.WaitForStateContext(ctx)
	if out, ok := outputRaw.(*awstypes.DatasetDescription); ok {
		return out, err
	}

	return nil, err
}

func waitDatasetUpdated(ctx context.Context, conn *rekognition.Client, id string, timeout time.Duration) (*awstypes.DatasetDescription, error) {
	stateConf := &retry.StateChangeConf{
		Pending:                   enum.Slice(datasetStatusUpdateInProgress),
		Target:                    enum.Slice(statusUpdated),
		Refresh:                   statusDataset(ctx, conn, id),
		Timeout:                   timeout,
		NotFoundChecks:            20,
		ContinuousTargetOccurence: 2,
	}

	outputRaw, err := stateConf.WaitForStateContext(ctx)
	if out, ok := outputRaw.(*awstypes.DatasetDescription); ok {
		return out, err
	}

	return nil, err
}

// TIP: A deleted waiter is almost like a backwards created waiter. There may
// be additional pending states, however.
func waitDatasetDeleted(ctx context.Context, conn *rekognition.Client, id string, timeout time.Duration) (*awstypes.Dataset, error) {
	stateConf := &retry.StateChangeConf{
		Pending: []string{statusDeleting, statusNormal},
		Target:  []string{},
		Refresh: statusDataset(ctx, conn, id),
		Timeout: timeout,
	}

	outputRaw, err := stateConf.WaitForStateContext(ctx)
	if out, ok := outputRaw.(*rekognition.Dataset); ok {
		return out, err
	}

	return nil, err
}

func statusDataset(ctx context.Context, conn *rekognition.Client, id string) retry.StateRefreshFunc {
	return func() (interface{}, string, error) {
		out, err := findDatasetByID(ctx, conn, id)
		if tfresource.NotFound(err) {
			return nil, "", nil
		}

		if err != nil {
			return nil, "", err
		}

		return out, string(out.Status), nil
	}
}

func findDatasetByID(ctx context.Context, conn *rekognition.Client, arn string) (*awstypes.DatasetDescription, error) {
	in := &rekognition.DescribeDatasetInput{
		DatasetArn: aws.String(arn),
	}

	out, err := conn.DescribeDataset(ctx, in)
	if err != nil {
		if errs.IsA[*awstypes.ResourceNotFoundException](err) {
			return nil, &retry.NotFoundError{
				LastError:   err,
				LastRequest: in,
			}
		}

		return nil, err
	}

	if out == nil || out.DatasetDescription == nil {
		return nil, tfresource.NewEmptyResultError(in)
	}

	return out.DatasetDescription, nil
}

type resourceDatasetData struct {
	ARN           types.String                             `tfsdk:"arn"`
	DatasetSource awstypes.DatasetSource                   `tfsdk:"source"`
	DatasetType   fwtypes.StringEnum[awstypes.DatasetType] `tfsdk:"type"`
	ProjectArn    types.String                             `tfsdk:"project_arn"`
	Timeouts      timeouts.Value                           `tfsdk:"timeouts"`
}
