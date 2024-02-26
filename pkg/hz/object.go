package hz

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/verifa/horizon/pkg/managedfields"
)

type Objecter interface {
	ObjectKeyer
	ObjectRevision() *uint64
	ObjectDeletionTimestamp() *Time
	ObjectOwnerReferences() []OwnerReference
	ObjectOwnerReference(Objecter) (OwnerReference, bool)
	ObjectManagedFields() managedfields.ManagedFields
	ObjectAPIVersion() string
}

// ObjectKeyer is an interface that can produce a unique key for an object.
type ObjectKeyer interface {
	ObjectName() string
	ObjectAccount() string
	ObjectKind() string
	ObjectGroup() string
}

func KeyFromObject(obj ObjectKeyer) string {
	account := "*"
	if obj.ObjectAccount() != "" {
		account = obj.ObjectAccount()
	}
	name := "*"
	if obj.ObjectName() != "" {
		name = obj.ObjectName()
	}
	return fmt.Sprintf(
		"%s.%s.%s.%s",
		obj.ObjectGroup(),
		obj.ObjectKind(),
		account,
		name,
	)
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

type EmptyObjectWithMeta struct {
	ObjectMeta `json:"metadata"`

	Spec   struct{} `json:"spec,omitempty"`
	Status struct{} `json:"status,omitempty"`
}

func (o EmptyObjectWithMeta) ObjectKind() string {
	return "EmptyObjectWithMeta"
}

func (o EmptyObjectWithMeta) ObjectGroup() string {
	return "hz-internal"
}

type ObjectMeta struct {
	Name    string `json:"name,omitempty" cue:"=~\"^[a-zA-Z0-9-_]+$\""`
	Account string `json:"account,omitempty" cue:"=~\"^[a-zA-Z0-9-_]+$\""`

	Labels map[string]string `json:"labels,omitempty" cue:",opt"`

	// Revision is the revision number of the object.
	Revision          *uint64                     `json:"revision,omitempty" cue:"-"`
	OwnerReferences   []OwnerReference            `json:"ownerReferences,omitempty" cue:"-"`
	DeletionTimestamp *Time                       `json:"deletionTimestamp,omitempty" cue:"-"`
	ManagedFields     managedfields.ManagedFields `json:"managedFields,omitempty" cue:"-"`
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

type Time struct {
	time.Time
}

var _ Objecter = (*GenericObject)(nil)

type GenericObject struct {
	ObjectMeta `json:"metadata,omitempty"`

	Kind       string          `json:"kind,omitempty"`
	Group      string          `json:"group,omitempty"`
	APIVersion string          `json:"apiVersion,omitempty"`
	Spec       json.RawMessage `json:"spec,omitempty"`
	Status     json.RawMessage `json:"status,omitempty"`
}

func (r GenericObject) ObjectKind() string {
	return r.Kind
}

func (r GenericObject) ObjectGroup() string {
	return r.Group
}

func (r GenericObject) ObjectAPIVersion() string {
	return r.APIVersion
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

func (r MetaOnlyObject) ObjectAPIVersion() string {
	return r.ObjectAPIVersion()
}
