package hz_test

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/server"
	tu "github.com/verifa/horizon/pkg/testutil"
)

func TestMutex(t *testing.T) {
	ctx := context.Background()
	ti := server.Test(t, ctx)

	js, err := jetstream.New(ti.Conn)
	tu.AssertNoError(t, err)
	mutex, err := hz.MutexFromBucket(ctx, js, hz.BucketObjects)
	tu.AssertNoError(t, err)

	lock, err := mutex.Lock(ctx, "test")
	tu.AssertNoError(t, err)

	_, err = mutex.Lock(ctx, "test")
	tu.AssertErrorIs(t, err, hz.ErrKeyLocked)

	err = lock.Release()
	tu.AssertNoError(t, err)

	lock2, err := mutex.Lock(ctx, "test")
	tu.AssertNoError(t, err)
	err = lock2.Release()
	tu.AssertNoError(t, err)
}
