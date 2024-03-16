package hz_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/server"
	"github.com/verifa/horizon/pkg/store"
	tu "github.com/verifa/horizon/pkg/testutil"
)

type DummyReconciler struct {
	DummyClient hz.ObjectClient[DummyObject]
}

func (r *DummyReconciler) Reconcile(
	ctx context.Context,
	request hz.Request,
) (hz.Result, error) {
	return hz.Result{}, nil
}

type ChildReconciler struct{}

func (r *ChildReconciler) Reconcile(
	ctx context.Context,
	request hz.Request,
) (hz.Result, error) {
	return hz.Result{}, nil
}

func TestReconciler(t *testing.T) {
	ctx := context.Background()
	ti := server.Test(t, ctx)

	client := hz.NewClient(
		ti.Conn,
		hz.WithClientInternal(true),
	)
	dummyClient := hz.ObjectClient[DummyObject]{Client: client}
	dr := DummyReconciler{
		DummyClient: dummyClient,
	}
	ctlr, err := hz.StartController(
		ctx,
		ti.Conn,
		hz.WithControllerReconciler(&dr),
		hz.WithControllerFor(&DummyObject{}),
		hz.WithControllerOwns(&ChildObject{}),
	)
	tu.AssertNoError(t, err)
	defer ctlr.Stop()

	// Start controller for child object.
	childCtlr, err := hz.StartController(
		ctx,
		ti.Conn,
		hz.WithControllerReconciler(&ChildReconciler{}),
		hz.WithControllerFor(&ChildObject{}),
	)
	tu.AssertNoError(t, err)
	defer childCtlr.Stop()

	do := DummyObject{
		ObjectMeta: hz.ObjectMeta{
			Account: "test",
			Name:    "dummy",
		},
	}
	err = dummyClient.Create(ctx, do)
	tu.AssertNoError(t, err)

	childClient := hz.ObjectClient[ChildObject]{Client: client}
	co := ChildObject{
		ObjectMeta: hz.ObjectMeta{
			Account: "test",
			Name:    "child",
			OwnerReferences: []hz.OwnerReference{
				{
					Group:   do.ObjectGroup(),
					Version: do.ObjectVersion(),
					Kind:    do.ObjectKind(),
					Account: do.Account,
					Name:    do.Name,
				},
			},
		},
	}

	err = childClient.Create(ctx, co)
	tu.AssertNoError(t, err)

	time.Sleep(time.Second * 1)
}

type PanicReconciler struct {
	wg sync.WaitGroup
}

func (r *PanicReconciler) Reconcile(
	ctx context.Context,
	request hz.Request,
) (hz.Result, error) {
	r.wg.Done()
	panic("PanicReconciler be good at one thing...")
}

func TestReconcilerPanic(t *testing.T) {
	ctx := context.Background()
	ti := server.Test(t, ctx)

	client := hz.NewClient(
		ti.Conn,
		hz.WithClientInternal(true),
		hz.WithClientManager("test"),
	)
	dummyClient := hz.ObjectClient[DummyObject]{Client: client}
	pr := PanicReconciler{}
	pr.wg.Add(2)
	ctlr, err := hz.StartController(
		ctx,
		ti.Conn,
		hz.WithControllerReconciler(&pr),
		hz.WithControllerFor(&DummyObject{}),
	)
	tu.AssertNoError(t, err)
	defer ctlr.Stop()

	do := DummyObject{
		ObjectMeta: hz.ObjectMeta{
			Account: "test",
			Name:    "dummy",
		},
	}

	err = dummyClient.Create(ctx, do)
	tu.AssertNoError(t, err)
	// If we publish messages too quickly the reconciler will only get the last.
	// Add a little sleep to make sure both messages get handled.
	time.Sleep(time.Second)
	_, err = dummyClient.Apply(ctx, do)
	tu.AssertNoError(t, err)

	done := make(chan struct{})
	go func() {
		pr.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second * 5):
		t.Fatal("timed out waiting for panic reconciler")
	}
}

type SlowReconciler struct {
	wg sync.WaitGroup
}

func (r *SlowReconciler) Reconcile(
	ctx context.Context,
	request hz.Request,
) (hz.Result, error) {
	dur := time.Second * 3
	fmt.Println("SlowReconciler is sleeping for ", dur.String())
	// Simulate a long running process...
	time.Sleep(dur)
	r.wg.Done()
	return hz.Result{
		Requeue: true,
	}, nil
}

func TestReconcilerSlow(t *testing.T) {
	ctx := context.Background()
	lockTTL := time.Second
	ti := server.Test(
		t,
		ctx,
		server.WithStoreOptions(store.WithMutexTTL(lockTTL)),
	)

	client := hz.NewClient(
		ti.Conn,
		hz.WithClientInternal(true),
		hz.WithClientManager("test"),
	)
	dummyClient := hz.ObjectClient[DummyObject]{Client: client}

	sr := SlowReconciler{}
	sr.wg.Add(2)
	ctlr, err := hz.StartController(
		ctx,
		ti.Conn,
		hz.WithControllerReconciler(&sr),
		hz.WithControllerFor(&DummyObject{}),
	)
	tu.AssertNoError(t, err)
	t.Cleanup(func() {
		ctlr.Stop()
	})

	do := DummyObject{
		ObjectMeta: hz.ObjectMeta{
			Account: "test",
			Name:    "dummy",
		},
	}

	_, err = dummyClient.Apply(ctx, do)
	tu.AssertNoError(t, err)
	// If we publish messages too quickly the reconciler will only get the last
	// message, so add a minor delay.
	time.Sleep(time.Millisecond * 100)
	_, err = dummyClient.Apply(ctx, do)
	tu.AssertNoError(t, err)

	done := make(chan struct{})
	go func() {
		sr.wg.Wait()
		close(done)
	}()

	timeBefore := time.Now()
	select {
	case <-done:
		t.Log("Slow reconciler finished")
	case <-time.After(time.Second * 15):
		t.Fatal("timed out waiting for slow reconciler")
	}
	if time.Since(timeBefore) < time.Second*5 {
		t.Fatal("reconciler did not wait for slow reconciler to run twice")
	}
}

