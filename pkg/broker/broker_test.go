package broker_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/server"
	tu "github.com/verifa/horizon/pkg/testutil"
)

var _ (hz.Objecter) = (*DummyObject)(nil)

type DummyObject struct {
	hz.ObjectMeta `json:"metadata,omitempty" cue:""`

	Spec   struct{} `json:"spec,omitempty" cue:""`
	Status struct{} `json:"status,omitempty"`
}

func (o DummyObject) ObjectKind() string {
	return "DummyObject"
}

var _ (hz.Action[DummyObject]) = (*DummyAction)(nil)

type DummyAction struct{}

func (a DummyAction) Action() string {
	return "do"
}

func (a DummyAction) Do(
	ctx context.Context,
	obj DummyObject,
) (DummyObject, error) {
	// Do something with the object...
	return obj, nil
}

type timeoutAction struct{}

func (a timeoutAction) Action() string {
	return "timeout"
}

func (a timeoutAction) Do(
	ctx context.Context,
	obj DummyObject,
) (DummyObject, error) {
	time.Sleep(time.Minute)
	return obj, nil
}

type returnIDObject struct {
	hz.ObjectMeta `json:"metadata,omitempty"`

	Spec   returnIDSpec   `json:"spec"`
	Status returnIDStatus `json:"status"`
}

func (n returnIDObject) ObjectKind() string {
	return "returnID"
}

type returnIDSpec struct{}

type returnIDStatus struct {
	ID string `json:"id"`
}

type returnIDAction struct {
	ID string
}

func (a returnIDAction) Action() string {
	return "returnID"
}

func (a returnIDAction) Do(
	ctx context.Context,
	obj returnIDObject,
) (returnIDObject, error) {
	obj.Status.ID = a.ID
	return obj, nil
}

func TestBroker(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ti := server.Test(t, ctx)

	client := hz.NewClient(ti.Conn, hz.WithClientInternal(true))
	dummyClient := hz.ObjectClient[DummyObject]{Client: client}
	dObj := DummyObject{
		ObjectMeta: hz.ObjectMeta{
			Name:    "dummy",
			Account: "test",
		},
	}

	t.Run("withActors", func(t *testing.T) {
		// Start a bunch of actors.
		for i := 0; i < 10; i++ {
			actor, err := hz.StartActor[DummyObject](
				ctx,
				ti.Conn,
				hz.WithActorActioner(DummyAction{}),
			)
			tu.AssertNoError(t, err)
			t.Cleanup(func() {
				_ = actor.Stop()
			})
		}
		_, err := dummyClient.Run(ctx, DummyAction{}, dObj)
		tu.AssertNoError(t, err)
	})
	t.Run("withActorLabelSelector", func(t *testing.T) {
		ridObject := returnIDObject{
			ObjectMeta: hz.ObjectMeta{
				Name:    "returnID",
				Account: "test",
			},
		}
		ridClient := hz.ObjectClient[returnIDObject]{Client: client}
		// Start a bunch of actors.
		for i := 0; i < 10; i++ {
			actor, err := hz.StartActor[returnIDObject](
				ctx,
				ti.Conn,
				hz.WithActorActioner(returnIDAction{
					ID: strconv.Itoa(i),
				}),
				hz.WithActorLabels[returnIDObject](map[string]string{
					"num": strconv.Itoa(i),
				}),
			)
			tu.AssertNoError(t, err)
			t.Cleanup(func() {
				_ = actor.Stop()
			})
		}
		expNum := "3"
		reply, err := ridClient.Run(
			ctx,
			returnIDAction{},
			ridObject,
			hz.WithRunLabelSelector(hz.LabelSelector{
				MatchExpressions: []hz.LabelSelectorRequirement{
					{
						Key:      "num",
						Operator: hz.LabelSelectorOpIn,
						Values:   []string{expNum},
					},
				},
			}),
		)
		tu.AssertNoError(t, err)
		tu.AssertEqual(t, expNum, reply.Status.ID)
	})

	t.Run("noActors", func(t *testing.T) {
		_, err := dummyClient.Run(ctx, DummyAction{}, dObj)
		tu.AssertErrorIs(t, err, hz.ErrBrokerNoActorResponders)
	})
	t.Run("timeoutActor", func(t *testing.T) {
		actor, err := hz.StartActor[DummyObject](
			ctx,
			ti.Conn,
			hz.WithActorActioner(timeoutAction{}),
		)
		tu.AssertNoError(t, err)
		t.Cleanup(func() {
			_ = actor.Stop()
		})
		_, err = dummyClient.Run(ctx, timeoutAction{}, dObj)
		tu.AssertErrorIs(t, err, hz.ErrBrokerActorTimeout)
	})
}
