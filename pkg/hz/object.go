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
}

// ObjectKeyer is an interface that can produce a unique key for an object.
type ObjectKeyer interface {
	ObjectGroup() string
	ObjectVersion() string
	ObjectKind() string
	ObjectAccount() string
	ObjectName() string
}

// KeyFromObject takes an ObjectKeyer and returns a string key.
// Any empty fields in the ObjectKeyer are replaced with "*" which works well
// for nats subjects to list objects.
//
// If performing an action on a specific object (e.g. get, create, apply) the
// key cannot contain "*".
// In this case you can use [KeyFromObjectStrict] which makes sure the
// ObjectKeyer is concrete.
func KeyFromObject(obj ObjectKeyer) string {
	group := "*"
	if obj.ObjectGroup() != "" {
		group = obj.ObjectGroup()
	}
	version := "*"
	if obj.ObjectVersion() != "" {
		version = obj.ObjectVersion()
	}
	kind := "*"
	if obj.ObjectKind() != "" {
		kind = obj.ObjectKind()
	}
	account := "*"
	if obj.ObjectAccount() != "" {
		account = obj.ObjectAccount()
	}
	name := "*"
	if obj.ObjectName() != "" {
		name = obj.ObjectName()
	}
	return fmt.Sprintf(
		"%s.%s.%s.%s.%s",
		group,
		version,
		kind,
		account,
		name,
	)
}

// KeyFromObjectStrict takes an ObjectKeyer and returns a string key.
// It returns an error if any of the fields are empty (except APIVersion).
// This is useful when you want to ensure the key is concrete when performing
// operations on specific objects (e.g. get, create, apply).
func KeyFromObjectStrict(obj ObjectKeyer) (string, error) {
	var errs error
	isEmptyOrStar := func(s string) bool {
		return s == "" || s == "*"
	}
	if isEmptyOrStar(obj.ObjectGroup()) {
		errs = errors.Join(errs, fmt.Errorf("group is required"))
	}
	if isEmptyOrStar(obj.ObjectVersion()) {
		errs = errors.Join(errs, fmt.Errorf("version is required"))
	}
	if isEmptyOrStar(obj.ObjectKind()) {
		errs = errors.Join(errs, fmt.Errorf("kind is required"))
	}
	if isEmptyOrStar(obj.ObjectAccount()) {
		errs = errors.Join(errs, fmt.Errorf("account is required"))
	}
	if isEmptyOrStar(obj.ObjectName()) {
		errs = errors.Join(errs, fmt.Errorf("name is required"))
	}
	if errs != nil {
		return "", errs
	}
	return KeyFromObject(obj), nil
}

func ObjectKeyFromString(key string) (ObjectKey, error) {
	parts := strings.Split(key, ".")
	if len(parts) != 5 {
		return ObjectKey{}, fmt.Errorf("invalid key: %q", key)
	}
	return ObjectKey{
		Group:   parts[0],
		Version: parts[1],
		Kind:    parts[2],
		Account: parts[3],
		Name:    parts[4],
	}, nil
}

func ObjectKeyFromObject(object Objecter) ObjectKey {
	return ObjectKey{
		Group:   object.ObjectGroup(),
		Version: object.ObjectVersion(),
		Kind:    object.ObjectKind(),
		Account: object.ObjectAccount(),
		Name:    object.ObjectName(),
	}
}

var _ ObjectKeyer = (*ObjectKey)(nil)

type ObjectKey struct {
	Group   string
	Version string
	Kind    string
	Account string
	Name    string
}

func (o ObjectKey) ObjectGroup() string {
	if o.Group == "" {
		return "*"
	}
	return o.Group
}

func (o ObjectKey) ObjectVersion() string {
	if o.Version == "" {
		return "*"
	}
	return o.Version
}