type SleepReconciler struct {
	dur time.Duration
}

func (r *SleepReconciler) Reconcile(
	ctx context.Context,
	request hz.Request,
) (hz.Result, error) {
	fmt.Println("SleepReconciler is sleeping for ", r.dur.String())
	// Simulate a long running process...
	time.Sleep(r.dur)
	return hz.Result{}, nil
}

// TestReconcilerWaitForFinish tests that the controller waits for the
// reconciler to finish before stopping.
func TestReconcilerWaitForFinish(t *testing.T) {
	ctx := context.Background()
	ti := server.Test(t, ctx)

	client := hz.NewClient(
		ti.Conn,
		hz.WithClientInternal(true),
		hz.WithClientDefaultManager(),
	)
	dummyClient := hz.ObjectClient[DummyObject]{Client: client}

	sr := SleepReconciler{
		dur: time.Second * 3,
	}
	ctlr, err := hz.StartController(
		ctx,
		ti.Conn,
		hz.WithControllerReconciler(&sr),
		hz.WithControllerFor(&DummyObject{}),
	)
	tu.AssertNoError(t, err)

	do := DummyObject{
		ObjectMeta: hz.ObjectMeta{
			Account: "test",
			Name:    "dummy",
		},
	}

	_, err = dummyClient.Apply(ctx, do)
	tu.AssertNoError(t, err)
	// Wait just a moment, before stopping the controller.
	time.Sleep(time.Millisecond * 100)
	timeBefore := time.Now()
	err = ctlr.Stop()
	tu.AssertNoError(t, err)

	if time.Since(timeBefore) < sr.dur-time.Second {
		t.Fatal("controller did not wait for slow reconciler to finish")
	}
}

// ConcurrentReconciler is made to test that a reconciler is NEVER called
// concurrently for the same object.
type ConcurrentReconciler struct {
	ch chan int
}

// Reconcile uses a channel to increment when the loop starts, and
// decrement when it is finished.
// It will sleep in the middle to simulate some longer running task.
// The test should check that the sum of the channel never goes above 1.
func (r *ConcurrentReconciler) Reconcile(
	ctx context.Context,
	request hz.Request,
) (hz.Result, error) {
	r.ch <- 1
	// Sleep for a little bit to simulate a long running process.
	// This will allow us to test that the reconciler is not called
	// concurrently.
	time.Sleep(time.Second * 3)
	r.ch <- -1
	return hz.Result{
		Requeue: true,
	}, nil
}

func TestReconcilerConcurrent(t *testing.T) {
	ctx := context.Background()
	ti := server.Test(t, ctx)

	client := hz.NewClient(
		ti.Conn,
		hz.WithClientInternal(true),
		hz.WithClientManager("test"),
	)
	dummyClient := hz.ObjectClient[DummyObject]{Client: client}
	childClient := hz.ObjectClient[ChildObject]{Client: client}

	sumCh := make(chan int)
	cr := ConcurrentReconciler{
		ch: sumCh,
	}
	// Start a few instances of the controller.
	for i := 0; i < 5; i++ {
		ctlr, err := hz.StartController(
			ctx,
			ti.Conn,
			hz.WithControllerReconciler(&cr),
			hz.WithControllerFor(&DummyObject{}),
		)
		tu.AssertNoError(t, err)
		defer ctlr.Stop()
	}
	// Start controller for child object
	childCtlr, err := hz.StartController(
		ctx,
		ti.Conn,
		hz.WithControllerReconciler(&ChildReconciler{}),
		hz.WithControllerFor(&ChildObject{}),
	)
	tu.AssertNoError(t, err)
	defer childCtlr.Stop()

	failCh := make(chan struct{})
	go func() {
		sum := 0
		for i := range sumCh {
			sum += i
			if sum > 1 {
				close(failCh)
				return
			}
		}
	}()

	do := DummyObject{
		ObjectMeta: hz.ObjectMeta{
			Account: "test",
			Name:    "dummy",
		},
	}
	co := ChildObject{
		ObjectMeta: hz.ObjectMeta{
			Account: "test",
			Name:    "child",
			OwnerReferences: []hz.OwnerReference{
				{
					Group:   do.ObjectGroup(),
					Version: do.ObjectVersion(),
					Kind:    do.ObjectKind(),
					Name:    do.Name,
					Account: do.Account,
				},
			},
		},
	}
	go func() {
		err = dummyClient.Create(ctx, do)
		tu.AssertNoError(t, err)
		err = childClient.Create(ctx, co)
		tu.AssertNoError(t, err)
		for i := 0; i < 50; i++ {
			_, err = dummyClient.Apply(ctx, do)
			tu.AssertNoError(t, err)
			_, err = childClient.Apply(ctx, co)
			tu.AssertNoError(t, err)
		}
	}()

	select {
	case <-time.After(time.Second * 10):
		// All good.
		return
	case <-failCh:
		// Concurrent call occurred!
		t.Fatal("concurent reconciler was called concurrently")
	}
}
