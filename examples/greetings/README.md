# Greetings

The greetings extension is a pointless extension that gives greetings to people.

The extension was developed to not speak with strangers and only recognises a few common Finnish names.

You can run an action (stateless, synchronous) or create an object (stateful, asynchronous) to get a greeting.

The example includes implementing:

1. A [controller](./controller.go) (with a `Reconciler` and `Validator`)
2. An [actor](./actor.go)
3. A [portal](./portal.go).

## Running the example

1. Start a Horizon server (e.g. `go run ./cmd/horizon/main.go`)
2. Generate NATS user credentials (TODO: document how)
3. Run the greetings extention: `go run ./examples/cmd/main.go`

Once you start the server and create an account you should see the "Greetings" portal on the left:

![greetings-screenshot](./greetings-example-screenshot.png)

From there you can click around and greet some people.
