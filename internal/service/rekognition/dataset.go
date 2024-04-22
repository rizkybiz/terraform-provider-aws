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
	framework.WithNoUpdate
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

// AWS SDK does not return enough information to reliably import this resource (specifically ProjectArn)
// func (r *resourceDataset) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
// 	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
// }

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
	conn := r.Meta().RekognitionClient(ctx)

	var state resourceDatasetData
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := findDatasetByID(ctx, conn, state.ARN.String())

	if tfresource.NotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(
			create.ProblemStandardMessage(names.Rekognition, create.ErrActionSetting, ResNameDataset, state.ARN.String(), err),
			err.Error(),
		)
		return
	}

	// what do we put into state on a read?

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *resourceDataset) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	conn := r.Meta().RekognitionClient(ctx)

	var state resourceDatasetData
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := &rekognition.DeleteDatasetInput{
		DatasetArn: aws.String(state.ARN.String()),
	}
	_, err := conn.DeleteDataset(ctx, in)

	if err != nil {
		if errs.IsA[*awstypes.ResourceNotFoundException](err) {
			return
		}
		resp.Diagnostics.AddError(
			create.ProblemStandardMessage(names.Rekognition, create.ErrActionDeleting, ResNameDataset, state.ARN.String(), err),
			err.Error(),
		)
		return
	}

	deleteTimeout := r.DeleteTimeout(ctx, state.Timeouts)
	_, err = waitDatasetDeleted(ctx, conn, state.ARN.ValueString(), deleteTimeout)
	if err != nil {
		resp.Diagnostics.AddError(
			create.ProblemStandardMessage(names.Rekognition, create.ErrActionWaitingForDeletion, ResNameDataset, state.ARN.String(), err),
			err.Error(),
		)
		return
	}
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

func waitDatasetDeleted(ctx context.Context, conn *rekognition.Client, id string, timeout time.Duration) (*awstypes.DatasetDescription, error) {
	stateConf := &retry.StateChangeConf{
		Pending: enum.Slice(statusDeleting),
		Target:  []string{},
		Refresh: statusDataset(ctx, conn, id),
		Timeout: timeout,
	}

	outputRaw, err := stateConf.WaitForStateContext(ctx)
	if out, ok := outputRaw.(*awstypes.DatasetDescription); ok {
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
