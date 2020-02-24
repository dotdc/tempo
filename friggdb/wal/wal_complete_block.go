package wal

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"time"

	bloom "github.com/dgraph-io/ristretto/z"
	"github.com/google/uuid"
	"github.com/grafana/frigg/friggdb/backend"
)

// complete block has all of the fields
type completeBlock struct {
	meta        *backend.BlockMeta
	bloom       *bloom.Bloom
	filepath    string
	records     []*backend.Record
	timeWritten time.Time

	readFile *os.File
}

type ReplayBlock interface {
	Iterator() (backend.Iterator, error)
	TenantID() string
	Clear() error
}

type CompleteBlock interface {
	ReplayBlock

	Find(id backend.ID) ([]byte, error)
	TimeWritten() time.Time

	BlockMeta() *backend.BlockMeta
	BloomFilter() *bloom.Bloom
	BlockWroteSuccessfully(t time.Time)
	WriteInfo() (blockID uuid.UUID, tenantID string, records []*backend.Record, filepath string) // todo:  i hate this method.  do something better.
}

func (c *completeBlock) TenantID() string {
	return c.meta.TenantID
}

func (c *completeBlock) WriteInfo() (uuid.UUID, string, []*backend.Record, string) {
	return c.meta.BlockID, c.meta.TenantID, c.records, c.fullFilename()
}

func (c *completeBlock) Find(id backend.ID) ([]byte, error) {

	i := sort.Search(len(c.records), func(idx int) bool {
		return bytes.Compare(c.records[idx].ID, id) >= 0
	})

	if i < 0 || i >= len(c.records) {
		return nil, nil
	}

	rec := c.records[i]

	b, err := c.readRecordBytes(rec)
	if err != nil {
		return nil, err
	}

	iter := backend.NewIterator(bytes.NewReader(b))
	var foundObject []byte
	for {
		foundID, b, err := iter.Next()
		if foundID == nil {
			break
		}
		if err != nil {
			return nil, err
		}
		if bytes.Equal(foundID, id) {
			foundObject = b
			break
		}
	}

	if err != nil {
		return nil, err
	}

	return foundObject, nil
}

func (c *completeBlock) Iterator() (backend.Iterator, error) {
	name := c.fullFilename()
	f, err := os.OpenFile(name, os.O_RDONLY, 0644)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return backend.NewIterator(f), nil
}

func (c *completeBlock) Clear() error {
	if c.readFile != nil {
		err := c.readFile.Close()
		if err != nil {
			return err
		}
	}

	name := c.fullFilename()
	return os.Remove(name)
}

func (c *completeBlock) TimeWritten() time.Time {
	return c.timeWritten
}

func (c *completeBlock) BlockWroteSuccessfully(t time.Time) {
	c.timeWritten = t
}

func (c *completeBlock) BlockMeta() *backend.BlockMeta {
	return c.meta
}

func (c *completeBlock) BloomFilter() *bloom.Bloom {
	return c.bloom
}

func (c *completeBlock) fullFilename() string {
	return fmt.Sprintf("%s/%v:%v", c.filepath, c.meta.BlockID, c.meta.TenantID)
}

func (c *completeBlock) readRecordBytes(r *backend.Record) ([]byte, error) {
	if c.readFile == nil {
		name := c.fullFilename()

		f, err := os.OpenFile(name, os.O_RDONLY, 0644)
		if err != nil {
			return nil, err
		}
		c.readFile = f
	}

	b := make([]byte, r.Length)
	_, err := c.readFile.ReadAt(b, int64(r.Start))
	if err != nil {
		return nil, err
	}

	return b, nil
}
