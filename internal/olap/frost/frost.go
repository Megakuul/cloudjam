package frost

import (
	"context"
	"os"

	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/go-kit/log"
	"github.com/polarsignals/frostdb"
	"github.com/polarsignals/frostdb/query"
	"github.com/thanos-io/objstore/providers/s3"
)

type Storage struct {
	store    *frostdb.ColumnStore
	database *frostdb.DB
}

func NewStorage(ctx context.Context, cfg s3.Config, name string) (*Storage, error) {
	bucket, err := s3.NewBucketWithConfig(log.NewJSONLogger(os.Stdout), cfg, name)
	if err != nil {
		return nil, err
	}
	store, err := frostdb.New(
		frostdb.WithReadOnlyStorage(frostdb.NewDefaultObjstoreBucket(bucket)),
	)
	database, err := store.DB(ctx, name)
	if err != nil {
		return nil, err
	}
	return &Storage{
		store:    store,
		database: database,
	}, nil

	// query.NewEngine(memory.DefaultAllocator, )
}

func (s *Storage) Close() error {
	return s.store.Close()
}

type Table[T any] struct {
	name   string
	table  *frostdb.GenericTable[T]
	engine *query.LocalEngine
}

func NewTable[T any](storage *Storage, name string) (*Table[T], error) {
	table, err := frostdb.NewGenericTable[T](storage.database, name, memory.DefaultAllocator)
	if err != nil {
		return nil, err
	}
	return &Table[T]{
		name:   name,
		table:  table,
		engine: query.NewEngine(memory.DefaultAllocator, storage.database.TableProvider()),
	}, nil
}

func (t *Table[T]) Insert(ctx context.Context, record T) error {
	_, err := t.table.Write(context.Background(), record)
	return err
}

func (t *Table[T]) Query() query.Builder {
	return t.engine.ScanTable(t.name)
}