func (o ObjectKey) ObjectKind() string {
	if o.Kind == "" {
		return "*"
	}
	return o.Kind
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

func (o ObjectKey) String() string {
	return fmt.Sprintf(
		"%s.%s.%s.%s.%s",
		o.ObjectGroup(),
		o.ObjectVersion(),
		o.ObjectKind(),
		o.ObjectAccount(),
		o.ObjectName(),
	)
}

type ObjectMeta struct {
	Name    string `json:"name,omitempty" cue:"=~\"^[a-zA-Z0-9-_]+$\""`
	Account string `json:"account,omitempty" cue:"=~\"^[a-zA-Z0-9-_]+$\""`

	Labels map[string]string `json:"labels,omitempty" cue:",opt"`

	// Revision is the revision number of the object.
	Revision          *uint64                     `json:"revision,omitempty" cue:",opt"`
	OwnerReferences   OwnerReferences             `json:"ownerReferences,omitempty" cue:",opt"`
	DeletionTimestamp *Time                       `json:"deletionTimestamp,omitempty" cue:",opt"`
	ManagedFields     managedfields.ManagedFields `json:"managedFields,omitempty" cue:",opt"`
	// Finalizers are a way for controllers to prevent garbage collection of
	// objects. The GC will not delete an object unless it has no finalizers.
	// Hence, it is the responsibility of the controller to remove the
	// finalizers once the object has been marked for deletion (by setting the
	// deletionTimestamp).
	//
	// Use type alias to "correctly" marshal to json.
	// A nil Finalizers is omitted from JSON.
	// A non-nil Finalizers is marshalled as an empty array if it is empty.
	Finalizers *Finalizers `json:"finalizers,omitempty" cue:",opt"`
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

func OwnerReferenceFromObject(object Objecter) OwnerReference {
	return OwnerReference{
		Group:   object.ObjectGroup(),
		Version: object.ObjectVersion(),
		Kind:    object.ObjectKind(),
		Name:    object.ObjectName(),
		Account: object.ObjectAccount(),
	}
}

type OwnerReferences []OwnerReference

func (o OwnerReferences) IsOwnedBy(obj Objecter) bool {
	for _, owner := range o {
		if owner.IsOwnedBy(obj) {
			return true
		}
	}
	return false
}

var _ ObjectKeyer = (*OwnerReference)(nil)

type OwnerReference struct {
	Group   string `json:"group,omitempty" cue:""`
	Version string `json:"version,omitempty" cue:""`
	Kind    string `json:"kind,omitempty" cue:""`
	Account string `json:"account,omitempty" cue:""`
	Name    string `json:"name,omitempty" cue:""`
}

func (o OwnerReference) ObjectGroup() string {
	return o.Group
}

func (o OwnerReference) ObjectVersion() string {
	return o.Version
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
	return o.Group == owner.ObjectGroup() &&
		o.Version == owner.ObjectVersion() &&
		o.Kind == owner.ObjectKind() &&
		o.Name == owner.ObjectName() &&
		o.Account == owner.ObjectAccount()
}

type Time struct {
	time.Time
}

func (t *Time) IsPast() bool {
	if t == nil {
		return false
	}
	return t.Before(time.Now())
}

// Finalizers are a way to prevent garbage collection of objects until a
// controller has finished some cleanup logic.
type Finalizers []string

func (f *Finalizers) Contains(finalizer string) bool {
	if f == nil {
		return false
	}
	for _, s := range *f {
		if s == finalizer {
			return true
		}
	}
	return false
}

func (f *Finalizers) String() string {
	if f == nil {
		return ""
	}
	return strings.Join(*f, ",")
}

var _ Objecter = (*MetaOnlyObject)(nil)

// MetaOnlyObject is an object that has no spec or status.
// It is used for unmarshalling objects from the store to read metadata.
type MetaOnlyObject struct {
	TypeMeta   `json:",inline"`
	ObjectMeta `json:"metadata"`
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
	Items []T `json:"items,omitempty"`
}
