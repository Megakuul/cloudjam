package aws

import (
	"context"
	"time"

	"codeberg.org/megakuul/cloudjam/internal/provider"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
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

func (r *ResourceController) Create(ctx context.Context, resourceType, resourceData string) (string, error) {
	resp, err := r.client.CreateResource(ctx, &cloudcontrol.CreateResourceInput{
		TypeName:     &resourceType,
		DesiredState: &resourceData,
	})
	if err != nil {
		return "", err
	}
	waiter := cloudcontrol.NewResourceRequestSuccessWaiter(r.client)
	status, err := waiter.WaitForOutput(ctx,
		&cloudcontrol.GetResourceRequestStatusInput{RequestToken: resp.ProgressEvent.RequestToken},
		20*time.Minute)
	if err != nil {
		return "", err
	}
	return *status.ProgressEvent.Identifier, nil
}

func (r *ResourceController) Read(ctx context.Context, resourceType, resourceID string) (string, error) {
	resp, err := r.client.GetResource(ctx, &cloudcontrol.GetResourceInput{
		TypeName:   &resourceType,
		Identifier: &resourceID,
	})
	if err != nil {
		return "", err
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
		return err
	}
	waiter := cloudcontrol.NewResourceRequestSuccessWaiter(r.client)
	_, err = waiter.WaitForOutput(ctx,
		&cloudcontrol.GetResourceRequestStatusInput{RequestToken: resp.ProgressEvent.RequestToken},
		10*time.Minute)
	return err
}

func (r *ResourceController) Delete(ctx context.Context, resourceType, resourceID string) error {
	_, err := r.client.DeleteResource(ctx, &cloudcontrol.DeleteResourceInput{
		TypeName:   &resourceType,
		Identifier: &resourceID,
	})
	return err
}

func (r *ResourceController) List(ctx context.Context, resourceType string) (map[string]string, error) {
	resp, err := r.client.ListResources(ctx, &cloudcontrol.ListResourcesInput{
		TypeName: &resourceType,
	})
	if err != nil {
		return nil, err
	}
	output := map[string]string{}
	for _, resource := range resp.ResourceDescriptions {
		output[*resource.Identifier] = *resource.Properties
	}
	return output, nil
}
