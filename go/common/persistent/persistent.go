// Package persistent provides a wrapper around a key-value database for use as
// general node-wide storage.
package persistent

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"

	ekbadger "github.com/oasislabs/oasis-core/go/common/badger"
	"github.com/oasislabs/oasis-core/go/common/cbor"
	"github.com/oasislabs/oasis-core/go/common/logging"
)

const dbName = "persistent-store.badger.db"

// ErrNotFound is returned when the requested key could not be found in the database.
var ErrNotFound = errors.New("persistent: key not found in database")

// CommonStore is the interface to the common storage for the node.
type CommonStore struct {
	db *badger.DB
	gc *ekbadger.GCWorker
}

// Close closes the database handle.
func (cs *CommonStore) Close() {
	cs.gc.Close()
	cs.db.Close()
}

// GetServiceStore returns a handle to a per-service bucket for the given service.
func (cs *CommonStore) GetServiceStore(name string) (*ServiceStore, error) {
	ss := &ServiceStore{
		store: cs,
		name:  []byte(name),
	}
	return ss, nil
}

// NewCommonStore opens the default common node storage and returns a handle.
func NewCommonStore(dataDir string) (*CommonStore, error) {
	logger := logging.GetLogger("common/persistent")

	opts := badger.DefaultOptions(filepath.Join(dataDir, dbName))
	opts = opts.WithLogger(ekbadger.NewLogAdapter(logger))
	opts = opts.WithSyncWrites(true)
	opts = opts.WithCompression(options.None)

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open persistence database: %w", err)
	}

	cs := &CommonStore{
		db: db,
		gc: ekbadger.NewGCWorker(logger, db),
	}

	return cs, nil
}

// ServiceStore is a storage wrapper that automatically calls view callbacks with appropriate buckets.
type ServiceStore struct {
	store *CommonStore

	name []byte
}

// Close invalidates the per-service database handle.
func (ss *ServiceStore) Close() {
	ss.store = nil
}

// GetCBOR is a helper for retrieving CBOR-serialized values.
func (ss *ServiceStore) GetCBOR(key []byte, value interface{}) error {
	return ss.store.db.View(func(tx *badger.Txn) error {
		item, txErr := tx.Get(ss.dbKey(key))
		switch txErr {
		case nil:
		case badger.ErrKeyNotFound:
			return ErrNotFound
		default:
			return txErr
		}
		return item.Value(func(val []byte) error {
			if val == nil {
				return ErrNotFound
			}
			return cbor.Unmarshal(val, value)
		})
	})
}

// PutCBOR is a helper for storing CBOR-serialized values.
func (ss *ServiceStore) PutCBOR(key []byte, value interface{}) error {
	return ss.store.db.Update(func(tx *badger.Txn) error {
		return tx.Set(ss.dbKey(key), cbor.Marshal(value))
	})
}

func (ss *ServiceStore) dbKey(key []byte) []byte {
	return bytes.Join([][]byte{ss.name, key}, []byte{'.'})
}
