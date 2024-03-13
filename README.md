# Horizon 🌅 - Build your Internal Developer Platform

> Horizon provides essential building blocks and patterns for creating a minimal internal platform that enables developer self-service and autonomy.

## Why Horizon?

Building internal platforms that enable developer self-service and improve the developer experience is hard.

At Verifa we have seen two common approaches from platform teams (also known as DevOps teams or DevEx teams):

1. Exposing tools like Terraform and Helm directly to the end users.

    Those abstractions (i.e. Terraform modules and Helm charts) are [leaky](https://en.wikipedia.org/wiki/Leaky_abstraction) and expose the underlying technology (i.e. Terraform and Kubernetes).
    This negatively impacts developers' flow and cognitive load.

2. Deploy an Internal Developer Platform / Portal (IdP), like [Backstage](https://backstage.io/).

    This provides a very elegant UI for developers, but does not take care of provisioning anything.
    For provisinioning you still need something like Kubernetes + some infrastructure provisioner (like CrossPlane or a Terraform controller).
    Also this is a very complex stack of tools to maintain and requires significant development to be useful.

Horizon is an attempt to get the best of both worlds without a complex stack to maintain.
Horizon's only hard dependency is [NATS](https://nats.io/), and it makes heavy use of it!

Horizon is not "off the shelf". In fact the idea is that you build and own the Horizon extensions you need.
Do not expect a big ecosystem or lots of plugins you can install or lots of pretty graphs and UI elements: expect a thin layer on top of NATS to help you build only what you need.

> [!WARNING]
> Horizon is still a Proof of Concept.
>
> If you have come here please be gentle and do not expect the APIs to be stable.
> If you think Horizon could be useful for your internal platform then please give it a try, and feel free to start a discussion with your use case and we will try to help.

## How it works

As the platform team building this for your end users (i.e. the software development teams) you are creating **an interface** on top of your **underlying platform**. This interface is incredibly important because it can **decouple your end users** away from the **platform technologies**.

> [!WARNING]
> If you want to use Horizon you will need to write and maintain code.
>
> At this time, Go is the only supported language. If the concept of building a platform with Go worries you, I suggest you turn around now :)

Below is a high-level overview of the different Horizon components. For a more detailed explanation of the components, see the [Architecture](#architecture) section.

![overiview](./docs/drawings/overview.excalidraw.png)

> [!NOTE]
> All communication is handled via NATS. As such, the dotted lines between components do not actually exist but are subject-based pub/subs via NATS.

Your end users will typically interact with Horizon via HTTP servers (portals) or command line tools that the platform team build. More on [portals](#platform---portals).

For provisioning "resources" (e.g. cloud infrastructure, Git repositories, artifact respositories) the platform team will define objects and a controller to handle reconciliation (taking an object specifiation and moving the object towards the desired state). More on [controllers](#platform---controllers).

For running operations across different nodes, actors can be called via the broker and actors are selected based on their labels. More on [actors](#platform---actors).

## Architecture

This section describes the different components of Horizon to build a platform.

### Core

The "core" consists of a [NATS](https://nats.io/) server and some internal services.

#### Core - NATS

Horizon requires a NATS server. Horizon makes heavy use of NATS: basic subject based pub/sub, accounts for multitenancy, streams and consumers for controllers and the Key-Value store for storing the objects (a NATS KV is actually just a glorified stream in the end).

You do not need to know NATS to get started with Horizon, but if you want to get serious with Horizon you should learn enough about NATS to debug any issues.
Horizon does not try to hide away the NATS abstractions.
Therefore if anything goes wrong, you can always connect to NATS directly for debugging.
Or if you create lots of data in Horizon and decide to migrate away, your data is readily available in NATS.

#### Core - Store

The `store` is a service that handles all server-side operations for objects in the NATS KV.

No other service is expected to interface directly with the NATS KV (except for controllers that create NATS consumers for the underlying KV stream).

The store provides basic CRUD operations (`create`, `get`, `list`, `update` and `delete`), as well as `validate` and `apply`.
The noteworthy operation is the `apply` because this works like Kubernetes' [server-side apply](https://kubernetes.io/docs/reference/using-api/server-side-apply/).

As objects in the store will be mutated by users and controllers, the server-side apply controls which fields are *owned* by the different entities.
Reading the Kubernetes documentation will give you a greater understanding of how this works.
Note that Horizon does not support client-side apply (of course you could write your own and use a store `update` operation if you really wanted to...).

The store defers all validation requests to the controller for an object.

#### Core - Gateway

The `gateway` is the single HTTP endpoint of a Horizon deployment.
It is the entrypoint (or gateway) into Horizon for end users.

It handles authentication (OIDC) and authorization (RBAC).
Portals are used to extend the HTTP-based UI and the gateway uses an HTTP-to-NATS proxy for serving requests to the portal.
After all, portals are just HTTP servers connected via NATS and all requests go via the gateway.
Portal HTTP handlers are expected to user server-side rendering using libraries like [htmx](https://htmx.org/).

The gateway is not very complex, and much of it can be re-used so building your own gateway is justifiable if you want complete control.

#### Core - Broker

The `broker` is a service that handles actor run requests.

Upon receiving a run request, it advertises the request to all actors and forwards the request on to the first actor that responds.
Actors can choose whether to accept a request based on label selectors or any other filtering technique you want to use.

### Platform

The "platform" layer contains all the components that the platform team will develop to make Horizon actually do something!

#### Platform - Portals

Portals are how the Horizon web UI is extended.

Portals are just HTTP servers connected to NATS and the gateway proxies HTTP requests over NATS to portals.

The goal is to have a single user-facing HTTP endpoing (i.e. the gateway) and as many portals as you need, all accessible under that one endpoint.

Typically your portal HTTP servers will render HTML and use a library like [htmx](https://htmx.org/) to modify the HTML on the client-side.
As the portals are just HTTP servers you can develop whatever you want, as long as it is HTTP-based (like JSON REST APIs).

#### Platform - Controllers

Controllers are similar to Kubernetes controllers.
A controller requires you to define an object that it controls, and will perform validation and reconciliation of that object.

`Reconcilers` take an object specifiation and move the object towards the desired state.
This is handled by a reconcile loop, which you can implement.
Under the hood, a controller creates a [NATS consumer](https://docs.nats.io/nats-concepts/jetstream/consumers) that gets notified about objects in the NATS KV store.

`Validators` validate objects as they are added to the KV store.
This is similar to Kubernetes' [Admission Controllers](https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/), but instead of config languages you write code to validate objects.
By default, NATS uses [CUE](https://cuelang.org/) to validate objects and you can write custom validate functions as well.

#### Platform - Actors

Actors enable you to write synchronous actions that operate on objects.
Actions do not require any persistence, but can interact with any persistence layer (like the NATS KV store).

Actors provide a broker mechanism for selecting an appropriate instance of an actor to run the action on.
For example, when scheduling a container you want it to run on a specific node.
Actors allow you to define an action such as `RunContainer` and the broker will ensure (based on label selection) that the relevant actors run the action.

Unless you are doing things that are node-dependent (like running containers, or executing CLIs that require specific tooling), you might not need actors at all.

## Getting started

TODO.

## Examples

Check the [examples](./examples/) folder for some examples.

A recommended starting point would be the incredibly useful (joke) [greetings](./examples/greetings/README.md) example.
It has a portal, controller and actor and does not depend on any external services, so there is no added complexity due to third party dependencies.

## Frequently Asked Questions

Check the [FAQ](./docs/faq.md) page.

## Debugging with NATS

Check the [debugging with nats](./docs/debugging_nats.md) page.

## Resources / Learning

Check the [resources](./docs/resources.md) for some material to help you learn NATS and generally about platform engineering.

## Alternatives / Similar tools

Horizon has no opinion about what tools/libraries/SDKs you use to communicate with external services: as you code in Go with Horizon you can do whatever you want.

Hence, things like [Terraform](https://www.terraform.io/), [Pulumi](https://www.pulumi.com/), [Helm](https://helm.sh/), various CDKs (e.g. [AWS](https://aws.amazon.com/cdk/), [K8s](https://cdk8s.io/)), or any cloud SDK can all be used by your controllers or actors.

At the same time, we feel that no existing Internal Development Portals/Platforms are quite the same as Horizon.

Tools like [Backstage](https://backstage.io/) provides a "single pane of glass" but do not provide the backend for provisioning external resources.

[Kratix](https://kratix.io/) is a "platform framework" that uses [Kubernetes](https://kubernetes.io/) to provision external resources, but requires something like [Backstage](https://backstage.io/) as a UI.

Here is a nice visual of the [platform tooling landscape](https://platformengineering.org/platform-tooling) and Horizon would sit in the Developer Portal box.

## License

This code is released under the [Apache-2.0 License](./LICENSE).
