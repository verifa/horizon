# Extension Template

This is a very basic example to get you started.

It embeds the Horizon and NATS server, so you can make a single executable that also includes your extension.

You can use [gonew](https://pkg.go.dev/golang.org/x/tools/cmd/gonew) to bootstrap your project:

```console
go install golang.org/x/tools/cmd/gonew@latest

gonew github.com/verifa/horizon/examples/services your.domain/selfservice
```

**TODO: handle new namespace 403 error when logging in as `user-a`**
