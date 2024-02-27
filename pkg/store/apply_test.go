package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func (r DummyApplyObject) ObjectVersion() string {
	return "v1"
}

func (r DummyApplyObject) ObjectGroup() string {
	return "dummy"
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
	Command testStepCommand `json:"cmd,omitempty"`
	Manager string          `json:"manager,omitempty"`
	Status  *int            `json:"status,omitempty"`
	Force   bool            `json:"force,omitempty"`
}

func (t testStep) String() string {
	return fmt.Sprintf("%s:%s:%v", t.Command, t.Manager, t.Status)
}

func parseTestFileName(t *testing.T, file string) testStep {
	ts := testStep{}
	err := json.Unmarshal([]byte(file), &ts)
	tu.AssertNoError(t, err)
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
		hz.WithControllerValidatorCUE(false),
		hz.WithControllerValidatorForceNone(),
	)
	tu.AssertNoError(t, err)
	t.Cleanup(func() {
		_ = ctlr.Stop()
	})

	tu.AssertNoError(t, err)

	ar, err := txtar.ParseFile("./testdata/apply/1.txtar")
	tu.AssertNoError(t, err)
	for i, file := range ar.Files {
		ts := parseTestFileName(t, file.Name)
		testName := fmt.Sprintf("%d:%s", i, ts.String())
		t.Run(testName, func(t *testing.T) {
			client := hz.NewClient(
				ti.Conn,
				hz.WithClientInternal(true),
				hz.WithClientManager(ts.Manager),
			)
			switch ts.Command {
			case testStepCommandApply:
				jsonData, err := yaml.YAMLToJSON(file.Data)
				tu.AssertNoError(t, err, "obj yaml to json")
				obj := hz.GenericObject{}
				err = json.Unmarshal(jsonData, &obj)
				tu.AssertNoError(t, err, "unmarshal obj")

				err = client.Apply(
					ctx,
					hz.WithApplyObject(obj),
					hz.WithApplyForce(ts.Force),
				)
				if ts.Status == nil {
					tu.AssertNoError(t, err, "client apply")
					return
				}
				var applyErr *hz.Error
				if errors.As(err, &applyErr) {
					tu.AssertEqual(t, applyErr.Status, *ts.Status)
					return
				} else {
					t.Fatal("expected error status")
				}
			case testStepCommandAssert:
				expJSONData, err := yaml.YAMLToJSON(file.Data)
				tu.AssertNoError(t, err, "expObj yaml to json")
				expObj := hz.GenericObject{}
				err = json.Unmarshal(expJSONData, &expObj)
				tu.AssertNoError(t, err, "unmarshal obj")
				actObj, err := ti.Store.Get(ctx, store.GetRequest{Key: expObj})
				tu.AssertNoError(t, err, "client get")
				var exp, act interface{}
				err = json.Unmarshal(expJSONData, &exp)
				tu.AssertNoError(t, err, "unmarshal exp")
				err = json.Unmarshal(actObj, &act)
				tu.AssertNoError(t, err, "unmarshal act")
				tu.AssertEqual(t, exp, act, cmpOptIgnoreRevision)

			case testStepCommandError:
				t.Errorf("invalid test file name: %s", file.Name)
			default:
				t.Errorf("invalid test file name: %s", file.Name)

			}
		})
	}
}
