package hz

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"github.com/verifa/horizon/pkg/internal/hzcue"
	"github.com/verifa/horizon/pkg/internal/managedfields"
	"github.com/verifa/horizon/pkg/internal/openapiv3"
)

const NamespaceRoot = "root"

// Objecter is an interface that represents an object in the Horizon API.
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
	ObjectNamespace() string
	ObjectName() string
}

type ObjectOpenAPIV3Schemer interface {
	OpenAPIV3Schema() (*openapiv3.Schema, error)
}

func validateKeyStrict(key ObjectKeyer) error {
	var errs error
	isEmptyOrStar := func(s string) bool {
		return s == "" || s == "*"
	}
	if isEmptyOrStar(key.ObjectGroup()) {
		errs = errors.Join(errs, fmt.Errorf("group is required"))
	}
	if isEmptyOrStar(key.ObjectVersion()) {
		errs = errors.Join(errs, fmt.Errorf("version is required"))
	}
	if isEmptyOrStar(key.ObjectKind()) {
		errs = errors.Join(errs, fmt.Errorf("kind is required"))
	}
	if isEmptyOrStar(key.ObjectNamespace()) {
		errs = errors.Join(errs, fmt.Errorf("namespace is required"))
	}
	if isEmptyOrStar(key.ObjectName()) {
		errs = errors.Join(errs, fmt.Errorf("name is required"))
	}
	return errs
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
	namespace := "*"
	if obj.ObjectNamespace() != "" {
		namespace = obj.ObjectNamespace()
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
		namespace,
		name,
	)
}

// KeyFromObjectStrict takes an ObjectKeyer and returns a string key.
// It returns an error if any of the fields are empty (except APIVersion).
// This is useful when you want to ensure the key is concrete when performing
// operations on specific objects (e.g. get, create, apply).
func KeyFromObjectStrict(obj ObjectKeyer) (string, error) {
	if err := validateKeyStrict(obj); err != nil {
		return "", err
	}
	return KeyFromObject(obj), nil
}

func ObjectKeyFromString(key string) (ObjectKey, error) {
	parts := strings.Split(key, ".")
	if len(parts) != 5 {
		return ObjectKey{}, fmt.Errorf("invalid key: %q", key)
	}
	return ObjectKey{
		Group:     parts[0],
		Version:   parts[1],
		Kind:      parts[2],
		Namespace: parts[3],
		Name:      parts[4],
	}, nil
}

func ObjectKeyFromObject(object Objecter) ObjectKey {
	return ObjectKey{
		Group:     object.ObjectGroup(),
		Version:   object.ObjectVersion(),
		Kind:      object.ObjectKind(),
		Namespace: object.ObjectNamespace(),
		Name:      object.ObjectName(),
	}
}

var _ ObjectKeyer = (*ObjectKey)(nil)

type ObjectKey struct {
	Group     string
	Version   string
	Kind      string
	Namespace string
	Name      string
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

func (o ObjectKey) ObjectNamespace() string {
	if o.Namespace == "" {
		return "*"
	}
	return o.Namespace
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
		o.ObjectNamespace(),
		o.ObjectName(),
	)
}

type ObjectMeta struct {
	Name      string `json:"name,omitempty"      cue:"=~\"^[a-zA-Z0-9-_]+$\""`
	Namespace string `json:"namespace,omitempty" cue:"=~\"^[a-zA-Z0-9-_]+$\""`

	Labels map[string]string `json:"labels,omitempty" cue:",opt"`

	// Revision is the revision number of the object.
	Revision          *uint64                     `json:"revision,omitempty"          cue:",opt"`
	OwnerReferences   OwnerReferences             `json:"ownerReferences,omitempty"   cue:",opt"`
	DeletionTimestamp *Time                       `json:"deletionTimestamp,omitempty" cue:",opt"`
	ManagedFields     managedfields.ManagedFields `json:"managedFields,omitempty"     cue:",opt"`
	// Finalizers are a way for controllers to prevent garbage collection of
	// objects. The GC will not delete an object unless it has no finalizers.
	// Hence, it is the responsibility of the controller to remove the
	// finalizers once the object has been marked for deletion (by setting the
	// deletionTimestamp).
	//
	// Use type alias to "correctly" marshal to json.
	// A nil Finalizers is omitted from JSON.
	// A non-nil Finalizers is marshalled as an empty array if it is empty.
	Finalizers *Finalizers `json:"finalizers,omitempty"        cue:",opt"`
}

func (o ObjectMeta) ObjectName() string {
	return o.Name
}

func (o ObjectMeta) ObjectNamespace() string {
	return o.Namespace
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
		Group:     object.ObjectGroup(),
		Version:   object.ObjectVersion(),
		Kind:      object.ObjectKind(),
		Name:      object.ObjectName(),
		Namespace: object.ObjectNamespace(),
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
	Group     string `json:"group,omitempty"     cue:""`
	Version   string `json:"version,omitempty"   cue:""`
	Kind      string `json:"kind,omitempty"      cue:""`
	Namespace string `json:"namespace,omitempty" cue:""`
	Name      string `json:"name,omitempty"      cue:""`
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

func (o OwnerReference) ObjectNamespace() string {
	return o.Namespace
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
		o.Namespace == owner.ObjectNamespace()
}

var _ hzcue.CueExpressioner = (*Time)(nil)

type Time struct {
	time.Time
}

func (t *Time) IsPast() bool {
	if t == nil {
		return false
	}
	return t.Before(time.Now())
}

func (t Time) CueExpr(cCtx *cue.Context) (cue.Value, error) {
	return cCtx.BuildExpr(ast.NewIdent("string")), nil
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

// GenericObject represents a generic object, containing the type and object
// meta, and also a body.
// GenericObject does not care what the body is, but it stores it so that you
// can unmarshal objects as a GenericObject, perform operations on the metadata
// and marshal back to JSON with the full body.
//
// If you only want to unmarshal and get the type or object meta, use
// [MetaOnlyObject] instead.
type GenericObject struct {
	TypeMeta   `json:",inline"`
	ObjectMeta `json:"metadata,omitempty"`

	Remaining map[string]json.RawMessage `json:"-"`
}

// MarhsalJSON marshals the object to JSON.
func (g GenericObject) MarshalJSON() ([]byte, error) {
	objMap := map[string]interface{}{}
	for k, v := range g.Remaining {
		objMap[k] = v
	}

	// Marshal the object into JSON and unmarshal it into the object map.
	// This might not be the most efficient but feels safer than modifying the
	// map manually.
	type genAlias GenericObject
	obj := genAlias(g)
	b, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &objMap); err != nil {
		return nil, err
	}

	return json.Marshal(objMap)
}

func (g *GenericObject) UnmarshalJSON(data []byte) error {
	type genAlias GenericObject
	var obj genAlias
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	if err := json.Unmarshal(data, &obj.Remaining); err != nil {
		return err
	}

	*g = GenericObject(obj)
	return nil
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
