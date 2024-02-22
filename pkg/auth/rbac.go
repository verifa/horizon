package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/pkg/hz"
)

type RBAC struct {
	Conn         *nats.Conn
	RoleBindings map[string]RoleBinding `json:"roleBindings,omitempty"`
	Roles        map[string]Role        `json:"roles,omitempty"`

	Permissions map[string]*Group `json:"permissions,omitempty"`

	mx       sync.RWMutex
	watchers []*hz.Watcher
}

func (r *RBAC) Start(ctx context.Context) error {
	//
	// Start role watcher
	//
	roleWatcher, err := hz.StartWatcher(
		ctx,
		r.Conn,
		hz.WithWatcherForObject(Role{}),
		hz.WithWatcherFn(r.HandleRoleEvent),
	)
	if err != nil {
		return fmt.Errorf("starting role watcher: %w", err)
	}
	r.watchers = append(r.watchers, roleWatcher)
	//
	// Start rolebinding watcher
	//
	roleBindingWatcher, err := hz.StartWatcher(
		ctx,
		r.Conn,
		hz.WithWatcherForObject(RoleBinding{}),
		hz.WithWatcherFn(r.HandleRoleBindingEvent),
	)
	if err != nil {
		return fmt.Errorf("starting rolebinding watcher: %w", err)
	}
	r.watchers = append(r.watchers, roleBindingWatcher)

	// Wait for all watchers to initialize.
	init := make(chan struct{})
	go func() {
		for _, w := range r.watchers {
			<-w.Init
		}
		close(init)
	}()

	select {
	case <-init:
		// Do nothing and continue.
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timed out waiting for watchers to initialize")
	}

	return nil
}

type Group struct {
	Name     string
	Accounts map[string]*Permissions
}

type Permissions struct {
	Allow []Verbs `json:"allow"`
	Deny  []Verbs `json:"deny"`
}

// ListObjects will return the list of objects a user has access to read.
func (r *RBAC) ListObjects() []hz.Objecter {
	return nil
}

type AccountRequest struct {
	User    string
	Groups  []string
	Account string
}

type Verb string

const (
	// VerbRead is the lowest level of allow access.
	// VerbRead is the highest level of deny access.
	// If you are denied read access, you are denied all levels of access.
	VerbRead Verb = "read"
	// VerbUpdate allows a user to update objects.
	// It implies VerbRead.
	VerbUpdate Verb = "update"
	// VerbCreate allows a user to create objects.
	// It implies VerbRead.
	VerbCreate Verb = "create"
	// VerbDelete allows a user to delete objects.
	// It implies VerbRead.
	VerbDelete Verb = "delete"
	// VerbRun allows a user to run actions for an actor.
	VerbRun Verb = "run"
)

type RBACRequest struct {
	Groups []string
	Verb   Verb
	Object hz.ObjectKeyer
}

func (r *RBAC) Check(ctx context.Context, req RBACRequest) bool {
	r.mx.RLock()
	defer r.mx.RUnlock()

	isAllow := false
	isDeny := false
	for _, gr := range req.Groups {
		group, ok := r.Permissions[gr]
		if !ok {
			continue
		}
		account, ok := group.Accounts[req.Object.ObjectAccount()]
		if !ok {
			continue
		}

		if !isAllow {
			for _, allow := range account.Allow {
				switch req.Verb {
				case VerbRead:
					isAllow = checkVerbFilter(allow.Read, req.Object) ||
						checkVerbFilter(allow.Update, req.Object) ||
						checkVerbFilter(allow.Create, req.Object) ||
						checkVerbFilter(allow.Delete, req.Object)

				case VerbUpdate:
					isAllow = checkVerbFilter(allow.Update, req.Object)
				case VerbCreate:
					isAllow = checkVerbFilter(allow.Create, req.Object)
				case VerbDelete:
					isAllow = checkVerbFilter(allow.Delete, req.Object)
				case VerbRun:
					isAllow = checkVerbFilter(allow.Run, req.Object)
				default:
					// Unknown verb.
					return false
				}
				if isAllow {
					break
				}
			}
		}
		if !isDeny {
			for _, deny := range account.Deny {
				switch req.Verb {
				case VerbRead:
					isDeny = checkVerbFilter(deny.Read, req.Object)
				case VerbUpdate:
					isDeny = checkVerbFilter(deny.Read, req.Object) ||
						checkVerbFilter(deny.Update, req.Object)
				case VerbCreate:
					isDeny = checkVerbFilter(deny.Read, req.Object) ||
						checkVerbFilter(deny.Create, req.Object)
				case VerbDelete:
					isDeny = checkVerbFilter(deny.Read, req.Object) ||
						checkVerbFilter(deny.Delete, req.Object)
				case VerbRun:
					isDeny = checkVerbFilter(deny.Run, req.Object)
				default:
					// Unknown verb.
					return false
				}
				if isDeny {
					break
				}
			}
		}
	}
	return isAllow && !isDeny
}

