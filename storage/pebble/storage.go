package pebble

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/cockroachdb/pebble"
	"github.com/deepfabric/beehive/util"
)

// Storage returns a kv storage based on badger
type Storage struct {
	db *pebble.DB
}

// NewStorage returns pebble kv store on a default options
func NewStorage(dir string) (*Storage, error) {
	return NewStorageWithOptions(dir, &pebble.Options{})
}

// NewStorageWithOptions returns badger kv store
func NewStorageWithOptions(dir string, opts *pebble.Options) (*Storage, error) {
	db, err := pebble.Open(dir, opts)
	if err != nil {
		return nil, err
	}

	return &Storage{
		db: db,
	}, nil
}

// Set put the key, value pair to the storage
func (s *Storage) Set(key []byte, value []byte) error {
	return s.db.Set(key, value, pebble.NoSync)
}

// SetWithTTL put the key, value pair to the storage with a ttl in seconds
func (s *Storage) SetWithTTL(key []byte, value []byte, ttl int32) error {
	return fmt.Errorf("pebble storage not support set key-value with TTL")
}

// BatchSet batch set
func (s *Storage) BatchSet(pairs ...[]byte) error {
	if len(pairs)%2 != 0 {
		return fmt.Errorf("invalid args len: %d", len(pairs))
	}

	b := s.db.NewBatch()
	defer b.Close()

	for i := 0; i < len(pairs)/2; i++ {
		b.Set(pairs[2*i], pairs[2*i+1], nil)
	}

	return s.db.Apply(b, pebble.NoSync)
}

