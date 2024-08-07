# Frequently Asked Questions

## 1. Is Horizon an alternative to Kubernetes?

No. You might run Horizon on Kubernetes, or use Horizon to provision Kubernetes resources.
So why build Horizon? To combine the controller model, an API and a web UI that is easy to extend and test.

For simple environments (e.g. home labs) you could write a container scheduler using controllers and actors and have Horizon run containers for you as an alternative to Kubernetes.

## 2. If we need to automate everything anyway, isn't this just more work?

Absolutely. Using Horizon is more work than just shipping some Terraform or Ansible scripts to end users.
Do some reading on Developer Experience and Platform Engineering.
Architecting abstractions to enable developer flow is not easy and requires effort and time.
The more developers using your platform, the more value there is in doing so.

## 3. Horizon is immature, can I trust it?

Horizon is very immature, not a CNCF project and there is no community (yet).
However, Horizon is just a thin layer on top of [NATS](https://nats.io/).
NATS is a very robust and mature technology that you can trust and Horizon makes no effort to "hide away" NATS, making it very easy to access NATS directly.
See the [debugging with nats](./debugging_nats.md) page.

## 4. If I start developing on Horizon, will I get "locked in"?

If you are solving problems that have fairly high essential complexity, then doing so with many tools glued together (bash, make, terraform, github actions etc.) adds a lot to the accidental complexity and makes it harder to test and maintain implementation.

With Horizon you will write Go code that can be reused outside of Horizon.
Reconcile logic could be ported to CI pipelines or some other automated event.
Portals are just Go servers that you could run somewhere else.

Like any tool, you will have to invest in Horizon to make it meaningful, but the idea that you get "locked in" because you write Go code (as opposed to bash) is an unfounded one.