func checkVerbFilter(vf *VerbFilter, obj hz.ObjectKeyer) bool {
	if vf == nil {
		return false
	}
	if !checkStringPattern(vf.Name, obj.ObjectName()) {
		return false
	}
	if !checkStringPattern(vf.Kind, obj.ObjectKind()) {
		return false
	}
	if !checkStringPattern(vf.Group, obj.ObjectGroup()) {
		return false
	}

	return true
}

// checkStringPattern checks if the value matches the pattern.
// The pattern matching is very basic, it is either:
//   - an exact string match
//   - a prefix match with a trailing "*"
//
// Everything after the optional "*" is ignored.
//
// E.g.
//   - "foo" matches "foo"
//   - "foo*" matches "foobar"
//   - "foo" does not match "foobar"
//   - "foo*bar" does not match "foobar"
func checkStringPattern(pattern *string, value string) bool {
	if pattern != nil && *pattern != "*" {
		prefix, ok := strings.CutSuffix(*pattern, "*")
		if ok {
			if !strings.HasPrefix(value, prefix) {
				return false
			}
		} else {
			if *pattern != value {
				return false
			}
		}
	}
	return true
}

func (r *RBAC) HandleRoleBindingEvent(event hz.Event) (hz.Result, error) {
	var rb RoleBinding
	if err := json.Unmarshal(event.Data, &rb); err != nil {
		return hz.Result{}, fmt.Errorf("unmarshalling role binding: %w", err)
	}

	switch event.Operation {
	case hz.EventOperationPut:
		r.RoleBindings[hz.KeyFromObject(rb)] = rb
	case hz.EventOperationDelete, hz.EventOperationPurge:
		delete(r.RoleBindings, hz.KeyFromObject(rb))
	default:
		return hz.Result{}, fmt.Errorf(
			"unexpected event operation: %v",
			event.Operation,
		)
	}

	r.refresh()
	return hz.Result{}, nil
}

func (r *RBAC) HandleRoleEvent(event hz.Event) (hz.Result, error) {
	var role Role
	if err := json.Unmarshal(event.Data, &role); err != nil {
		return hz.Result{}, fmt.Errorf("unmarshalling role: %w", err)
	}

	switch event.Operation {
	case hz.EventOperationPut:
		r.Roles[hz.KeyFromObject(role)] = role
	case hz.EventOperationDelete, hz.EventOperationPurge:
		delete(r.Roles, hz.KeyFromObject(role))
	default:
		return hz.Result{}, fmt.Errorf(
			"unexpected event operation: %v",
			event.Operation,
		)
	}

	r.refresh()
	return hz.Result{}, nil
}

func (r *RBAC) refresh() {
	cache := make(map[string]*Group)
	for _, roleBinding := range r.RoleBindings {
		for _, subject := range roleBinding.Spec.Subjects {
			if subject.Kind != "Group" {
				continue
			}

			// Get group object, or create if not exists.
			group, ok := cache[subject.Name]
			if !ok {
				group = &Group{
					Name:     subject.Name,
					Accounts: make(map[string]*Permissions),
				}
				cache[subject.Name] = group
			}

			// Get permissions for the account, or create if not exists.
			permissions, ok := group.Accounts[roleBinding.Account]
			if !ok {
				permissions = &Permissions{
					Allow: []Verbs{},
					Deny:  []Verbs{},
				}
				group.Accounts[roleBinding.Account] = permissions
			}

			roleKey := hz.KeyFromObject(hz.ObjectKey{
				Name:    roleBinding.Spec.RoleRef.Name,
				Group:   "hz-internal",
				Kind:    "Role",
				Account: roleBinding.Account,
			})
			// Get the role key. It should exist.
			// A RoleBinding cannot be created with the Role.
			role, ok := r.Roles[roleKey]
			if !ok {
				// Might be that the role does not exist yet.
				// No worries, once the role gets created this gets re-run.
				slog.Error(
					"role not found",
					"role",
					roleKey,
					"roleBinding",
					hz.KeyFromObject(roleBinding),
				)
				return
			}

			for _, allowRule := range role.Spec.Allow {
				permissions.Allow = append(permissions.Allow, allowRule)
			}
			for _, denyRule := range role.Spec.Deny {
				permissions.Deny = append(permissions.Deny, denyRule)
			}
		}
	}

	// Add implicit account permissions based on Group<-->Account
	// relations.
	// If a group has any relation to an account, we should give it read
	// access to the account object, implicitly.
	for _, group := range cache {
		for accountName, permissions := range group.Accounts {
			if accountName == hz.RootAccount {
				continue
			}
			localAccount := accountName

			if len(permissions.Allow) > 0 {
				rootAccount, ok := group.Accounts[hz.RootAccount]
				if !ok {
					rootAccount = &Permissions{
						Allow: []Verbs{},
						Deny:  []Verbs{},
					}
					group.Accounts[hz.RootAccount] = rootAccount
				}

				rootAccount.Allow = append(rootAccount.Allow, Verbs{
					Read: &VerbFilter{
						Name: &localAccount,
						Kind: hz.P("Account"),
						// Group: hz.P("TODO"),
					},
				})
			}
		}
	}

	r.mx.Lock()
	defer r.mx.Unlock()
	r.Permissions = cache
}
