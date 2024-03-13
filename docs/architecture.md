# Architecture

This section describes the different components of Horizon to build a platform.

## Core

The "core" consists of a [NATS](https://nats.io/) server and some internal services.

### Core - NATS

Horizon requires a NATS server. Horizon makes heavy use of NATS: basic subject based pub/sub, accounts for multitenancy, streams and consumers for controllers and the Key-Value store for storing the objects (a NATS KV is actually just a glorified stream in the end).

You do not need to know NATS to get started with Horizon, but if you want to get serious with Horizon you should learn enough about NATS to debug any issues.
Horizon does not try to hide away the NATS abstractions.
Therefore if anything goes wrong, you can always connect to NATS directly for debugging.
Or if you create lots of data in Horizon and decide to migrate away, your data is readily available in NATS.

### Core - Store

The `store` is a service that handles all server-side operations for objects in the NATS KV.

No other service is expected to interface directly with the NATS KV (except for controllers that create NATS consumers for the underlying KV stream).

The store provides basic CRUD operations (`create`, `get`, `list`, `update` and `delete`), as well as `validate` and `apply`.
The noteworthy operation is the `apply` because this works like Kubernetes' [server-side apply](https://kubernetes.io/docs/reference/using-api/server-side-apply/).

As objects in the store will be mutated by users and controllers, the server-side apply controls which fields are *owned* by the different entities.
Reading the Kubernetes documentation will give you a greater understanding of how this works.
Note that Horizon does not support client-side apply (of course you could write your own and use a store `update` operation if you really wanted to...).

The store defers all validation requests to the controller for an object.

### Core - Gateway

The `gateway` is the single HTTP endpoint of a Horizon deployment.
It is the entrypoint (or gateway) into Horizon for end users.

It handles authentication (OIDC) and authorization (RBAC).
Portals are used to extend the HTTP-based UI and the gateway uses an HTTP-to-NATS proxy for serving requests to the portal.
After all, portals are just HTTP servers connected via NATS and all requests go via the gateway.
Portal HTTP handlers are expected to user server-side rendering using libraries like [htmx](https://htmx.org/).

The gateway is not very complex, and much of it can be re-used so building your own gateway is justifiable if you want complete control.

### Core - Broker

The `broker` is a service that handles actor run requests.

Upon receiving a run request, it advertises the request to all actors and forwards the request on to the first actor that responds.
Actors can choose whether to accept a request based on label selectors or any other filtering technique you want to use.

## Platform

The "platform" layer contains all the components that the platform team will develop to make Horizon actually do something!

### Platform - Portals

Portals are how the Horizon web UI is extended.

Portals are just HTTP servers connected to NATS and the gateway proxies HTTP requests over NATS to portals.

The goal is to have a single user-facing HTTP endpoing (i.e. the gateway) and as many portals as you need, all accessible under that one endpoint.

Typically your portal HTTP servers will render HTML and use a library like [htmx](https://htmx.org/) to modify the HTML on the client-side.
As the portals are just HTTP servers you can develop whatever you want, as long as it is HTTP-based (like JSON REST APIs).

### Platform - Controllers

Controllers are similar to Kubernetes controllers.
A controller requires you to define an object that it controls, and will perform validation and reconciliation of that object.

`Reconcilers` take an object specifiation and move the object towards the desired state.
This is handled by a reconcile loop, which you can implement.
Under the hood, a controller creates a [NATS consumer](https://docs.nats.io/nats-concepts/jetstream/consumers) that gets notified about objects in the NATS KV store.

`Validators` validate objects as they are added to the KV store.
This is similar to Kubernetes' [Admission Controllers](https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/), but instead of config languages you write code to validate objects.
By default, NATS uses [CUE](https://cuelang.org/) to validate objects and you can write custom validate functions as well.

### Platform - Actors

Actors enable you to write synchronous actions that operate on objects.
Actions do not require any persistence, but can interact with any persistence layer (like the NATS KV store).

Actors provide a broker mechanism for selecting an appropriate instance of an actor to run the action on.
For example, when scheduling a container you want it to run on a specific node.
Actors allow you to define an action such as `RunContainer` and the broker will ensure (based on label selection) that the relevant actors run the action.

Unless you are doing things that are node-dependent (like running containers, or executing CLIs that require specific tooling), you might not need actors at all.
