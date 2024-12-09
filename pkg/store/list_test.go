package store_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/verifa/horizon/pkg/controller"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/internal/managedfields"
	"github.com/verifa/horizon/pkg/server"
	tu "github.com/verifa/horizon/pkg/testutil"
)

var cmpOptIgnoreMetaRevision = cmp.FilterPath(func(p cmp.Path) bool {
	return p.Last().String() == ".Revision" &&
		p.Last().Type() == reflect.TypeOf(new(uint64))
}, cmp.Ignore())

var cmpOptIgnoreMetaManagedFields = cmp.FilterPath(func(p cmp.Path) bool {
	return p.Last().String() == ".ManagedFields" &&
		p.Last().Type() == reflect.TypeOf(managedfields.ManagedFields{})
}, cmp.Ignore())

func TestList(t *testing.T) {
	ctx := context.Background()
	ti := server.Test(t, ctx)

	// SETUP DUMMY CONTROLLER
	ctlr, err := controller.Start(
		ctx,
		ti.Conn,
		controller.WithFor(DummyApplyObject{}),
	)
	tu.AssertNoError(t, err)
	t.Cleanup(func() {
		_ = ctlr.Stop()
	})

	client := hz.ObjectClient[DummyApplyObject]{
		Client: hz.NewClient(ti.Conn, hz.WithClientInternal(true)),
	}
	// Create a dummy object
	obj1 := DummyApplyObject{
		ObjectMeta: hz.ObjectMeta{
			Name:      "obj1",
			Namespace: "test",
		},
	}
	if _, err := client.Apply(ctx, obj1); err != nil {
		t.Fatal("creating obj1: ", err)
	}

	objs, err := client.List(ctx)
	tu.AssertNoError(t, err)
	tu.AssertEqual(t, 1, len(objs))
	// Create a dummy object
	obj2 := DummyApplyObject{
		ObjectMeta: hz.ObjectMeta{
			Name:      "obj2",
			Namespace: "test",
		},
	}
	if _, err := client.Apply(ctx, obj2); err != nil {
		t.Fatal("creating obj2: ", err)
	}
	objs, err = client.List(ctx)
	tu.AssertNoError(t, err)
	tu.AssertEqual(t, 2, len(objs))
	tu.AssertEqual(
		t,
		obj1,
		objs[0],
		cmpOptIgnoreMetaRevision,
		cmpOptIgnoreMetaManagedFields,
	)
	tu.AssertEqual(
		t,
		obj2,
		objs[1],
		cmpOptIgnoreMetaRevision,
		cmpOptIgnoreMetaManagedFields,
	)
}
