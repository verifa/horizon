package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/verifa/horizon/pkg/hz"
)

func InitKeyValue(
	ctx context.Context,
	conn *nats.Conn,
	opts ...StoreOption,
) error {
	opt := defaultStoreOptions
	for _, o := range opts {
		o(&opt)
	}

	js, err := jetstream.New(conn)
	if err != nil {
		return fmt.Errorf("new jetstream: %w", err)
	}

	if _, err := js.KeyValue(ctx, hz.BucketObjects); err != nil {
		if !errors.Is(err, jetstream.ErrBucketNotFound) {
			return fmt.Errorf(
				"get objects bucket %q: %w",
				hz.BucketObjects,
				err,
			)
		}
		if _, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
			Description: "KV bucket for storing horizon objects.",
			Bucket:      hz.BucketObjects,
			History:     1,
			TTL:         0,
		}); err != nil {
			return fmt.Errorf(
				"create objects bucket %q: %w",
				hz.BucketObjects,
				err,
			)
		}
	}
	// TODO: handle updating the objects bucket if it exists.

	if _, err := js.KeyValue(ctx, hz.BucketMutex); err != nil {
		if !errors.Is(err, jetstream.ErrBucketNotFound) {
			return fmt.Errorf(
				"get mutex bucket %q for %q: %w",
				hz.BucketMutex,
				hz.BucketObjects,
				err,
			)
		}
		if _, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
			Bucket:      hz.BucketMutex,
			Description: "Mutex for " + hz.BucketObjects,
			History:     1,
			// In case unlocking fails, or there's a serious error,
			// NATS will automatically unlock the mutex after the TTL.
			// Behind the scenes, NATS will delete the TTL value,
			// which from the mutex's perspective means there is no lock.
			TTL: opt.mutexTTL,
		}); err != nil {
			return fmt.Errorf(
				"create mutex bucket %q for %q: %w",
				hz.BucketMutex,
				hz.BucketObjects,
				err,
			)
		}
	}
	// TODO: handle updating the mutex bucket if it exists.
	return nil
}
