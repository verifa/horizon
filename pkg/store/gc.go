package store

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/verifa/horizon/pkg/hz"
)

type GarbageCollector struct {
	Conn *nats.Conn
	KV   jetstream.KeyValue

	watcher *hz.Watcher
}

func (gc *GarbageCollector) Start(ctx context.Context) error {
	watcher, err := hz.StartWatcher(
		ctx,
		gc.Conn,
		// Watch all objects.
		hz.WithWatcherFor(hz.ObjectKey{}),
		hz.WithWatcherDurable("horizon-garbage-collector"),
		hz.WithWatcherFn(func(event hz.Event) (hz.Result, error) {
			return gc.garbageCollect(ctx, event)
		}),
	)
	if err != nil {
		return fmt.Errorf("start garbage collector watcher: %w", err)
	}
	gc.watcher = watcher
	return nil
}

func (gc *GarbageCollector) Stop() {
	if gc.watcher != nil {
		gc.watcher.Close()
	}
}

func (gc *GarbageCollector) garbageCollect(
	ctx context.Context,
	event hz.Event,
) (hz.Result, error) {
	// Only care about delete operations, which means the
	// metadata.deletionTimestamp field is set.
	if event.Operation != hz.EventOperationDelete {
		return hz.Result{}, nil
	}
	var obj hz.MetaOnlyObject
	if err := json.Unmarshal(event.Data, &obj); err != nil {
		return hz.Result{}, fmt.Errorf("unmarshal object: %w", err)
	}
	// If the object has no deletion timestamp, it's not ready to be deleted.
	if obj.DeletionTimestamp == nil {
		return hz.Result{}, nil
	}
	// Check the deletion is due. If not, reschedule for when it is.
	if !obj.ObjectMeta.DeletionTimestamp.IsPast() {
		// If the deletion timestamp has not expired yet, requeue the event
		// to be processed once it has.
		return hz.Result{
			RequeueAfter: time.Until(obj.ObjectMeta.DeletionTimestamp.Time),
		}, nil
	}
	result, err := gc.deleteObjectCascading(ctx, obj)
	if err != nil {
		slog.Error("deleting object", "key", event.Key, "error", err)
		return hz.Result{
			// Try again in a short while.
			RequeueAfter: time.Second * 5,
		}, nil
	}
	if result == DeleteResultFinalizers {
		// If the object still has finalizers, ACK and we will try again when
		// the object is updated.
		return hz.Result{}, nil
	}

	return hz.Result{}, nil
}

func (gc *GarbageCollector) deleteObjectCascading(
	ctx context.Context,
	obj hz.MetaOnlyObject,
) (DeleteResult, error) {
	slog.Info(
		"CASCADING DELETE",
		"key",
		hz.KeyFromObject(obj),
		// "revision",
		// *obj.ObjectMeta.Revision,
		"finalizers",
		obj.Finalizers,
	)
	// If the object has finalizers, it's not ready to be deleted.
	if obj.ObjectMeta.Finalizers != nil && len(*obj.ObjectMeta.Finalizers) > 0 {
		return DeleteResultFinalizers, nil
	}
	// Check any child objects and delete those first.
	wOpts := []jetstream.WatchOpt{jetstream.IgnoreDeletes()}
	watcher, err := gc.KV.Watch(ctx, hz.KeyFromObject(hz.ObjectKey{}), wOpts...)
	if err != nil {
		return DeleteResultError, fmt.Errorf("watching key: %w", err)
	}
	defer func() {
		_ = watcher.Stop()
	}()
	children := []hz.MetaOnlyObject{}
	for entry := range watcher.Updates() {
		// Nil entry is sent once all updates have been processed.
		if entry == nil {
			break
		}
		var child hz.MetaOnlyObject
		if err := json.Unmarshal(entry.Value(), &child); err != nil {
			return DeleteResultError, fmt.Errorf(
				"unmarshal child object: %w",
				err,
			)
		}
		for _, ownerRef := range child.ObjectMeta.OwnerReferences {
			if ownerRef.IsOwnedBy(obj) {
				children = append(children, child)
				break
			}
		}
	}

	for _, child := range children {
		result, err := gc.deleteObjectCascading(ctx, child)
		// If deletion was not a success, propagate the result and error.
		if result != DeleteResultSuccess {
			return result, err
		}
	}

	// Finally, delete the object itself.
	if err := gc.KV.Delete(ctx, hz.KeyFromObject(obj)); err != nil {
		return DeleteResultError, fmt.Errorf("deleting object: %w", err)
	}

	return DeleteResultSuccess, nil
}

type DeleteResult int

const (
	DeleteResultSuccess DeleteResult = iota
	DeleteResultError
	DeleteResultFinalizers
)
