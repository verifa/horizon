package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

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
	testStepCommandApply        testStepCommand = "apply"
	testStepCommandCreate       testStepCommand = "create"
	testStepCommandDelete       testStepCommand = "delete"
	testStepCommandAssert       testStepCommand = "assert"
	testStepCommandAssertDelete testStepCommand = "assert_delete"
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

func TestStore(t *testing.T) {
	ctx := context.Background()
	txtarFiles, err := filepath.Glob("./testdata/*.txtar")
	tu.AssertNoError(t, err)
	for _, txtarFile := range txtarFiles {
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
		t.Run(txtarFile, func(t *testing.T) {
			ar, err := txtar.ParseFile(txtarFile)
			tu.AssertNoError(t, err)
			runTest(t, ctx, ti.Store, ar)
		})
	}
}

func runTest(
	t *testing.T,
	ctx context.Context,
	st *store.Store,
	ar *txtar.Archive,
) {
	for i, file := range ar.Files {
		ts := parseTestFileName(t, file.Name)
		testName := fmt.Sprintf("%d:%s", i, ts.String())
		t.Run(testName, func(t *testing.T) {
			client := hz.NewClient(
				st.Conn,
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

				_, err = client.Apply(
					ctx,
					hz.WithApplyObject(obj),
					hz.WithApplyForce(ts.Force),
				)
				if ts.Status == nil {
					tu.AssertNoError(t, err)
				} else {
					var getErr *hz.Error
					if errors.As(err, &getErr) {
						tu.AssertEqual(t, getErr.Status, *ts.Status)
						return
					} else {
						t.Fatal("expected hz.Error")
					}
				}
			case testStepCommandCreate:
				jsonData, err := yaml.YAMLToJSON(file.Data)
				tu.AssertNoError(t, err, "obj yaml to json")
				err = client.Create(ctx, hz.WithCreateData(jsonData))
				if ts.Status == nil {
					tu.AssertNoError(t, err)
				} else {
					var getErr *hz.Error
					if errors.As(err, &getErr) {
						tu.AssertEqual(t, getErr.Status, *ts.Status)
						return
					} else {
						t.Fatal("expected hz.Error")
					}
				}

			case testStepCommandDelete:
				jsonData, err := yaml.YAMLToJSON(file.Data)
				tu.AssertNoError(t, err, "obj yaml to json")
				obj := hz.GenericObject{}
				err = json.Unmarshal(jsonData, &obj)
				tu.AssertNoError(t, err, "unmarshal obj")

				err = client.Delete(
					ctx,
					hz.WithDeleteObject(obj),
				)
				if ts.Status == nil {
					tu.AssertNoError(t, err)
				} else {
					var getErr *hz.Error
					if errors.As(err, &getErr) {
						tu.AssertEqual(t, getErr.Status, *ts.Status)
						return
					} else {
						t.Fatal("expected hz.Error")
					}
				}

			case testStepCommandAssert:
				expJSONData, err := yaml.YAMLToJSON(file.Data)
				tu.AssertNoError(t, err, "expObj yaml to json")
				expObj := hz.GenericObject{}
				err = json.Unmarshal(expJSONData, &expObj)
				tu.AssertNoError(t, err, "unmarshal obj")
				actObj, err := st.Get(ctx, store.GetRequest{Key: expObj})
				if ts.Status == nil {
					tu.AssertNoError(t, err)
				} else {
					var getErr *hz.Error
					if errors.As(err, &getErr) {
						tu.AssertEqual(t, getErr.Status, *ts.Status)
						return
					} else {
						t.Fatal("expected hz.Error")
					}
				}

				var exp, act interface{}
				err = json.Unmarshal(expJSONData, &exp)
				tu.AssertNoError(t, err, "unmarshal exp")
				err = json.Unmarshal(actObj, &act)
				tu.AssertNoError(t, err, "unmarshal act")
				tu.AssertEqual(t, exp, act, cmpOptIgnoreRevision)
			case testStepCommandAssertDelete:
				expJSONData, err := yaml.YAMLToJSON(file.Data)
				tu.AssertNoError(t, err, "expObj yaml to json")
				expObj := hz.GenericObject{}
				err = json.Unmarshal(expJSONData, &expObj)
				tu.AssertNoError(t, err, "unmarshal obj")

				done := make(chan struct{})
				watcher, err := hz.StartWatcher(
					ctx,
					st.Conn,
					hz.WithWatcherFor(expObj),
					hz.WithWatcherFn(func(event hz.Event) (hz.Result, error) {
						if event.Operation == hz.EventOperationPurge {
							close(done)
						}
						return hz.Result{}, nil
					}),
				)
				tu.AssertNoError(t, err, "start watcher")
				t.Cleanup(func() {
					watcher.Close()
				})
				select {
				case <-done:
				case <-time.After(time.Second * 5):
					t.Fatal("timed out waiting for purge event")
				}

			default:
				t.Errorf("invalid test file name: %s", file.Name)

			}
		})
	}
}
