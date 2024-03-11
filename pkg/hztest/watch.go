package hztest

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/pkg/hz"
)

func WatchWaitUntil[T hz.Objecter](
	t *testing.T,
	ctx context.Context,
	conn *nats.Conn,
	timeout time.Duration,
	watchObj hz.ObjectKeyer,
	isDone func(T) bool,
) {
	// Create a timeout and a done channel.
	// Watch until the replicaset is ready.
	// If the timeout is reached, fail the test.
	timeoutCh := time.After(timeout)
	done := make(chan struct{})
	watcher, err := hz.StartWatcher(
		ctx,
		conn,
		hz.WithWatcherFor(watchObj),
		hz.WithWatcherFn(
			func(event hz.Event) (hz.Result, error) {
				var t T
				if err := json.Unmarshal(event.Data, &t); err != nil {
					return hz.Result{}, fmt.Errorf(
						"unmarshalling object: %w",
						err,
					)
				}
				if isDone(t) {
					close(done)
				}
				return hz.Result{}, nil
			},
		),
	)
	if err != nil {
		t.Fatalf("start watcher: %v", err)
	}
	defer watcher.Close()

	select {
	case <-timeoutCh:
		t.Errorf("timeout")
	case <-done:
	}
}
