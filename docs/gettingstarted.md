# Getting Started

This getting started guide is intended for "platform teams" looking to develop their internal developer platform on top of Horizon.

We will go over the basics to:

1. Run a local dev Horizon server
2. Develop an extension that includes
    - An API object
    - A controller
    - A web portal
    - An actor

If you do not know what these things are, please refer to the [architecture](./architecture.md).

## 1. Running a local dev Horizon server

Clone this repository, run the code generation (templ) and run the horizon server, e.g.:

```console
git clone git@github.com:verifa/horizon.git
cd horizon

# Run code generation.
go run ./cmd/ci/ci.go -generate

# Run the horizon server.
go run ./cmd/horizon/horizon.go
```

If all works, you should see the following output:

```console
 _                _
| |__   ___  _ __(_)_______  _ __
| '_ \ / _ \| '__| |_  / _ \| '_ \
| | | | (_) | |  | |/ / (_) | | | |
|_| |_|\___/|_|  |_/___\___/|_| |_|
    _                         _
 __| |_____ __  _ __  ___  __| |___
/ _` / -_) V / | '  \/ _ \/ _` / -_)
\__,_\___|\_/  |_|_|_\___/\__,_\___|

Below is a NATS credential for the root namespace.
Copy it to a file such as nats.creds


-----BEGIN NATS USER JWT-----
...
------END NATS USER JWT------

************************* IMPORTANT *************************
NKEY Seed printed below can be used to sign and prove identity.
NKEYs are sensitive and should be treated as secrets.

-----BEGIN USER NKEY SEED-----
...
------END USER NKEY SEED------

