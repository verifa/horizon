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
	"github.com/verifa/horizon/pkg/extensions/core"
	"github.com/verifa/horizon/pkg/hz"
)

type RBAC struct {
	Conn *nats.Conn
	// TODO: RoleBindings and Roles maps are not thread safe.
	// E.g. HandleRoleEvent and refresh both write and read from Roles.
	RoleBindings map[string]RoleBinding `json:"roleBindings,omitempty"`
	Roles        map[string]Role        `json:"roles,omitempty"`

	Permissions map[string]*Group `json:"permissions,omitempty"`

	AdminGroups []string `json:"adminGroups,omitempty"`

	// init is true if the RBAC has been initialised.
	// RBAC has been initialised if all watchers have been started and
	// have received their initial state.
	// Essentially: have all the existing RBAC objects that existed on startup
	// been loaded?
	init     bool
	eventCh  chan hz.Event
	mx       sync.RWMutex
	watchers []*hz.Watcher
}

func (r *RBAC) Start(ctx context.Context) error {
	r.eventCh = make(chan hz.Event)
	go func() {
		for event := range r.eventCh {
			var result hz.Result
			var err error
			switch event.Key.ObjectKind() {
			case "Role":
				result, err = r.HandleRoleEvent(event)
			case "RoleBinding":
				result, err = r.HandleRoleBindingEvent(event)
			default:
				err = fmt.Errorf(
					"unexpected object kind: %v",
					event.Key.ObjectKind(),
				)
			}
			if err := event.Respond(hz.EventResult{
				Result: result,
				Err:    err,
			}); err != nil {
				slog.Error("responding to event", "err", err)
			}
		}
	}()
	//
	// Start role watcher
	//
	roleWatcher, err := hz.StartWatcher(
		ctx,
		r.Conn,
		hz.WithWatcherFor(Role{}),
		hz.WithWatcherCh(r.eventCh),
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
		hz.WithWatcherFor(RoleBinding{}),
		hz.WithWatcherCh(r.eventCh),
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
		r.init = true
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timed out waiting for watchers to initialize")
	}

	// Refresh on startup
	r.refresh()
	return nil
}

func (r *RBAC) Close() error {
	for _, w := range r.watchers {
		w.Close()
	}
	close(r.eventCh)
	return nil
}

type Group struct {
	Name       string
	Namespaces map[string]*Permissions
}

type Permissions struct {
	Allow []Verbs `json:"allow"`
	Deny  []Verbs `json:"deny"`
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
		wildcardNS, wildcardOK := group.Namespaces["*"]
		ns, nsOK := group.Namespaces[req.Object.ObjectNamespace()]
		if !wildcardOK && !nsOK {
			continue
		}

		// Merge wildcard and namespace permissions.
		if ns == nil {
			ns = &Permissions{
				Allow: []Verbs{},
				Deny:  []Verbs{},
			}
		}
		if wildcardNS != nil {
			ns.Allow = append(ns.Allow, wildcardNS.Allow...)
			ns.Deny = append(ns.Deny, wildcardNS.Deny...)
		}

		if !isAllow {
			for _, allow := range ns.Allow {
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
			for _, deny := range ns.Deny {
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
	if !checkStringPattern(vf.Group, obj.ObjectGroup()) {
		return false
	}
	if !checkStringPattern(vf.Kind, obj.ObjectKind()) {
		return false
	}
	if !checkStringPattern(vf.Name, obj.ObjectName()) {
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

	// Only refresh if rbac has been initialised.
	if r.init {
		r.refresh()
	}
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

	// Only refresh if rbac has been initialised.
	if r.init {
		r.refresh()
	}
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
					Name:       subject.Name,
					Namespaces: make(map[string]*Permissions),
				}
				cache[subject.Name] = group
			}

			// Get permissions for the namespace, or create if not exists.
			permissions, ok := group.Namespaces[roleBinding.Namespace]
			if !ok {
				permissions = &Permissions{
					Allow: []Verbs{},
					Deny:  []Verbs{},
				}
				group.Namespaces[roleBinding.Namespace] = permissions
			}

			roleKey := hz.KeyFromObject(hz.ObjectKey{
				Group:     roleBinding.Spec.RoleRef.Group,
				Version:   "v1",
				Kind:      roleBinding.Spec.RoleRef.Kind,
				Namespace: roleBinding.Namespace,
				Name:      roleBinding.Spec.RoleRef.Name,
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

			permissions.Allow = append(permissions.Allow, role.Spec.Allow...)
			permissions.Deny = append(permissions.Deny, role.Spec.Deny...)
		}
	}

	// Add implicit namespace permissions based on Group<-->Namespace relations.
	// If a group has any relation to a namespace, we should give it read
	// access to the namespace object, implicitly.
	for _, group := range cache {
		for nsName, permissions := range group.Namespaces {
			if nsName == hz.RootNamespace {
				continue
			}
			localNS := nsName

			if len(permissions.Allow) > 0 {
				rootNS, ok := group.Namespaces[hz.RootNamespace]
				if !ok {
					rootNS = &Permissions{
						Allow: []Verbs{},
						Deny:  []Verbs{},
					}
					group.Namespaces[hz.RootNamespace] = rootNS
				}

				rootNS.Allow = append(rootNS.Allow, Verbs{
					Read: &VerbFilter{
						Name: &localNS,
						Kind: hz.P(core.ObjectKindNamespace),
						// Group: hz.P("TODO"),
					},
				})
			}
		}
	}

	// Add admin group permissions (if any).
	for _, adminGroup := range r.AdminGroups {
		group, ok := cache[adminGroup]
		if !ok {
			group = &Group{
				Name:       adminGroup,
				Namespaces: make(map[string]*Permissions),
			}
			cache[adminGroup] = group
		}
		ns, ok := group.Namespaces["*"]
		if !ok {
			ns = &Permissions{
				Allow: []Verbs{},
				Deny:  []Verbs{},
			}
			group.Namespaces["*"] = ns
		}
		ns.Allow = append(ns.Allow, Verbs{
			Read:   &VerbFilter{},
			Update: &VerbFilter{},
			Create: &VerbFilter{},
			Delete: &VerbFilter{},
			Run:    &VerbFilter{},
		})
	}

	r.mx.Lock()
	defer r.mx.Unlock()
	r.Permissions = cache
}
