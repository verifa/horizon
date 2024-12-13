package schema

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/pkg/extensions/core"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/internal/openapiv3"
)

type Schema struct {
	Conn *nats.Conn

	definitions map[string]*core.CustomResourceDefinition
	crdWatcher  *hz.Watcher
	init        bool
	eventCh     chan hz.Event
	mx          sync.RWMutex
}

// What does schema need to do?
// Store receives commands, and needs to resolve the Kind.
// Store receives command for providing the openapiv3 schema.

// KindForKey returns the "real" Kind for the given key.
// The key could be lowercase, plural, singular, short-hand, etc.
// The key might not contain a group or version.
func (s *Schema) KindForKey(ctx context.Context, key hz.ObjectKeyer) string {
	s.mx.RLock()
	defer s.mx.RUnlock()
	// Check if the key is valid.
	// Group, Version, Namespace and Name are exact matches.
	// Kind could be lowercase, plural, singular, short-hand, etc.
	// Return the "real" Kind.
	return ""
}

func (s *Schema) IsKnownObject(ctx context.Context, key hz.ObjectKeyer) bool {
	s.mx.RLock()
	defer s.mx.RUnlock()
	// Check if the object is known.
	return true
}

func (s *Schema) OpenAPIV3Schema(
	ctx context.Context,
	key hz.ObjectKeyer,
) (*openapiv3.Schema, error) {
	// Get the OpenAPIV3 schema for the given key.
	return nil, nil
}

func (s *Schema) Start(ctx context.Context) error {
	s.eventCh = make(chan hz.Event)
	go func() {
		for event := range s.eventCh {
			var result hz.Result
			var err error
			switch event.Key.ObjectKind() {
			case core.ObjectKindCustomResourceDefinition:
				result, err = s.handleCustomResourceDefinition(event)
			default:
				err = fmt.Errorf(
					"unexpected object kind: %v",
					event.Key.ObjectKind(),
				)
			}
			if err := event.Respond(hz.EventResult{
				Result: result,
				Err:    err,
			}); err != nil {
				slog.Error("responding to event", "err", err)
			}
		}
	}()
	//
	// Start crd watcher
	//
	crdWatcher, err := hz.StartWatcher(
		ctx,
		s.Conn,
		hz.WithWatcherFor(core.CustomResourceDefinition{}),
		hz.WithWatcherCh(s.eventCh),
	)
	if err != nil {
		return fmt.Errorf("starting crd watcher: %w", err)
	}
	s.crdWatcher = crdWatcher

	// Wait for all watchers to initialize.
	init := make(chan struct{})
	go func() {
		<-s.crdWatcher.Init
		close(init)
	}()

	select {
	case <-init:
		// Do nothing and continue.
		s.init = true
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timed out waiting for watchers to initialize")
	}

	return nil
}

func (s *Schema) Close() error {
	s.crdWatcher.Close()
	close(s.eventCh)
	return nil
}

func (s *Schema) handleCustomResourceDefinition(
	event hz.Event,
) (hz.Result, error) {
	var crd core.CustomResourceDefinition
	if err := json.Unmarshal(event.Data, &crd); err != nil {
		return hz.Result{}, fmt.Errorf(
			"unmarshalling custom resource definition: %w",
			err,
		)
	}
	s.mx.Lock()
	defer s.mx.Unlock()
	s.definitions[hz.KeyFromObject(crd)] = &crd
	return hz.Result{}, nil
}
