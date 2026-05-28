package frost

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/go-kit/log"
	"github.com/polarsignals/frostdb"
	"github.com/thanos-io/objstore/providers/s3"
)

type Log struct {
	Timestamp time.Time
	Level     slog.Level
	Message   string
	Labels    map[string]string `frostdb:",asc"`
}

type Storage struct{}

func NewStorage(url, bucket, region, accessKey, secretKey string) (*Storage, error) {
	bucket, err := s3.NewBucketWithConfig(log.NewJSONLogger(os.Stdout), s3.Config{
		Bucket:    "my-frostdb-bucket",
		Endpoint:  "s3.amazonaws.com",
		Region:    "us-east-1",
		AccessKey: "",
		SecretKey: "",
	}, "frostdb")
	if err != nil {
		return nil, err
	}
	store, err := frostdb.New(
		frostdb.WithReadOnlyStorage(frostdb.NewDefaultObjstoreBucket(bucket)),
	)
	defer store.Close()
	database, err := store.DB(context.TODO(), "cloudjam")
	if err != nil {
		return nil, err
	}
	logs, err := frostdb.NewGenericTable[Log](database, "log", memory.DefaultAllocator)
	if err != nil {
		return nil, err
	}
	logs.Write(context.Background(), Log{})

	// query.NewEngine(memory.DefaultAllocator, )
}
