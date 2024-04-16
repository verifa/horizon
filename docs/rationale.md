# Rationale

## Why did we build Horizon?

Honestly, it was more of an accident through exploratory work which we now believe is incredibly useful.

It started as exploratory work using NATS for data streaming and platform engineering tasks.
We needed to dynamically manage NATS accounts and users. The initial approach was to build a Kubernetes operator to handle this (the existing [nack controller](https://github.com/nats-io/nack) supports TLS authentication only, meaning static accounts).

After writing the initial Kubernetes operator and learning more about NATS we discovered that building a controller-like reconciler on top of NATS was rather simple because a lot of the complexity is handled by NATS.
So we ported the Kubernetes controller over to NATS, and found the developer experience improved so much.

Then the first iteration of Horizon was more of a container scheduling fun experiment, trying to adopt more of a synchronous actor model style approach, and less of the stateful controller-style reconcile loops approach.
However, lo and behold, we came to accept that a reconcile loop is actually what we needed, we just didn't want to accept it...

The second iteration of Horizon was therefore much closer to what we have today, and we kept some of the actor capabilities around because they are still useful.

## So, why do we believe Horizon is useful?

Kubernetes is King in the container orchestration space.
That is not what we are trying to compete with.

However, more and more teams are using Kubernetes as a foundation for their platform (the "OS of the cloud"), by defining [custom resources](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) and developing controllers.
Developing Kubernetes operators (operator ~= custom resource + controller) is very challenging and testing them even more so.
Actually writing the controller is the fun part, maintaining it is not so fun.

Once you have your Kubernetes Operator, you still need a way for users to interact with your custom resources.
Let's pretend we do not want to expose Kubernetes directly to our end users (you might disagree here, and if so, that's fine).

Teams might create CLIs or deploy something like Backstage as a developer portal to abstract away Kubernetes.
This is quite the undertaking. Even developing CLIs to interact with Kubernetes is quite hard because there's a lot baggage that comes with Kubernetes.

This is where we believe Horizon offers a compelling alternative: a Kubernetes-like API with a great developer experience together with custom web portals, all powered by NATS.

## Alternatives / Similar tools

Horizon has no opinion about what tools/libraries/SDKs you use to communicate with external services: as you code in Go with Horizon you can do whatever you want.

Hence, things like [Terraform](https://www.terraform.io/), [Pulumi](https://www.pulumi.com/), [Helm](https://helm.sh/), various CDKs (e.g. [AWS](https://aws.amazon.com/cdk/), [K8s](https://cdk8s.io/)), or any cloud SDK can all be used by your controllers or actors.

At the same time, we feel that no existing Internal Development Portals/Platforms are quite the same as Horizon.

Tools like [Backstage](https://backstage.io/) provides a "single pane of glass" but do not provide the backend for provisioning external resources.
Of course Backstage provides a lot of features that Horizon does not, so using Backstage as a UI for Horizon is not an unfounded idea.
You could use just the server-side apply and controller logic from Horizon, and skip the portals and developing UI components.

[Kratix](https://kratix.io/) is a "platform framework" that uses [Kubernetes](https://kubernetes.io/) to provision external resources, but requires something like [Backstage](https://backstage.io/) as a UI.

Hence, we do not know of a direct alternative other than "build it yourself" :)
If you think we are wrong, please let us know.

Here is a nice visual of the [platform tooling landscape](https://platformengineering.org/platform-tooling) for reference.
