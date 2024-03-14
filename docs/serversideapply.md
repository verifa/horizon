# Server Side Apply

Server side apply is a very important concept to understand when working with Horizon.

To help understand its significance we can look at a simple use case:

1. An end user creates (or applies) an object
2. A controller reconcile loop is triggered and it needs to add something to the object `.status`

In this common use case we have two entities writing to the same object.
How do we make sure they are not both writing to the same field? And how do we remove fields that an entity no longer wants to update?

The answer: a server-side patching strategy that keeps track of which entity manages which fields.

> [!NOTE]
> Server-side apply in Horizon works very similarly to Kubernetes.
>
> Documentation: <https://kubernetes.io/docs/reference/using-api/server-side-apply/>

## Manager and Managed Fields

When an object is applied, the `store` (server-side) requires a "manager" (string name) and will calculate the fields that this manager manages.

The computed managed fields are stored along with the object in the `.metadata.managedFields` field.

On a subsequent apply, the `store` will calculate the managed fields for the apply operation, fetch the existing object, and merge the managed fields.

The merge operation does a number of things:

1. Calculates conflicts in case different managers are trying to manage the same field. If there are conflicts, the operation is aborted. You can force the operation and the new manager will take owernership.
2. Calculate any removed fields. If a manager owns fields that are not present in a subsequent apply from the same manager, those fields are removed from the object.

> [!IMPORTANT]
> It is very important that when you apply an object you include only fields you want to manage.
>
> This applies for both end users and controllers. This affects how you model your objects because you want a clear separation of concerns and is why the `.spec` field is typically for users and the `.status` field for controllers.

## Extracing Managed Fields

When a reconciler enters its reconcile loop, the first step will typically be to get the object from the store.
The returned object will include the entire object (all the fields, including those not managed by the reconciler).

The reconciler will want to modify some fields and apply the object back to the store, such as updating the `.status` field.
The object which the reconcilier applies should only include the fields which the reconciler should manage.

So, how do we "extract" the managed fields from object, so that we can mutate it and apply it back afterwards? Using the `hz.ExtractManagedFields(...)` function.

A typical reconcile loop will look like this:

```go
func (r *GreetingReconciler) Reconcile(
    ctx context.Context,
    req hz.Request,
) (hz.Result, error) {
    // Get the entire object from the store, including fields
    // that the reconciler does not manage.
    greeting, err := r.GreetingClient.Get(ctx, hz.WithGetKey(req.Key))
    if err != nil {
        return hz.Result{}, hz.IgnoreNotFound(err)
    }
    // Extract the fields that the reconciler manages.
    // We can now mutate this object and apply it.
    applyGreet, err := hz.ExtractManagedFields(
        greeting,
        r.GreetingClient.Client.Manager,
    )
    if err != nil {
        return hz.Result{}, fmt.Errorf("extracting managed fields: %w", err)
    }
    if greeting.DeletionTimestamp.IsPast() {
        // TODO: Handle any cleanup logic here.
        return hz.Result{}, nil
    }

    // TODO: handle any reconcile logic here.

    // Mutate the object status in memory.
    applyGreet.Status = &GreetingStatus{
        Ready: false,
    }
    // Apply the object, triggering a server-side apply.
    // Note that if the object does not change after the server-side apply
    // merge, then this is a no-op, and will not trigger a subsequent
    // reconcile loop.
    if err := r.GreetingClient.Apply(ctx, applyGreet); err != nil {
        return hz.Result{}, fmt.Errorf("updating greeting: %w", err)
    })
    return hz.Result{}, nil
}
```
