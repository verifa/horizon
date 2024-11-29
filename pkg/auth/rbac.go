package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
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
	RoleBindings map[string]RoleBinding
	Roles        map[string]Role

	Permissions map[string]*Group

	AdminGroup string

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
				result, err = r.handleRoleEvent(event)
			case "RoleBinding":
				result, err = r.handleRoleBindingEvent(event)
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
	Allow []Rule `json:"allow"`
	Deny  []Rule `json:"deny"`
}

func (p *Permissions) AllowRules() []Rule {
	if p == nil {
		return nil
	}
	return p.Allow
}

func (p *Permissions) DenyRules() []Rule {
	if p == nil {
		return nil
	}
	return p.Deny
}

type Verb string

const (
	// VerbRead allows/denies a subject to read objects.
	VerbRead Verb = "read"
	// VerbUpdate allows/denies a subject to update objects.
	VerbUpdate Verb = "update"
	// VerbCreate allows/denies a subject to create objects.
	VerbCreate Verb = "create"
	// VerbDelete allows/denies a subject to delete objects.
	VerbDelete Verb = "delete"
	// VerbRun allows/denies a subject to run actions for an actor.
	VerbRun Verb = "run"
	// VerbAll allows/denies a subject to perform all verbs.
	VerbAll Verb = "*"
)

// Request is a request to check if Subject is allowed to perform Verb on
// Object.
type Request struct {
	Subject RequestSubject
	Verb    Verb
	Object  hz.ObjectKeyer
}

type RequestSubject struct {
	Groups []string
}

func (r *RBAC) Check(ctx context.Context, req Request) bool {
	// Members of the admin group is allowed to do anything.
	if slices.Contains(req.Subject.Groups, r.AdminGroup) {
		return true
	}
	r.mx.RLock()
	defer r.mx.RUnlock()

	isAllow := false
	isDeny := false
	for _, gr := range req.Subject.Groups {
		group, ok := r.Permissions[gr]
		if !ok {
			continue
		}
		rootNS, wildcardOK := group.Namespaces[hz.NamespaceRoot]
		ns, nsOK := group.Namespaces[req.Object.ObjectNamespace()]
		// Exit early if there are no permissions for either.
		if !(wildcardOK || nsOK) {
			continue
		}

		lenAllow := len(rootNS.AllowRules()) + len(ns.AllowRules())
		lenDeny := len(rootNS.DenyRules()) + len(ns.DenyRules())

		// Merge wildcard and namespace permissions.
		perm := &Permissions{
			Allow: make([]Rule, 0, lenAllow),
			Deny:  make([]Rule, 0, lenDeny),
		}
		perm.Allow = append(rootNS.AllowRules(), ns.AllowRules()...)
		perm.Deny = append(rootNS.DenyRules(), ns.DenyRules()...)

		if !isAllow {
			for _, allow := range perm.Allow {
				isAllow = checkVerb(allow, req.Verb, req.Object)
				if isAllow {
					break
				}
			}
		}
		if !isDeny {
			for _, deny := range perm.Deny {
				isDeny = checkVerb(deny, req.Verb, req.Object)
				if isDeny {
					break
				}
			}
		}
	}
	return isAllow && !isDeny
}

func checkVerb(rule Rule, verb Verb, obj hz.ObjectKeyer) bool {
	// If the rule does not specify the wildcard ("*") verb, check if the verb
	// is allowed.
	if !slices.Contains(rule.Verbs, VerbAll) {
		if !slices.Contains(rule.Verbs, verb) {
			return false
		}
	}
	if !checkStringPattern(rule.Group, obj.ObjectGroup()) {
		return false
	}
	if !checkStringPattern(rule.Kind, obj.ObjectKind()) {
		return false
	}
	if !checkStringPattern(rule.Name, obj.ObjectName()) {
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

func (r *RBAC) handleRoleBindingEvent(event hz.Event) (hz.Result, error) {
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

func (r *RBAC) handleRoleEvent(event hz.Event) (hz.Result, error) {
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
					Allow: []Rule{},
					Deny:  []Rule{},
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
			if nsName == hz.NamespaceRoot {
				continue
			}
			localNS := nsName

			if len(permissions.Allow) > 0 {
				rootNS, ok := group.Namespaces[hz.NamespaceRoot]
				if !ok {
					rootNS = &Permissions{
						Allow: []Rule{},
						Deny:  []Rule{},
					}
					group.Namespaces[hz.NamespaceRoot] = rootNS
				}

				rootNS.Allow = append(rootNS.Allow, Rule{
					Name:  &localNS,
					Kind:  hz.P(core.ObjectKindNamespace),
					Verbs: []Verb{VerbRead},
				})
			}
		}
	}

	r.mx.Lock()
	defer r.mx.Unlock()
	r.Permissions = cache
}