*************************************************************
```

Copy this NATS credential and put it into a file, such as `nats.creds` as we will need this when developing our Horizon extension in the next part.

Open your browser at <http://localhost:9999> and you should be redirected to login.

Login with username `admin` and password `admin`.
Horizon comes with an embedded OIDC provider based on [zitadel/oidc](https://github.com/zitadel/oidc/tree/main/example/server) and is pre-populated with these credentials.

## 2. Developing a Horizon extension

There is no strict definition of a Horizon extension.
It could be just a web portal or an object definition, or both and a controller and actor.

For the example, we will rewrite the greetings example that can be found under the [examples](../examples/greetings/).

You could fork the horizon project and make your own example, or create a separate Go module and create an extension there.

Let's start by defining our object.

### 2.1 Defining an object

Objects in Horizon are indexed in the NATS KV with a *key* (a NATS subject relative to the NATS KV).
The key includes the following fields:

1. **Object Group:** groups are a logical way to organise resources together for things like searching and RBAC.
2. **Object Version:** the object version is a way to version the API. It helps maintain things like backwards compatability.
3. **Object Kind:** is just a name for the kind of object.
4. **Object Namespace:** is the namespace that this object belongs to.
5. **Object Name:** is the unique identifier for this object within the namespace.

An example key looks like: `group.v1.Object.namespace.name`.

There are two important interfaces in the `hz` package:

```go
// ObjectKeyer is an interface that can produce a unique key for an object.
type ObjectKeyer interface {
    ObjectGroup() string
    ObjectVersion() string
    ObjectKind() string
    ObjectNamespace() string
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

They serve two different use cases. If you want to act on an object in Horizon, you need only something that implements `hz.ObjectKeyer` (like `hz.ObjectKey`).
To define an object we need a struct that implements `hz.Objecter`.
Any struct that implements `hz.Objecter` will also implement `hz.ObjectKeyer`.

Let's say we define an object `Greeting` as follows:

```go
// Compiler check that Greeting implements hz.Objecter.
var _ hz.Objecter = (*Greeting)(nil)

type Greeting struct {
    hz.ObjectMeta `json:"metadata" cue:""`

    // Add custom fields here.
    // Convention is to use a spec and status field.
    Spec   *GreetingSpec   `json:"spec,omitempty" cue:""`
    Status *GreetingStatus `json:"status,omitempty"`
}

func (s Greeting) ObjectGroup() string {
    return "greetings"
}

func (s Greeting) ObjectVersion() string {
    return "v1"
}

func (s Greeting) ObjectKind() string {
    return "Greeting"
}

// GreetingSpec defines the desired state of Greeting.
type GreetingSpec struct {
    // Name of the person to greet.
    Name string `json:"name,omitempty" cue:""`
}

// GreetingStatus defines the observed state of Greeting.
type GreetingStatus struct {
    // Ready indicates whether the greeting is ready.
    Ready bool `json:"ready"`
    // Error is the error message if the greeting failed.
    Error string `json:"error,omitempty" cue:",opt"`
    // Response is the response of the greeting.
    Response string `json:"response,omitempty" cue:",opt"`
}
```

That's it. Now we have an object, but for someone to be able to create an object in the NATS KV store we need to start a controller which handles validation and reconciliation of all greetings.

### 2.2 Creating a controller

Whenever a greeting is created we want our controller for the greeting object to reconcile and add a response to the `.status.response` field.

This controller does not do very much but its intention is to teach you the basics of Horizon.
For a real example, we might use a cloud SDK or call Terraform from our Go code to provision some resources.

To start a controller we will call the `hz.StartController(...)` function.
A controller does actually not need a reconciler, nor a validator.
A controller does need an object though, and from that object a NATS key will be calculated which determines which objects will be reconciled.
For example, if we do the following we will start a controller that will reconcile objects with the key `greetings.v1.Greeting.*.*`, where `*` is a wildcard to match any string.

```go
ctlr, err := hz.StartController(
    ctx,
    conn,
    hz.WithControllerFor(greetings.Greeting{}),
 )
```

This key will match all objects with the kind `Greeting`, for `v1` in the `greetings` group in any namespace with any name.

#### 2.2.1 Creating a reconciler

To define a reconciler we need to implement the `hz.Reconciler` interface:

```go
type Reconciler interface {
    Reconcile(context.Context, Request) (Result, error)
}
```

We can do so with a struct as follows:

```go
type GreetingReconciler struct {}

// Reconcile implements hz.Reconciler.
func (r *GreetingReconciler) Reconcile(
    ctx context.Context,
    req hz.Request,
) (hz.Result, error) {
    // TODO: the actual reconcile logic here.
    return hz.Result{}, nil
}
```

You can see the full example in [reconciler.go](../examples/greetings/reconciler.go).

#### 2.2.2 Creating a validator

TODO: once validator supports create/update/delete validation.

You can see the full example in [validator.go](../examples/greetings/validator.go).

#### 2.2.3 Running our controller

Look at [greetings.go](../examples/greetings/cmd/greetings.go) for an example of how to start a controller.

You will need the NATS credentials we generated earlier. To run the greetings example, just do:

```console
export NATS_CREDS=./nats.creds"
go run ./examples/greetings/cmd/greetings.go
```

You can run multiple controllers in the same binary, so don't feel like you have to create a separate binary for every controller.

### 2.3 Creating a portal

A portal in Horizon simply subscribes to a NATS subject that receives HTTP messages (as `[]byte`).
It converts each message into a `http.Request` and sends it to your `http.Handler` that you can implement however you want (e.g. Go stdlib, [Chi](https://github.com/go-chi/chi) or [Echo](https://github.com/labstack/echo)).

We start the portal by calling `hz.StartPortal(...)` which handles all the NATS subscriptions and conversion of NATS messages to `http.Request` and calling your `handler.ServeHTTP(w htt.ResponseWriter, r *http.Request)`.

All you need to do is write a Go server!

Here's the most minimal example imagineable:

```go
ctx := context.Background()
conn, _ := nats.Connect(nats.DefaultURL)
portalObj := hz.Portal{
   ObjectMeta: hz.ObjectMeta{
       Namespace: hz.RootNamespace,
       Name:    "greetings",
   },
   Spec: &hz.PortalSpec{
       DisplayName: "Greetings",
   },
}
mux := http.NewServeMux()
portal, err := hz.StartPortal(ctx, conn, portalObj, mux)
```

We have been having a great time with [Templ](https://templ.guide/) and [HTMX](https://htmx.org/) for building portals.

Take a look at the greetings [portal.go](../examples/greetings/portal.go).

### 2.4 Testing your extension

Testing was one of the major reasons why we accidently started building Horizon.
Having working with Kubernetes controllers we found the testing phase to be... lacking developer experience.

One major advantage of NATS, and therefore Horizon, is that we can easily embed a NATS server into our Go binaries, and therefore our tests!

To make it nicer, we wrapped this into a nice package.

To start a test Horizon server, with NATS and all the core components, just do this:

```go
import (
    "context"
    "testing"
    "github.com/verifa/horizon/pkg/server"
)

func TestGreeting(t *testing.T) {
    ctx := context.Background()
    // Create a test server which includes the core of Horizon.
    ts := server.Test(t, ctx)
    // Rest of the test here...
}
```

That's it! Look at the greetings [test](../examples/greetings/greetings_test.go) for more inspiration.

## 3. Next steps

Now that you have an idea of how to write controllers and portals, the best idea is to think of a simple use case you have in mind.

Try to define an object that represents how your users would think about it.
Have a quick read up on Domain Driven Design and about using language that your users would understand.
I.e. **do not** copy the interface of whatever service you are interfacing over into Horizon - that would be pointless.

It is also very important that you understand the why and how of [server side apply](./serversideapply.md). It is important when modelling your objects and writing controllers.
