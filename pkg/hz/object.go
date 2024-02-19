package hz

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Objecter interface {
	ObjectKeyer
	ObjectRevision() *uint64
	ObjectDeletionTimestamp() *Time
	// ObjectDeleteAt(Time)
	ObjectOwnerReferences() []OwnerReference
	ObjectOwnerReference(Objecter) (OwnerReference, bool)
}

// ObjectKeyer is an interface that can produce a unique key for an object.
type ObjectKeyer interface {
	ObjectName() string
	ObjectAccount() string
	Kinder
}

type Kinder interface {
	ObjectKind() string
	// ObjectAPIVersion() string
	// ObjectGroup() string
}

var _ ObjectKeyer = (*ObjectKey[EmptyObjectWithMeta])(nil)

type ObjectKey[T Objecter] struct {
	Name    string `json:"name,omitempty"`
	Account string `json:"account,omitempty"`
}

func (o ObjectKey[T]) ObjectAccount() string {
	if o.Account == "" {
		return "*"
	}
	return o.Account
}

func (o ObjectKey[T]) ObjectKind() string {
	var t T
	return t.ObjectKind()
}

func (o ObjectKey[T]) ObjectName() string {
	if o.Name == "" {
		return "*"
	}
	return o.Name
}

func (o ObjectKey[T]) String() string {
	return o.ObjectKind() + "/" + o.Account + "/" + o.Name
}

var KeyAllObjects = Key{
	Name:    "*",
	Account: "*",
	Kind:    "*",
}

func KeyFromString(s string) (Key, error) {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Key{}, fmt.Errorf("invalid key: %q", s)
	}
	return Key{
		Kind:    parts[0],
		Account: parts[1],
		Name:    parts[2],
	}, nil
}

var _ ObjectKeyer = (*Key)(nil)

type Key struct {
	Name    string
	Account string
	Kind    string
}

func (o Key) ObjectAccount() string {
	if o.Account == "" {
		return "*"
	}
	return o.Account
}

func (o Key) ObjectKind() string {
	if o.Kind == "" {
		return "*"
	}
	return o.Kind
}

func (o Key) ObjectName() string {
	if o.Name == "" {
		return "*"
	}
	return o.Name
}

func (o Key) String() string {
	return o.Kind + "/" + o.Account + "/" + o.Name
}

type EmptyObjectWithMeta struct {
	ObjectMeta `json:"metadata"`

	Spec   struct{} `json:"spec,omitempty"`
	Status struct{} `json:"status,omitempty"`
}

func (o EmptyObjectWithMeta) ObjectKind() string {
	return "EmptyObjectWithMeta"
}

type ObjectMeta struct {
	Name    string `json:"name,omitempty" cue:"=~\"^[a-zA-Z0-9-_]+$\""`
	Account string `json:"account,omitempty" cue:"=~\"^[a-zA-Z0-9-_]+$\""`

	Labels map[string]string `json:"labels,omitempty" cue:",opt"`

	// Revision is the revision number of the object.
	Revision          *uint64          `json:"revision,omitempty" cue:"-"`
	OwnerReferences   []OwnerReference `json:"ownerReferences,omitempty" cue:"-"`
	DeletionTimestamp *Time            `json:"deletionTimestamp,omitempty" cue:"-"`
	ManagedFields     json.RawMessage  `json:"managedFields,omitempty" cue:"-"`
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

func OwnerReferenceFromObject(object Objecter) *OwnerReference {
	return &OwnerReference{
		Kind:    object.ObjectKind(),
		Name:    object.ObjectName(),
		Account: object.ObjectAccount(),
	}
}

type OwnerReference struct {
	Kind    string
	Name    string
	Account string
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

type Time struct {
	time.Time
}

var _ Objecter = (*GenericObject)(nil)

type GenericObject struct {
	ObjectMeta `json:"metadata,omitempty"`

	Kind string          `json:"kind,omitempty"`
	Spec json.RawMessage `json:"spec,omitempty"`
}

func (r GenericObject) ObjectKind() string {
	return r.Kind
}

var _ Objecter = (*MetaOnlyObject)(nil)

// MetaOnlyObject is an object that has no spec or status.
// It is used for unmarshalling objects from the store to read metadata.
type MetaOnlyObject struct {
	ObjectMeta `json:"metadata,omitempty"`
	Kind       string `json:"kind,omitempty"`
}

func (r MetaOnlyObject) ObjectKind() string {
	return r.Kind
}
