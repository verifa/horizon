package hz

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/verifa/horizon/pkg/internal/managedfields"
)

type Objecter interface {
	ObjectKeyer
	ObjectRevision() *uint64
	ObjectDeletionTimestamp() *Time
	ObjectOwnerReferences() []OwnerReference
	ObjectOwnerReference(Objecter) (OwnerReference, bool)
	ObjectManagedFields() managedfields.ManagedFields
	ObjectVersion() string
}

// ObjectKeyer is an interface that can produce a unique key for an object.
type ObjectKeyer interface {
	ObjectName() string
	ObjectAccount() string
	ObjectKind() string
	ObjectGroup() string
}

// KeyFromObject takes an ObjectKeyer and returns a string key.
// Any empty fields in the ObjectKeyer are replaced with "*" which works well
// for nats subjects to list objects.
//
// If performing an action on a specific object (e.g. get, create, apply) the
// key cannot contain "*".
// In this case you can use [KeyFromObjectConcrete] which makes sure the
// ObjectKeyer is concrete.
func KeyFromObject(obj ObjectKeyer) string {
	account := "*"
	if obj.ObjectAccount() != "" {
		account = obj.ObjectAccount()
	}
	name := "*"
	if obj.ObjectName() != "" {
		name = obj.ObjectName()
	}
	group := "*"
	if obj.ObjectGroup() != "" {
		group = obj.ObjectGroup()
	}
	kind := "*"
	if obj.ObjectKind() != "" {
		kind = obj.ObjectKind()
	}
	return fmt.Sprintf(
		"%s.%s.%s.%s",
		group,
		kind,
		account,
		name,
	)
}

// KeyFromObjectConcrete takes an ObjectKeyer and returns a string key.
// It returns an error if any of the fields are empty.
// This is useful when you want to ensure the key is concrete when performing
// operations on specific objects (e.g. get, create, apply).
func KeyFromObjectConcrete(obj ObjectKeyer) (string, error) {
	var errs error
	if obj.ObjectAccount() == "" {
		errs = errors.Join(errs, fmt.Errorf("account is required"))
	}
	if obj.ObjectName() == "" {
		errs = errors.Join(errs, fmt.Errorf("name is required"))
	}
	if obj.ObjectKind() == "" {
		errs = errors.Join(errs, fmt.Errorf("kind is required"))
	}
	if obj.ObjectGroup() == "" {
		errs = errors.Join(errs, fmt.Errorf("group is required"))
	}
	if errs != nil {
		return "", errs
	}
	return KeyFromObject(obj), nil
}

func objectKeyFromKey(key string) (ObjectKey, error) {
	parts := strings.Split(key, ".")
	if len(parts) != 4 {
		return ObjectKey{}, fmt.Errorf("invalid key: %q", key)
	}
	return ObjectKey{
		Group:   parts[0],
		Kind:    parts[1],
		Account: parts[2],
		Name:    parts[3],
	}, nil
}

func ObjectKeyFromObject(object Objecter) ObjectKey {
	return ObjectKey{
		Name:    object.ObjectName(),
		Account: object.ObjectAccount(),
		Kind:    object.ObjectKind(),
		Group:   object.ObjectGroup(),
	}
}

var _ ObjectKeyer = (*ObjectKey)(nil)

type ObjectKey struct {
	Name    string
	Account string
	Kind    string
	Group   string
}

func (o ObjectKey) ObjectAccount() string {
	if o.Account == "" {
		return "*"
	}
	return o.Account
}

func (o ObjectKey) ObjectName() string {
	if o.Name == "" {
		return "*"
	}
	return o.Name
}

func (o ObjectKey) ObjectKind() string {
	if o.Kind == "" {
		return "*"
	}
	return o.Kind
}

func (o ObjectKey) ObjectGroup() string {
	if o.Group == "" {
		return "*"
	}
	return o.Group
}

func (o ObjectKey) String() string {
	return o.ObjectGroup() + "." + o.ObjectKind() + "." + o.ObjectAccount() + "." + o.ObjectName()
}

var _ Objecter = (*EmptyObjectWithMeta)(nil)

type EmptyObjectWithMeta struct {
	TypeMeta   `json:",inline"`
	ObjectMeta `json:"metadata"`
}

