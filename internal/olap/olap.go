// olap provides writers for the analytic data ingestion system.
// The models including read controllers are defined as subpackages.
package olap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/google/uuid"
	"github.com/parquet-go/parquet-go"
)

// Inserter abstracts an OLAP inserter for the generic model.
// Abstraction exists to add a batcher system between the frostdb insertion engine and the insert call (to avoid serverless parquet churn).
type Inserter[T any] interface {
	Insert(context.Context, T) error
}

// LocalInserter directly writes inserts to an underlying frostdb instance.
type LocalInserter[T any] struct {
	table parquet.GenericWriter[T]
}

func NewLocalInserter[T any](table parquet.GenericWriter[T]) *LocalInserter[T] {
	return &LocalInserter[T]{
		table: table,
	}
}

func (i *LocalInserter[T]) Insert(ctx context.Context, records ...T) error {
	_, err := i.table.Write(records)
	if err != nil {
		return err
	}
	return nil
}

type engine struct {
	client      *s3.Client
	bucket      string
	catalog     Catalog
	catalogLock sync.RWMutex
}

type Catalog struct {
	Key    string             `json:"-"`
	ETag   string             `json:"-"`
	Blocks map[string][]Block `json:"blocks"`
}

type Block struct {
	Version string `json:"version"`
	Max     int64  `json:"max"`
	Min     int64  `json:"min"`
	Target  string `json:"target"`
}

func (e *engine) Lookup(from, to time.Time) ([]string, error) {
	return nil, nil
}

func (e *engine) Write(ctx context.Context, file parquet.File) error {
	table, primary, version := "", "timestamp", ""
	for _, kv := range file.Metadata().KeyValueMetadata {
		switch kv.Key {
		case "table":
			table = kv.Value
		case "primary":
			primary = kv.Value
		case "version":
			version = kv.Value
		}
	}
	ts, ok := file.Schema().Lookup(primary)
	if !ok {
		return fmt.Errorf("invalid parquet write: schema does not contain mandatory '%s' column", primary)
	}
	var max, min int64
	for _, rg := range file.RowGroups() {
		chunk := rg.ColumnChunks()[ts.ColumnIndex]
		idx, err := chunk.ColumnIndex()
		if err != nil {
			return fmt.Errorf("invalid parquet write: schema does not index mandatory '%s' column", primary)
		}

		for i := range idx.NumPages() {
			if idx.NullPage(i) {
				continue
			}

			if val := idx.MaxValue(i); val.Kind() == parquet.Int64 {
				if val.Int64() > max || max == 0 {
					max = val.Int64()
				} else if val.Int64() < min || min == 0 {
					min = val.Int64()
				}
			}
		}
	}

	target := fmt.Sprintf("%s-%s", table, uuid.New())
	_, err := e.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      &e.bucket,
		Key:         &target,
		IfNoneMatch: new("*"),
		Body:        io.NewSectionReader(&file, 0, file.Size()),
	})
	if err != nil {
		return err
	}

	block := Block{
		Version: version,
		Max:     max,
		Min:     min,
		Target:  target,
	}
	e.catalog.Blocks[table] = append(e.catalog.Blocks[table], block)
	if err = e.commitCatalog(ctx); err != nil {
		// retry once on optimistic lock failure
		if errors.Is(err, ErrOptimisticLock) {
			if err := e.loadCatalog(ctx); err != nil {
				return err
			}
			e.catalog.Blocks[table] = append(e.catalog.Blocks[table], block)
			return e.commitCatalog(ctx)
		}
		return err
	}
	return nil
}

// loadCatalog loads the current catalog from datastore into the engine.
// It creates the catalog if not existent.
func (e *engine) loadCatalog(ctx context.Context) error {
	e.catalogLock.Lock()
	defer e.catalogLock.Unlock()
	result, err := e.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &e.bucket,
		Key:    &e.catalog.Key,
	})
	if err != nil {
		if _, ok := errors.AsType[*types.NotFound](err); ok {
			rawCatalog, err := json.Marshal(e.catalog)
			if err != nil {
				return err
			}
			result, err := e.client.PutObject(ctx, &s3.PutObjectInput{
				Bucket:      &e.bucket,
				Key:         &e.catalog.Key,
				IfNoneMatch: new("*"),
				Body:        bytes.NewReader(rawCatalog),
			})
			if err != nil {
				return err
			}
			e.catalog.ETag = *result.ETag
			return nil
		}
		return err
	}
	defer result.Body.Close()
	rawCatalog, err := io.ReadAll(result.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(rawCatalog, &e.catalog)
	if err != nil {
		return err
	}
	e.catalog.ETag = *result.ETag
	return nil
}

var ErrOptimisticLock = errors.New("optimistic lock failure")

// commitCatalog writes the current catalog to datastore.
// It uses optimistic locking, if retry is set to true it will retry once on optimistic failure.
func (e *engine) commitCatalog(ctx context.Context) error {
	e.catalogLock.Lock()
	defer e.catalogLock.Unlock()
	rawCatalog, err := json.Marshal(e.catalog)
	if err != nil {
		return err
	}
	result, err := e.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:  &e.bucket,
		Key:     &e.catalog.Key,
		IfMatch: &e.catalog.ETag,
		Body:    bytes.NewReader(rawCatalog),
	})
	if err != nil {
		sErr, ok := errors.AsType[smithy.APIError](err)
		if ok && sErr.ErrorCode() == "PreconditionFailed" {
			return ErrOptimisticLock
		}
		return err
	}
	e.catalog.ETag = *result.ETag
	return nil
}

type Reader struct {
	ctx    context.Context
	bucket string
	key    string
	client *s3.Client
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
