# Role Based Access Control (RBAC)

This page talks about the RBAC model that Horizon uses.

At this time, only one very rudimentary model is supported, which is inspired by [Kubernetes' RBAC model](https://kubernetes.io/docs/reference/access-authn-authz/rbac/).

## Request

Whenever the [store](./architecture.md#core---store) receives a command (e.g. apply, get, list) it needs to check if the subject trying to make the request has the necessary permissions to do so.
This is defined in the `auth.Request` struct, which includes a `Subject`, `Verb` and `Object`.
The request is asking: can the Subject perform Verb on the Object.

It is the responsibility of the `auth` package to check a request and say whether the action is permitted or not.

### Subjects

At this time, the only supported Subject is a list of groups that a user belongs to (fetched via the OIDC provider).

#### Special groups

There are some special groups to consider:

- `system:authenticated` is added to the UserInfo that comes from the session. You can use this group to target anyone who has logged in.

### Verbs

The supported verbs are:

- `read`
- `create`
- `update`
- `delete`
- `run`
- `*`

### Objects

## Roles and RoleBindings