// Get returns the value of the key
func (s *Storage) Get(key []byte) ([]byte, error) {
	value, closer, err := s.db.Get(key)
	if err == pebble.ErrNotFound {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	defer closer.Close()
	if len(value) == 0 {
		return nil, nil
	}

	v := make([]byte, len(value), len(value))
	copy(v, value)

	return v, nil
}

// MGet returns multi values
func (s *Storage) MGet(keys ...[]byte) ([][]byte, error) {
	var values [][]byte
	for _, key := range keys {
		v, err := s.Get(key)
		if err != nil {
			return nil, err
		}

		values = append(values, v)
	}

	return values, nil
}

// Delete remove the key from the storage
func (s *Storage) Delete(key []byte) error {
	return s.db.Delete(key, pebble.NoSync)
}

// BatchDelete batch delete
func (s *Storage) BatchDelete(keys ...[]byte) error {
	b := s.db.NewBatch()
	defer b.Close()

	for _, key := range keys {
		b.Delete(key, nil)
	}

	return s.db.Apply(b, pebble.NoSync)
}

// RangeDelete remove data in [start,end)
func (s *Storage) RangeDelete(start, end []byte) error {
	return s.db.DeleteRange(start, end, pebble.NoSync)
}

// Scan scans the key-value paire in [start, end), and perform with a handler function, if the function
// returns false, the scan will be terminated, if the `pooledKey` is true, raftstore will call `Free` when
// scan completed.
func (s *Storage) Scan(start, end []byte, handler func(key, value []byte) (bool, error), pooledKey bool) error {
	iter := s.db.NewIter(&pebble.IterOptions{LowerBound: start, UpperBound: end})
	defer iter.Close()

	iter.First()
	for iter.Valid() {
		err := iter.Error()
		if err != nil {
			return err
		}

		ok, err := handler(clone(iter.Key()), clone(iter.Value()))
		if err != nil {
			return err
		}

		if !ok {
			break
		}

		iter.Next()
	}

	return nil
}

// Free free the pooled bytes
func (s *Storage) Free(pooled []byte) {

}

// SplitCheck Find a key from [start, end), so that the sum of bytes of the value of [start, key) <=size,
// returns the current bytes in [start,end), and the founded key
func (s *Storage) SplitCheck(start []byte, end []byte, size uint64) (uint64, []byte, error) {
	total := uint64(0)
	found := false
	var splitKey []byte

	iter := s.db.NewIter(&pebble.IterOptions{LowerBound: start, UpperBound: end})
	defer iter.Close()

	iter.First()
	for iter.Valid() {
		err := iter.Error()
		if err != nil {
			return 0, nil, err
		}

		if bytes.Compare(iter.Key(), end) >= 0 {
			break
		}

		total += uint64(len(iter.Key()) + len(iter.Value()))
		if !found && total >= size {
			found = true
			splitKey = clone(iter.Key())
			break
		}

		iter.Next()
	}

	return total, splitKey, nil
}

// Seek returns the first key-value that >= key
func (s *Storage) Seek(target []byte) ([]byte, []byte, error) {
	var key, value []byte

	iter := s.db.NewIter(&pebble.IterOptions{LowerBound: target})
	defer iter.Close()

	iter.First()
	if iter.Valid() {
		err := iter.Error()
		if err != nil {
			return nil, nil, err
		}

		key = clone(iter.Key())
		value = clone(iter.Value())
	}

	return key, value, nil
}

// Write write the data in batch
func (s *Storage) Write(wb *util.WriteBatch, sync bool) error {
	if len(wb.Ops) == 0 {
		return nil
	}

	b := s.db.NewBatch()
	defer b.Close()

	var err error
	for idx, op := range wb.Ops {
		switch op {
		case util.OpDelete:
			err = b.Delete(wb.Keys[idx], nil)
		case util.OpSet:
			if wb.TTLs[idx] > 0 {
				return fmt.Errorf("pebble storage not support set key-value with TTL")
			}

			err = b.Set(wb.Keys[idx], wb.Values[idx], nil)
		}

		if err != nil {
			return err
		}
	}

	opts := pebble.NoSync
	if sync {
		opts = pebble.Sync
	}

	return s.db.Apply(b, opts)
}

// CreateSnapshot create a snapshot file under the giving path
func (s *Storage) CreateSnapshot(path string, start, end []byte) error {
	err := os.MkdirAll(path, os.ModeDir)
	if err != nil {
		return err
	}

	file := filepath.Join(path, "db.data")
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()

	err = writeBytes(f, start)
	if err != nil {
		return err
	}

	err = writeBytes(f, end)
	if err != nil {
		return err
	}

	snap := s.db.NewSnapshot()
	defer snap.Close()

	iter := snap.NewIter(&pebble.IterOptions{LowerBound: start, UpperBound: end})
	defer iter.Close()

	iter.First()
	for iter.Valid() {
		err := iter.Error()
		if err != nil {
			return err
		}

		if bytes.Compare(iter.Key(), end) >= 0 {
			break
		}

		err = writeBytes(f, iter.Key())
		if err != nil {
			return err
		}

		err = writeBytes(f, iter.Value())
		if err != nil {
			return err
		}

		iter.Next()
	}

	return nil
}

// ApplySnapshot apply a snapshort file from giving path
func (s *Storage) ApplySnapshot(path string) error {
	f, err := os.Open(filepath.Join(path, "db.data"))
	if err != nil {
		return err
	}
	defer f.Close()

	start, err := readBytes(f)
	if err != nil {
		return err
	}
	if len(start) == 0 {
		return fmt.Errorf("error format, missing start field")
	}

	end, err := readBytes(f)
	if err != nil {
		return err
	}
	if len(end) == 0 {
		return fmt.Errorf("error format, missing end field")
	}

	err = s.db.DeleteRange(start, end, pebble.NoSync)
	if err != nil {
		return err
	}

	for {
		key, err := readBytes(f)
		if err != nil {
			return err
		}
		if len(key) == 0 {
			break
		}

		value, err := readBytes(f)
		if err != nil {
			return err
		}
		if len(value) == 0 {
			return fmt.Errorf("error format, missing value field")
		}

		err = s.db.Set(key, value, pebble.NoSync)
		if err != nil {
			return err
		}
	}

	return nil
}

// Close close the storage
func (s *Storage) Close() error {
	return s.db.Close()
}

func clone(value []byte) []byte {
	v := make([]byte, len(value), len(value))
	copy(v, value)
	return v
}

func writeBytes(f *os.File, data []byte) error {
	size := make([]byte, 4)
	binary.BigEndian.PutUint32(size, uint32(len(data)))
	_, err := f.Write(size)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err != nil {
		return err
	}

	return nil
}

func readBytes(f *os.File) ([]byte, error) {
	size := make([]byte, 4)
	n, err := f.Read(size)
	if n == 0 && err == io.EOF {
		return nil, nil
	}

	total := int(binary.BigEndian.Uint32(size))
	written := 0
	data := make([]byte, total, total)
	for {
		n, err = f.Read(data[written:])
		if err != nil && err != io.EOF {
			return nil, err
		}
		written += n
		if written == total {
			return data, nil
		}
	}
}
