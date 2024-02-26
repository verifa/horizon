package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/server"
	"github.com/verifa/horizon/pkg/store"
	tu "github.com/verifa/horizon/pkg/testutil"
	"golang.org/x/tools/txtar"
	"sigs.k8s.io/yaml"
)

var cmpOptIgnoreRevision = cmp.FilterPath(func(p cmp.Path) bool {
	if len(p) != 4 {
		return false
	}
	return p.Index(1).String() == "[\"metadata\"]" &&
		p.Last().String() == "[\"revision\"]"
}, cmp.Ignore())

type DummyApplyObject struct {
	hz.ObjectMeta `json:"metadata"`
	Spec          struct{} `json:"spec"`
}

func (r DummyApplyObject) ObjectAPIVersion() string {
	return "v1"
}

func (r DummyApplyObject) ObjectGroup() string {
	return "DummyApplyGroup"
}

func (r DummyApplyObject) ObjectKind() string {
	return "DummyApplyObject"
}

type testStepCommand string

const (
	testStepCommandError  testStepCommand = "error"
	testStepCommandApply  testStepCommand = "apply"
	testStepCommandAssert testStepCommand = "assert"
)

type testStep struct {
	command   testStepCommand
	manager   string
	errStatus *int
}

type testStepConflict struct{}

func parseTestFileName(t *testing.T, file string) testStep {
	parts := strings.Split(file, ":")
	ts := testStep{}
	for i, part := range parts {
		switch i {
		case 0:
			ts.command = testStepCommand(part)
		case 1:
			ts.manager = part
		case 2:
			status, err := strconv.Atoi(part)
			tu.AssertNoError(t, err)
			ts.errStatus = &status
		default:
			ts.command = testStepCommandError
		}
	}
	return ts
}

func TestApply(t *testing.T) {
	ctx := context.Background()

	ti := server.Test(t, ctx)
	// SETUP DUMMY CONTROLLER
	ctlr, err := hz.StartController(
		ctx,
		ti.Conn,
		hz.WithControllerFor(DummyApplyObject{}),
		hz.WithControllerValidatorForceNone(),
	)
	tu.AssertNoError(t, err)
	t.Cleanup(func() {
		_ = ctlr.Stop()
	})

	tu.AssertNoError(t, err)
	key := hz.ObjectKey{
		Group:   "DummyApplyGroup",
		Kind:    "DummyApplyObject",
		Name:    "test",
		Account: "test",
	}

	ar, err := txtar.ParseFile("./testdata/apply/1.txtar")
	tu.AssertNoError(t, err)
	for _, file := range ar.Files {
		ts := parseTestFileName(t, file.Name)
		client := hz.NewClient(
			ti.Conn,
			hz.WithClientInternal(true),
			hz.WithClientManager(ts.manager),
		)
		switch ts.command {
		case testStepCommandApply:
			obj, err := yaml.YAMLToJSON([]byte(file.Data))
			tu.AssertNoError(t, err, "obj yaml to json")
			err = client.Apply(
				ctx,
				hz.WithApplyKey(key),
				hz.WithApplyData(obj),
			)
			if ts.errStatus == nil {
				tu.AssertNoError(t, err, "client apply")
				return
			}
			var applyErr *hz.Error
			if errors.As(err, &applyErr) {
				tu.AssertEqual(t, applyErr.Status, *ts.errStatus)
				return
			}
		case testStepCommandAssert:
			expObj, err := yaml.YAMLToJSON([]byte(file.Data))
			tu.AssertNoError(t, err, "expObj yaml to json")
			actObj, err := ti.Store.Get(ctx, store.GetRequest{Key: key})
			tu.AssertNoError(t, err, "client get")
			var exp, act interface{}
			err = json.Unmarshal(expObj, &exp)
			tu.AssertNoError(t, err, "unmarshal exp")
			err = json.Unmarshal(actObj, &act)
			tu.AssertNoError(t, err, "unmarshal act")
			tu.AssertEqual(t, exp, act, cmpOptIgnoreRevision)

		case testStepCommandError:
			t.Errorf("invalid test file name: %s", file.Name)
		default:
			t.Errorf("invalid test file name: %s", file.Name)

		}
	}
}
