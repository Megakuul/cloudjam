package aws

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"codeberg.org/megakuul/cloudjam/internal/provider"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type AssetController struct {
	bucket string
	client *s3.Client
}

// NewAssetController creates a aws s3 asset controller.
// Usually you would create the asset controller from provider.Assets(), this is just for external tooling.
func NewAssetController(client *s3.Client, bucket string) *AssetController {
	return &AssetController{client: client, bucket: bucket}
}

func (p *Provider) Assets(ctx context.Context, id string, lifetime time.Duration) (provider.AssetController, error) {
	cfg, err := p.assume(ctx, id, p.adminRole, lifetime)
	if err != nil {
		return nil, err
	}
	return &AssetController{
		bucket: p.assetBucket,
		client: s3.NewFromConfig(cfg),
	}, nil
}

func (a *AssetController) Create(ctx context.Context, path string, asset io.Reader) (string, error) {
	path = strings.TrimPrefix(path, "/")
	_, err := a.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      &a.bucket,
		Key:         &path,
		Body:        asset,
		IfNoneMatch: new("*"),
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("s3://%s/%s", a.bucket, path), nil
}

func (a *AssetController) Read(ctx context.Context, path string) (string, io.ReadCloser, error) {
	path = strings.TrimPrefix(path, "/")
	resp, err := a.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &a.bucket,
		Key:    &path,
	})
	if err != nil {
		return "", nil, err
	}
	return fmt.Sprintf("s3://%s/%s", a.bucket, path), resp.Body, nil
}

func (a *AssetController) Update(ctx context.Context, oldPath, newPath string) (string, error) {
	oldPath = url.PathEscape(strings.TrimPrefix(oldPath, "/"))
	newPath = strings.TrimPrefix(newPath, "/")

	_, err := a.client.RenameObject(ctx, &s3.RenameObjectInput{
		Bucket:       &a.bucket,
		Key:          &newPath,
		RenameSource: &oldPath,
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("s3://%s/%s", a.bucket, newPath), nil
}

func (a *AssetController) Delete(ctx context.Context, path string) error {
	path = strings.TrimPrefix(path, "/")

	_, err := a.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &a.bucket,
		Key:    &path,
	})
	return err
}
