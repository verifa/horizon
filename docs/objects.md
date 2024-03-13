# Objects

Horizon object definitions are how you define your platform API in Horizon.

Typically they have a `spec` field which should be user-supplied, and a `status` field which a controller or actor will fill.

Objects serve two main purposes:

1. Objects can be stored in the NATS KV store, and a controller will enter its reconcile loop to handle the object. This is stateful and asynchronous.
2. Objects can be sent to an actor to complete some action. This is stateless and synchronous.

For the most part, you will use the first option and write controllers and persist objects.
Actors fill a more specific use case (such as running actions on specific nodes).

## Object keys

Objects in Horizon are indexed in the NATS KV with a *key* (a NATS subject relative to the NATS KV).
The key includes the following fields:

1. **Object Group:** groups are a logical way to organise resources together for things like searching and RBAC.
2. **Object Version:** the object version is a way to version the API. It helps maintain things like backwards compatability.
3. **Object Kind:** is just a name for the kind of object.
4. **Object Account:** is the account that this object belongs to.
5. **Object Name:** is the unique identifier for this object within the account.

An example key looks like: `group.v1.Object.account.name`.

## Defining an object

There are two important interfaces in the `hz` package:

```go
// ObjectKeyer is an interface that can produce a unique key for an object.
type ObjectKeyer interface {
    ObjectGroup() string
    ObjectVersion() string
    ObjectKind() string
    ObjectAccount() string
    ObjectName() string
}

// Objecter is an interface that represents an object in the Horizon API.
type Objecter interface {
    ObjectKeyer
    ObjectRevision() *uint64
    ObjectDeletionTimestamp() *Time
    ObjectOwnerReferences() []OwnerReference
    ObjectOwnerReference(Objecter) (OwnerReference, bool)
    ObjectManagedFields() managedfields.ManagedFields
}
```

They serve two different use cases. If you want to act on an object in Horizon, you need only something that implements `hz.ObjectKeyer`
To define an object we need a struct that implements `hz.Objecter`.
Any struct that implements `hz.Objecter` will also implement `hz.ObjectKeyer`.

## Next steps

Read about [server side apply](./serversideapply.md) and how objects are managed by multiple entities (such as end users and controllers).