type ObjectMeta struct {
	Name    string `json:"name,omitempty" cue:"=~\"^[a-zA-Z0-9-_]+$\""`
	Account string `json:"account,omitempty" cue:"=~\"^[a-zA-Z0-9-_]+$\""`

	Labels map[string]string `json:"labels,omitempty" cue:",opt"`

	// Revision is the revision number of the object.
	Revision          *uint64                     `json:"revision,omitempty" cue:"-"`
	OwnerReferences   []OwnerReference            `json:"ownerReferences,omitempty" cue:",opt"`
	DeletionTimestamp *Time                       `json:"deletionTimestamp,omitempty" cue:"-"`
	ManagedFields     managedfields.ManagedFields `json:"managedFields,omitempty" cue:"-"`
	// Finalizers are a way for controllers to prevent garbage collection of
	// objects. The GC will not delete an object unless it has no finalizers.
	// Hence, it is the responsibility of the controller to remove the
	// finalizers once the object has been marked for deletion (by setting the
	// deletionTimestamp).
	Finalizers []Finalizer `json:"finalizers,omitempty" cue:",opt"`
}

func (o ObjectMeta) ObjectName() string {
	return o.Name
}

func (o ObjectMeta) ObjectAccount() string {
	return o.Account
}

func (o ObjectMeta) ObjectRevision() *uint64 {
	return o.Revision
}

func (o ObjectMeta) ObjectDeletionTimestamp() *Time {
	return o.DeletionTimestamp
}

// ObjectDeleteNow returns true if the object has a deletion timestamp that
// has expired, and the controller should therefore delete the object.
func (o ObjectMeta) ObjectDeleteNow() bool {
	return o.DeletionTimestamp != nil && o.DeletionTimestamp.Before(time.Now())
}

func (o ObjectMeta) ObjectOwnerReferences() []OwnerReference {
	return o.OwnerReferences
}

func (o ObjectMeta) ObjectOwnerReference(
	owner Objecter,
) (OwnerReference, bool) {
	if o.OwnerReferences == nil {
		return OwnerReference{}, false
	}
	for _, ow := range o.OwnerReferences {
		if ow.IsOwnedBy(owner) {
			return ow, true
		}
	}
	return OwnerReference{}, false
}

func (o ObjectMeta) ObjectManagedFields() managedfields.ManagedFields {
	return o.ManagedFields
}

type TypeMeta struct {
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind,omitempty"`
}

func (t TypeMeta) ObjectKind() string {
	return t.Kind
}

func (t TypeMeta) ObjectGroup() string {
	parts := strings.Split(t.APIVersion, "/")
	if len(parts) != 2 {
		return ""
	}
	return parts[0]
}

func (t TypeMeta) ObjectVersion() string {
	parts := strings.Split(t.APIVersion, "/")
	if len(parts) != 2 {
		return ""
	}
	return parts[1]
}

func OwnerReferenceFromObject(object Objecter) *OwnerReference {
	return &OwnerReference{
		Group:   object.ObjectGroup(),
		Kind:    object.ObjectKind(),
		Name:    object.ObjectName(),
		Account: object.ObjectAccount(),
	}
}

type OwnerReference struct {
	Group   string
	Kind    string
	Name    string
	Account string
}

func (o OwnerReference) ObjectGroup() string {
	return o.Group
}

func (o OwnerReference) ObjectKind() string {
	return o.Kind
}

func (o OwnerReference) ObjectAccount() string {
	return o.Account
}

func (o OwnerReference) ObjectName() string {
	return o.Name
}

func (o OwnerReference) IsOwnedBy(owner Objecter) bool {
	if owner == nil {
		return false
	}
	return o.Kind == owner.ObjectKind() &&
		o.Name == owner.ObjectName() &&
		o.Account == owner.ObjectAccount()
}

type Finalizer struct {
	ID string `json:"id,omitempty" cue:"=~\"^[a-zA-Z0-9-_]+$\""`
}

type Time struct {
	time.Time
}

var _ Objecter = (*GenericObject)(nil)

type GenericObject struct {
	TypeMeta   `json:",inline"`
	ObjectMeta `json:"metadata,omitempty"`

	Spec   json.RawMessage `json:"spec,omitempty"`
	Status json.RawMessage `json:"status,omitempty"`
}

type GenericObjectList struct {
	Items []GenericObject `json:"items,omitempty"`
}

type ObjectList struct {
	Items []json.RawMessage `json:"items,omitempty"`
}

type TypedObjectList[T Objecter] struct {
	Items []*T `json:"items,omitempty"`
}

var _ Objecter = (*MetaOnlyObject)(nil)

// MetaOnlyObject is an object that has no spec or status.
// It is used for unmarshalling objects from the store to read metadata.
type MetaOnlyObject struct {
	ObjectMeta `json:"metadata,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Group      string `json:"group,omitempty"`
	APIVersion string `json:"apiVersion,omitempty"`
}

func (r MetaOnlyObject) ObjectKind() string {
	return r.Kind
}

func (r MetaOnlyObject) ObjectGroup() string {
	return r.Group
}

func (r MetaOnlyObject) ObjectVersion() string {
	return r.APIVersion
}
