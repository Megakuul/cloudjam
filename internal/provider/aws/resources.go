package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codeberg.org/megakuul/cloudjam/internal/provider"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
)

type ResourceController struct {
	client *cloudcontrol.Client
}

// NewResourceController creates a aws resource controller.
// Usually you would create the resource controller from provider.Resources(), this is just for external tooling.
func NewResourceController(client *cloudcontrol.Client) *ResourceController {
	return &ResourceController{client}
}

func (p *Provider) Resources(ctx context.Context, id string, lifetime time.Duration) (provider.ResourceController, error) {
	cfg, err := p.assume(ctx, id, p.adminRole, lifetime)
	if err != nil {
		return nil, err
	}
	return &ResourceController{
		client: cloudcontrol.NewFromConfig(cfg),
	}, nil
}

// await blocks until a cloud control request settles and returns its final progress event.
//
// The sdk waiter collapses every terminal non-success state into "waiter state
// transitioned to Failure" and throws away the response that carries the handler
// error code and status message. The event is therefore refetched on failure to
// build an error that actually names the problem.
func (r *ResourceController) await(
	ctx context.Context, resourceType string, token *string, timeout time.Duration,
) (*types.ProgressEvent, error) {
	input := &cloudcontrol.GetResourceRequestStatusInput{RequestToken: token}

	status, waitErr := cloudcontrol.NewResourceRequestSuccessWaiter(r.client).WaitForOutput(ctx, input, timeout)
	if waitErr == nil && status.ProgressEvent != nil {
		return status.ProgressEvent, nil
	}

	resp, err := r.client.GetResourceRequestStatus(ctx, input)
	if err != nil || resp.ProgressEvent == nil {
		// the request status is unreachable, usually because the context is gone;
		// the waiter error is all there is to report.
		return nil, fmt.Errorf("%s request %q: %w", resourceType, awssdk.ToString(token), waitErr)
	}
	event := resp.ProgressEvent
	operation := strings.ToLower(string(event.Operation))

	// the request can settle in the window between the waiter giving up and the
	// refetch, in which case there is nothing to report.
	if event.OperationStatus == types.OperationStatusSuccess {
		return event, nil
	}
	// the waiter ran out its own deadline while cloud control is still reconciling.
	if event.OperationStatus == types.OperationStatusPending ||
		event.OperationStatus == types.OperationStatusInProgress {
		return nil, fmt.Errorf("%s %s request %q: still %s after %s",
			operation, resourceType, awssdk.ToString(token), event.OperationStatus, timeout)
	}

	message := strings.TrimSpace(awssdk.ToString(event.StatusMessage))
	if message == "" {
		message = "no status message returned"
	}
	if event.ErrorCode != "" {
		return nil, fmt.Errorf("%s %s request %q: %s (%s): %s",
			operation, resourceType, awssdk.ToString(token), event.OperationStatus, event.ErrorCode, message)
	}
	return nil, fmt.Errorf("%s %s request %q: %s: %s",
		operation, resourceType, awssdk.ToString(token), event.OperationStatus, message)
}

func (r *ResourceController) Create(ctx context.Context, resourceType, resourceData string) (string, error) {
	resp, err := r.client.CreateResource(ctx, &cloudcontrol.CreateResourceInput{
		TypeName:     &resourceType,
		DesiredState: &resourceData,
	})
	if err != nil {
		return "", fmt.Errorf("create %s: %w", resourceType, err)
	}
	event, err := r.await(ctx, resourceType, resp.ProgressEvent.RequestToken, 20*time.Minute)
	if err != nil {
		return "", err
	}
	if event.Identifier == nil {
		return "", fmt.Errorf("create %s: succeeded without returning an identifier", resourceType)
	}
	return *event.Identifier, nil
}

func (r *ResourceController) Read(ctx context.Context, resourceType, resourceID string) (string, error) {
	resp, err := r.client.GetResource(ctx, &cloudcontrol.GetResourceInput{
		TypeName:   &resourceType,
		Identifier: &resourceID,
	})
	if err != nil {
		return "", fmt.Errorf("read %s %q: %w", resourceType, resourceID, err)
	}
	return *resp.ResourceDescription.Properties, nil
}

func (r *ResourceController) Update(ctx context.Context, resourceType, resourceID, resourceData string) error {
	resp, err := r.client.UpdateResource(ctx, &cloudcontrol.UpdateResourceInput{
		TypeName:      &resourceType,
		Identifier:    &resourceID,
		PatchDocument: &resourceData,
	})
	if err != nil {
		return fmt.Errorf("update %s %q: %w", resourceType, resourceID, err)
	}
	_, err = r.await(ctx, resourceType, resp.ProgressEvent.RequestToken, 10*time.Minute)
	return err
}

func (r *ResourceController) Delete(ctx context.Context, resourceType, resourceID string) error {
	_, err := r.client.DeleteResource(ctx, &cloudcontrol.DeleteResourceInput{
		TypeName:   &resourceType,
		Identifier: &resourceID,
	})
	if err != nil {
		return fmt.Errorf("delete %s %q: %w", resourceType, resourceID, err)
	}
	return nil
}

func (r *ResourceController) List(ctx context.Context, resourceType string) (map[string]string, error) {
	resp, err := r.client.ListResources(ctx, &cloudcontrol.ListResourcesInput{
		TypeName: &resourceType,
	})
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", resourceType, err)
	}
	output := map[string]string{}
	for _, resource := range resp.ResourceDescriptions {
		output[*resource.Identifier] = *resource.Properties
	}
	return output, nil
}
