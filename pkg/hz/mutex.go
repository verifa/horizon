package hz

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

var ErrKeyLocked = errors.New("key locked")

func MutexFromBucket(
	ctx context.Context,
	js jetstream.JetStream,
	bucket string,
) (mutex, error) {
	mxkv, err := js.KeyValue(ctx, bucket)
	if err != nil {
		return mutex{}, fmt.Errorf(
			"get mutex bucket %q: %w",
			bucket,
			err,
		)
	}
	status, err := mxkv.Status(ctx)
	if err != nil {
		return mutex{}, fmt.Errorf(
			"get mutex bucket %q status: %w",
			bucket,
			err,
		)
	}

	return mutex{
		kv:  mxkv,
		ttl: status.TTL(),
	}, nil
}

type mutex struct {
	kv  jetstream.KeyValue
	ttl time.Duration
}

// Lock acquires a lock for the given key.
// If a lock already exists, then ErrKeyLocked is returned.
// The returned lock can be used to keep the lock alive (for long running
// processes) and eventually release the lock.
func (m *mutex) Lock(ctx context.Context, key string) (*lock, error) {
	kve, err := m.kv.Get(ctx, key)
	if err != nil {
		// For the first mutex per key, the key won't exist, obviously.
		// That means we can acquire the lock.
		if !errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, fmt.Errorf("geting current mutex value: %w", err)
		}
		rev, err := m.kv.Create(ctx, key, []byte("1"))
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyExists) {
				// Someone else got the lock before us.
				return nil, ErrKeyLocked
			}
			return nil, fmt.Errorf("writing lock to mutex bucket: %w", err)
		}
		return &lock{
			kv:  m.kv,
			rev: rev,
			key: key,
		}, nil
	}
	// If we get here, then the key exists, and we need to check the value.
	if len(kve.Value()) != 0 {
		// Someone else has acquired the lock.
		return nil, ErrKeyLocked
	}
	// We have a chance to acquire the lock.
	rev, err := m.kv.Update(ctx, key, []byte("1"), kve.Revision())
	if err != nil {
		// If we get a bad revision error, then someone else has acquired
		// the lock.
		if isErrWrongLastSequence(err) {
			return nil, ErrKeyLocked
		}
		return nil, fmt.Errorf("writing lock to mutex bucket: %w", err)
	}
	return &lock{
		kv:  m.kv,
		rev: rev,
		key: key,
	}, nil
}

type lock struct {
	kv       jetstream.KeyValue
	rev      uint64
	key      string
	released bool
}

// InProgress keeps the lock alive.
// A lock expires based on the TTL value set for the kv store that the lock
// belongs to.
func (l *lock) InProgress() error {
	// TODO: this needs to reset the TTL of the key.
	// Easiest thing to do is to update the value?
	ctx := context.TODO()
	rev, err := l.kv.Update(ctx, l.key, []byte("1"), l.rev)
	if err != nil {
		return fmt.Errorf("updating lock: %w", err)
	}
	l.rev = rev
	return nil
}

// Release releases the lock, meaning a new lock can be acquired for the mutex.
func (l *lock) Release() error {
	if l.released {
		return nil
	}
	_, err := l.kv.Update(context.Background(), l.key, nil, l.rev)
	if err != nil {
		return fmt.Errorf("unlocking mutex: %w", err)
	}
	l.released = true
	return nil
}
