package hz

import (
	"context"
	"time"
)

const NamespaceRoot = "root"

type Reconciler interface {
	Reconcile(context.Context, Request) (Result, error)
}

type Request struct {
	// Key is the unique identifier of the object in the nats kv store.
	// It is the key of the object that is being reconciled.
	Key ObjectKeyer
}

// Result contains the result of a Reconciler invocation.
type Result struct {
	// Requeue tells the Controller to requeue the reconcile key.  Defaults to
	// false.
	Requeue bool

	// RequeueAfter if greater than 0, tells the Controller to requeue the
	// reconcile key after the Duration. Implies that Requeue is true, there is
	// no need to set Requeue to true at the same time as RequeueAfter.
	RequeueAfter time.Duration
}

// IsZero returns true if this result is empty.
func (r *Result) IsZero() bool {
	if r == nil {
		return true
	}
	return *r == Result{}
}
