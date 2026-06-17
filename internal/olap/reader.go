package olap

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// reader implements readerAt for s3. Used to fetch the parquet footers.
type Reader struct {
	ctx    context.Context
	bucket string
	key    string
	client *s3.Client
}

func newReader(ctx context.Context, client *s3.Client, bucket, key string) *Reader {
	return &Reader{
		ctx:    ctx,
		bucket: bucket,
		key:    key,
		client: client,
	}
}

func (r *Reader) ReadAt(p []byte, offset int64) (int, error) {
	result, err := r.client.GetObject(r.ctx, &s3.GetObjectInput{
		Bucket: &r.bucket,
		Key:    &r.key,
		Range:  new(fmt.Sprintf("bytes=%d-", offset)),
	})
	if err != nil {
		return 0, err
	}
	defer result.Body.Close()
	return io.ReadFull(result.Body, p)
}
