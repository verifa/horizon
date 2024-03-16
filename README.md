# Horizon 🌅 - Build your Internal Developer Platform

When building an internal developer platform it is very important to define a Platform API.
The API should model your end users' domain and decouple them from the underlying platform technologies (such as Kubernetes, AWS, GCP, Azure).
On top of the platform API we can build user interfaces, such as web portals and command-line tools.

Horizon offers a Kubernetes-like API and libraries to help you build internal developer platforms with significantly less complexity and overhead, whilst focusing on developer experience for the platform developers. It is a thin wrapper written in Go on top of [NATS](https://nats.io/).

Learn more about [Platform Engineering](./docs/learning.md#platform-engineering) to understand the use cases for Horizon.

## Why Horizon?

Building internal platforms that enable developer self-service and improve the developer experience is hard.

Traditional "DevOps Teams" typically expose tools like Terraform and Helm directly to the end users.
Those abstractions are [leaky](https://en.wikipedia.org/wiki/Leaky_abstraction) and do not decouple your users from  the underlying technology (i.e. Terraform and Kubernetes).
This negatively impacts developers' flow and cognitive load and also makes maintaining a stable platform API very difficult.

Modern "Platform Teams" typically deploy an Internal Developer Platform / Portal, like [Backstage](https://backstage.io/).
This provides a very elegant UI for developers, but does not take care of provisioning anything.
For provisioning you still need something like Kubernetes + some infrastructure provisioner (like CrossPlane or a Terraform controller).
This is a very complex stack of tools to maintain and requires significant platform development to be useful.

Horizon is an attempt to get the best of both worlds without a complex stack to maintain.
Horizon's only hard dependency is [NATS](https://nats.io/), and it makes heavy use of it!

Horizon is not "off the shelf".
In fact the idea is that you build and own the Horizon extensions you need.
Do not expect an ecosystem of plugins you can install or lots of pretty graphs and UI elements: expect a thin layer on top of NATS to help you build only what you need.

## How it works

As the platform team building this for your end users (i.e. the software development teams) you are creating **an interface** on top of your **underlying platform**. This interface is incredibly important because it **decouples your end users** away from the **platform technologies**.
The interface should be designed to enable your developers in thier domain, not to mirror the API on top of the underlying platform (e.g. Kubernetes, AWS, GCP, Azure).

> [!WARNING]
> If you want to use Horizon you will need to write and maintain code.
>
> At this time, Go is the only supported language.

Below is a high-level overview of the different Horizon components. For a more detailed explanation of the components, see the [Architecture](#architecture) section.

![overiview](./docs/drawings/overview.excalidraw.png)

> [!NOTE]
> All communication is handled via NATS. As such, the dotted lines between components do not actually exist but are subject-based pub/subs via NATS.

Your end users will typically interact with Horizon via HTTP servers (portals) or command line tools that the platform team build. More on [portals](./docs/architecture.md#platform---portals).

For provisioning "resources" (e.g. cloud infrastructure, Git repositories, artifact respositories) the platform team will define objects and a controller to handle reconciliation (taking an object specifiation and moving the object towards the desired state). More on [controllers](./docs/architecture.md#platform---controllers).

For running operations across different nodes, actors can be called via the broker and actors are selected based on their labels. More on [actors](./docs/architecture.md#platform---actors).

## Project status

> [!WARNING]
> Horizon is still a Proof of Concept.
>
> We are piloting Horizon with a few projects at the moment.
> If you think Horizon could be useful for your internal platform then please give it a try, and feel free to start a discussion with your use case and we will try to help.

**Store and Controllers:** should be considered fairly stable. This is the foundations of Horizon and is where most of the effort has been spent.

**Gateway and Portals:** we are not sure if the abstraction is correct. It might be easier to provide the gateway as a library allowing teams to fully customise and build their own version. There are so many opinions about developing web servers and styling with CSS.

If you try Horizon please give us your feedback via [GitHub discussions](https://github.com/verifa/horizon/discussions)!

## Architecture

Check the [architecture](./docs/architecture.md) document for some more information on the different components.

## Getting started

Check the [getting started](./docs/gettingstarted.md) section.

## Examples

Check the [examples](./examples/) folder for some examples.

A recommended starting point would be the incredibly useful (joke) [greetings](./examples/greetings/README.md) example.
It has a portal, controller and actor and does not depend on any external services, so there is no added complexity due to third party dependencies.

## Rationale

To understand how and why Horizon came into existence, check the [rationale](./docs/rationale.md).

## Frequently Asked Questions

Check the [FAQ](./docs/faq.md) page.

## Debugging with NATS

Check the [debugging with nats](./docs/debugging_nats.md) page.

## Resources / Learning

Check the [learning](./docs/learning.md) for some material to help you learn NATS and generally about platform engineering.

## Who is behind Horizon?

[Verifa](https://verifa.io/) is a Nordic-based niche consultancy company who have developed Horizon to solve problems for our customers.

Our business model is based on consulting services and we are happy to help you build internal platforms with Horizon.

If you would like help with developer experience and platform engineering topics, please [contact us](https://verifa.io/contact/)!

## License

This code is released under the [Apache-2.0 License](./LICENSE).
