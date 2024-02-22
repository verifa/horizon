package store_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/server"
	tu "github.com/verifa/horizon/pkg/testutil"
)

var cmpOptIgnoreMetaRevision = cmp.FilterPath(func(p cmp.Path) bool {
	if len(p) != 4 {
		return false
	}
	return p.Last().String() == ".Revision" &&
		p.Last().Type() == reflect.TypeOf(new(uint64))
}, cmp.Ignore())

func TestList(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ti := server.Test(t, ctx)

	// SETUP DUMMY CONTROLLER
	ctlr, err := hz.StartController(
		ctx,
		ti.Conn,
		hz.WithControllerFor(DummyApplyObject{}),
		hz.WithControllerValidatorCUE(),
	)
	tu.AssertNoError(t, err)
	t.Cleanup(func() {
		_ = ctlr.Stop()
	})

	client := hz.ObjectClient[DummyApplyObject]{
		Client: hz.InternalClient(ti.Conn),
	}
	// Create a dummy object
	obj1 := DummyApplyObject{
		ObjectMeta: hz.ObjectMeta{
			Name:    "obj1",
			Account: "test",
		},
	}
	if err := client.Create(ctx, obj1); err != nil {
		t.Fatal("creating obj1: ", err)
	}

	objs, err := client.List(ctx)
	tu.AssertNoError(t, err)
	tu.AssertEqual(t, 1, len(objs))
	// Create a dummy object
	obj2 := DummyApplyObject{
		ObjectMeta: hz.ObjectMeta{
			Name:    "obj2",
			Account: "test",
		},
	}
	if err := client.Create(ctx, obj2); err != nil {
		t.Fatal("creating obj2: ", err)
	}
	objs, err = client.List(ctx)
	tu.AssertNoError(t, err)
	tu.AssertEqual(t, 2, len(objs))
	tu.AssertEqual(t, &obj1, objs[0], cmpOptIgnoreMetaRevision)
	tu.AssertEqual(t, &obj2, objs[1], cmpOptIgnoreMetaRevision)
}
