package hz

import (
	"encoding/json"
	"time"
)

type Objecter interface {
	ObjectKeyer
	ObjectRevision() *uint64
	ObjectDeletionTimestamp() *Time
	// ObjectDeleteAt(Time)
	ObjectOwnerReference() *OwnerReference
	ObjectIsOwnedBy(Objecter) bool
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

type EmptyObjectWithMeta struct {
	ObjectMeta `json:"metadata"`
}

func (o EmptyObjectWithMeta) ObjectKind() string {
	return "EmptyObjectWithMeta"
}

type ObjectMeta struct {
	Name    string `json:"name,omitempty" cue:"=~\"^[a-zA-Z0-9-_]+$\""`
	Account string `json:"account,omitempty" cue:"=~\"^[a-zA-Z0-9-_]+$\""`

	Labels map[string]string `json:"labels,omitempty" cue:",opt"`

	// Revision is the revision number of the object.
	Revision          *uint64         `json:"revision,omitempty" cue:"-"`
	OwnerReference    *OwnerReference `json:"ownerReference,omitempty" cue:"-"`
	DeletionTimestamp *Time           `json:"deletionTimestamp,omitempty" cue:"-"`
	ManagedFields     json.RawMessage `json:"managedFields,omitempty" cue:"-"`
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

func (o ObjectMeta) ObjectOwnerReference() *OwnerReference {
	return o.OwnerReference
}

func (o ObjectMeta) ObjectIsOwnedBy(owner Objecter) bool {
	if o.OwnerReference == nil {
		return false
	}
	return o.OwnerReference.IsOwnedBy(owner.ObjectOwnerReference())
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

func (o OwnerReference) IsOwnedBy(owner *OwnerReference) bool {
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
