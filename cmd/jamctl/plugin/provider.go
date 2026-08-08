package plugin

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"

	"codeberg.org/megakuul/cloudjam/internal/provider"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/api"
	"github.com/google/uuid"
)

type localProvider struct {
	score     float64
	access    provider.AccessController
	assets    provider.AssetController
	resources provider.ResourceController
}

func (p *localProvider) report(ctx context.Context, in *api.ReportInput) (*api.ReportOutput, error) {
	slog.Warn(in.Error, "source", "plugin")
	return &api.ReportOutput{}, nil
}

func (p *localProvider) log(ctx context.Context, in *api.LogInput) (*api.LogOutput, error) {
	slog.Log(ctx, in.Severity, in.Message, "source", "plugin")
	return &api.LogOutput{}, nil
}

func (p *localProvider) createMeta(ctx context.Context, in *api.CreateMetaInput) (*api.CreateMetaOutput, error) {
	slog.Info("challenge metadata created", "title", in.Title)
	slog.Info(strings.Join(in.Descriptions, "\n\n"))
	for id, clue := range in.Clues {
		slog.Info(clue, "clue", id)
	}
	for id, asset := range in.Assets {
		slog.Info(asset, "asset", id)
	}
	return &api.CreateMetaOutput{}, nil
}

func (p *localProvider) updateMeta(ctx context.Context, in *api.UpdateMetaInput) (*api.UpdateMetaOutput, error) {
	slog.Info("challenge metadata updated")
	slog.Info(strings.Join(in.AdditionalDescriptions, "\n\n"))
	for id, clue := range in.AdditionalClues {
		slog.Info(clue, "clue", id)
	}
	for id, asset := range in.AdditionalAssets {
		slog.Info(asset, "asset", id)
	}
	return &api.UpdateMetaOutput{}, nil
}

func (p *localProvider) readScore(ctx context.Context, in *api.ReadScoreInput) (*api.ReadScoreOutput, error) {
	return &api.ReadScoreOutput{Score: p.score}, nil
}

func (p *localProvider) updateScore(ctx context.Context, in *api.UpdateScoreInput) (*api.UpdateScoreOutput, error) {
	p.score += in.Increment
	slog.Info(fmt.Sprintf("%+g points — %s", in.Increment, in.Reason), "score", p.score)
	return &api.UpdateScoreOutput{}, nil
}

func (p *localProvider) createAsset(ctx context.Context, in api.CreateAssetInput) (*api.CreateAssetOutput, error) {
	if len(in) > 50_000_000 {
		return nil, fmt.Errorf("assets larger then 50 MB are not supported")
	}
	name := uuid.NewString()
	url, err := p.assets.Create(ctx, name, bytes.NewReader(in))
	if err != nil {
		return nil, err
	}
	return &api.CreateAssetOutput{
		Name: name,
		URL:  url,
	}, nil
}

func (p *localProvider) updateAsset(ctx context.Context, in *api.UpdateAssetInput) (*api.UpdateAssetOutput, error) {
	url, err := p.assets.Update(ctx, in.OldName, in.NewName)
	if err != nil {
		return nil, err
	}
	return &api.UpdateAssetOutput{
		NewURL: url,
	}, nil
}

func (p *localProvider) createPermission(ctx context.Context, in *api.CreatePermissionInput) (*api.CreatePermissionOutput, error) {
	slog.Info("creating permission")
	err := p.access.CreatePermission(ctx, in.Permission)
	return &api.CreatePermissionOutput{}, err
}

func (p *localProvider) updatePermission(ctx context.Context, in *api.UpdatePermissionInput) (*api.UpdatePermissionOutput, error) {
	slog.Info("updating permission")
	err := p.access.UpdatePermission(ctx, in.Permission)
	return &api.UpdatePermissionOutput{}, err
}

func (p *localProvider) createGuardrail(ctx context.Context, in *api.CreateGuardrailInput) (*api.CreateGuardrailOutput, error) {
	slog.Info("creating guardrail")
	err := p.access.CreateGuardrail(ctx, in.Guardrail)
	return &api.CreateGuardrailOutput{}, err
}

func (p *localProvider) updateGuardrail(ctx context.Context, in *api.UpdateGuardrailInput) (*api.UpdateGuardrailOutput, error) {
	slog.Info("creating guardrail")
	err := p.access.UpdateGuardrail(ctx, in.Guardrail)
	return &api.UpdateGuardrailOutput{}, err
}

func (p *localProvider) createResource(ctx context.Context, in *api.CreateResourceInput) (*api.CreateResourceOutput, error) {
	slog.Info("creating resource", "type", in.Type)
	identifier, err := p.resources.Create(ctx, in.Type, in.Desired)
	if err != nil {
		return nil, err
	}
	slog.Info("created resource", "type", in.Type, "identifier", identifier)
	return &api.CreateResourceOutput{Identifier: identifier}, nil
}

func (p *localProvider) readResource(ctx context.Context, in *api.ReadResourceInput) (*api.ReadResourceOutput, error) {
	state, err := p.resources.Read(ctx, in.Type, in.Identifier)
	return &api.ReadResourceOutput{State: state}, err
}

func (p *localProvider) updateResource(ctx context.Context, in *api.UpdateResourceInput) (*api.UpdateResourceOutput, error) {
	slog.Info("updating resource", "type", in.Type, "identifier", in.Identifier)
	return &api.UpdateResourceOutput{}, p.resources.Update(ctx, in.Type, in.Identifier, in.Patch)
}

func (p *localProvider) deleteResource(ctx context.Context, in *api.DeleteResourceInput) (*api.DeleteResourceOutput, error) {
	slog.Info("deleting resource", "type", in.Type, "identifier", in.Identifier)
	return &api.DeleteResourceOutput{}, p.resources.Delete(ctx, in.Type, in.Identifier)
}

func (p *localProvider) listResource(ctx context.Context, in *api.ListResourceInput) (*api.ListResourceOutput, error) {
	resources, err := p.resources.List(ctx, in.Type)
	return &api.ListResourceOutput{Resources: resources}, err
}
