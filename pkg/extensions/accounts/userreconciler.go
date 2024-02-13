package accounts

import (
	"context"
	"fmt"

	"github.com/verifa/horizon/pkg/hz"
)

const userCtlrName = "ctlr-user"

var _ hz.Reconciler = (*UserReconciler)(nil)

type UserReconciler struct {
	Client hz.Client
}

func (r *UserReconciler) Reconcile(
	ctx context.Context,
	req hz.Request,
) (hz.Result, error) {
	userClient := hz.ObjectClient[User]{Client: r.Client}
	user, err := userClient.Get(ctx, req.Key)
	if err != nil {
		return hz.Result{}, hz.IgnoreNotFound(err)
	}
	if user.Spec.Claims == nil {
		return hz.Result{}, fmt.Errorf("user %q has no claims", req.Key)
	}
	claims := *user.Spec.Claims

	if len(claims.Groups) == 0 {
		return hz.Result{}, nil
	}
	groupClient := hz.ObjectClient[Group]{Client: r.Client}
	memberClient := hz.ObjectClient[Member]{Client: r.Client}
	// For each group the user has a group claim for, check if the group exists
	// in any account.
	// For each account that a group exists in, check if the user is a member of
	// that group.
	for _, userGroup := range claims.Groups {
		groups, err := groupClient.List(
			ctx,
			hz.WithListKeyFromObject(hz.ObjectKey[Group]{
				Name:    userGroup,
				Account: "*",
			}),
		)
		if err != nil {
			return hz.Result{}, fmt.Errorf(
				"getting groups for %q: %w",
				userGroup,
				err,
			)
		}
		for _, group := range groups {
			memberKey := hz.ObjectKey[Member]{
				Name:    memberObjectName(group.Name, user.Name),
				Account: group.Account,
			}
			// If the group exists, check that membership exists.
			_, err := memberClient.Get(
				ctx,
				hz.KeyForObject(memberKey),
			)
			if err != nil {
				if hz.IgnoreNotFound(err) != nil {
					return hz.Result{}, fmt.Errorf(
						"getting member %q: %w",
						memberKey,
						err,
					)
				}
				// If member does not exist, we need to create it.
				member := Member{
					ObjectMeta: hz.ObjectMeta{
						Name:    memberKey.Name,
						Account: memberKey.Account,
						OwnerReferences: []hz.OwnerReference{
							{
								Kind:    group.ObjectKind(),
								Name:    group.Name,
								Account: group.Account,
							},
							{
								Kind:    user.ObjectKind(),
								Name:    user.Name,
								Account: user.Account,
							},
						},
					},
					Spec: MemberSpec{
						GroupRef: &GroupRef{
							Name: &group.Name,
						},
						UserRef: &UserRef{
							Name: &user.Name,
						},
					},
				}
				if err := memberClient.Apply(ctx, member, hz.WithApplyManager(userCtlrName)); err != nil {
					return hz.Result{}, fmt.Errorf(
						"applying member %q: %w",
						memberKey,
						err,
					)
				}
				continue
			}
			// TODO: handle if the membership has been modified?
		}

	}

	return hz.Result{}, nil
}

func memberObjectName(groupName string, userName string) string {
	return fmt.Sprintf("%s-%s", groupName, userName)
}
