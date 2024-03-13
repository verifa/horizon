# Debugging NATS

This page tells you how to access NATS directly for any debugging you might want to do.

## Prerequisites

1. Horizon server + NATS server running
2. [NATS CLI](https://github.com/nats-io/natscli) installed

## Using NATS CLI

You will need NATS credentials to access NATS.

TODO: how to generate NATS credentials.

Save this to a file, such as `nats.creds` and `export NATS_CREDS=nats.creds`.

Now you can just call the NATS CLI and access the KV and other streams (remember that NATS KV is just a stream under the hood).

### Listing KVs

```console
❯ nats kv ls
╭────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮
│                                                   Key-Value Buckets                                                    │
├──────────────────┬──────────────────────────────────────────────┬─────────────────────┬─────────┬────────┬─────────────┤
│ Bucket           │ Description                                  │ Created             │ Size    │ Values │ Last Update │
├──────────────────┼──────────────────────────────────────────────┼─────────────────────┼─────────┼────────┼─────────────┤
│ hz_objects_mutex │ Mutex for hz_objects                         │ 2024-03-09 19:16:50 │ 0 B     │ 0      │ 17h26m25s   │
│ hz_session       │ KV bucket for storing horizon user sessions. │ 2024-03-09 19:16:50 │ 1.1 KiB │ 6      │ 17h32m19s   │
│ hz_objects       │ KV bucket for storing horizon objects.       │ 2024-03-09 19:16:50 │ 15 KiB  │ 26     │ 17h26m26s   │
╰──────────────────┴──────────────────────────────────────────────┴─────────────────────┴─────────┴────────┴─────────────╯
```

### Get object from KV

```console
❯ nats kv get hz_objects hz-examples.v1.Greeting.test.Pekka --raw | jq
{
  "apiVersion": "hz-examples/v1",
  "kind": "Greeting",
  "metadata": {
    "account": "test",
    "managedFields": [
      {
        "fieldsType": "FieldsV1",
        "fieldsV1": {
          "f:status": {
            "f:failureMessage": {},
            "f:failureReason": {},
            "f:phase": {},
            "f:ready": {},
            "f:response": {}
          }
        },
        "manager": "ctlr-greetings"
      }
    ],
    "name": "Pekka"
  },
  "spec": {
    "name": "Pekka"
  },
  "status": {
    "failureMessage": "",
    "failureReason": "",
    "phase": "Completed",
    "ready": true,
    "response": "Greetings, Pekka!"
  }
}
```

### Listing Streams

```console
❯ nats stream ls -a
╭──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮
│                                                           Streams                                                            │
├─────────────────────┬──────────────────────────────────────────────┬─────────────────────┬──────────┬─────────┬──────────────┤
│ Name                │ Description                                  │ Created             │ Messages │ Size    │ Last Message │
├─────────────────────┼──────────────────────────────────────────────┼─────────────────────┼──────────┼─────────┼──────────────┤
│ KV_hz_objects_mutex │ Mutex for hz_objects                         │ 2024-03-09 21:16:50 │ 0        │ 0 B     │ 17h19m32s    │
│ KV_hz_session       │ KV bucket for storing horizon user sessions. │ 2024-03-09 21:16:50 │ 6        │ 1.1 KiB │ 17h25m26s    │
│ KV_hz_objects       │ KV bucket for storing horizon objects.       │ 2024-03-09 21:16:50 │ 26       │ 15 KiB  │ 17h19m33s    │
╰─────────────────────┴──────────────────────────────────────────────┴─────────────────────┴──────────┴─────────┴──────────────╯
```

### Consumer Information

```console
❯ nats consumer info KV_hz_objects rc_AgentPool
? Select a Consumer rc_Account
Information for Consumer KV_hz_objects > rc_Account created 2024-03-13T09:05:53+02:00

Configuration:

                Name: rc_Account
         Description: Reconciler for Account
           Pull Mode: true
      Deliver Policy: Last Per Subject
          Ack Policy: Explicit
            Ack Wait: 1m0s
       Replay Policy: Instant
   Max Waiting Pulls: 512

State:

   Last Delivered Message: Consumer sequence: 6 Stream sequence: 88
     Acknowledgment floor: Consumer sequence: 6 Stream sequence: 80
         Outstanding Acks: 0
     Redelivered Messages: 0
     Unprocessed Messages: 0
            Waiting Pulls: 1 of maximum 512
```
