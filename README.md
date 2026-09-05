# ![Logo](images/logo.png) Ghoti

[![Go Reference](https://pkg.go.dev/badge/github.com/dankomiocevic/ghoti.svg)](https://pkg.go.dev/github.com/dankomiocevic/ghoti)
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Fdankomiocevic%2Fghoti.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2Fdankomiocevic%2Fghoti?ref=badge_shield)
[![Codecov](https://img.shields.io/codecov/c/github/dankomiocevic/ghoti)](https://app.codecov.io/gh/dankomiocevic/ghoti)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/dankomiocevic/ghoti/badge)](https://securityscorecards.dev/viewer/?uri=github.com/dankomiocevic/ghoti)

Ghoti is a fast and simple service that helps distributed systems by centralizing some key information that is really hard to work with when it is distributed.
Distributed systems are complicated, there are too many moving parts and sometimes a simple task becomes really complex.

This is why we created Ghoti, because sometimes the problem can be easily resolved by removing all the distributed parts of it. But having a centralized solution, even for a small part of the problem usually doesn't come for free, it just generates a single point of failure and it is harder to scale up.

This is why Ghoti is created with a very Unix approach, let's do one thing and do it right!

There are not many things that can be done with Ghoti, but these things, like the classic Unix CLI tools are the building blocks for bigger things.

Ghoti is created with the following requirements:
- Is fast, all requests must have single digit latency.
- Is focused on throughput, it can handle tens of thousands of clients and support thousands of requests per second (official benchmarks on the way)
- It is resilient, a Ghoti cluster is designed to have minimal downtime and high availability.
- Chaos is in its core, design decisions are based on the fact that everything fails.

Ghoti does not persist data.

A Ghoti cluster allows to maintain availability but does not enforce data persistence. Ghoti servers are used to keep track or propagate what is happening in the moment but must not be used to store information.

Let's say that you are using a Ghoti node as a cache to store information, that information can be lost. But, if you are using it as a cache, and your application depends on this information, there is something in the design that needs to be revisited. If is truly a cache, the overall system must not fail when is down. It could have a performance hit, or be degraded temporarily, but not fail.

This is why by enforcing the no-persistence and reminding you about that "systems can fail" we want to make Ghoti simple and hopefully make the overall design better.

## Protocol

Ghoti uses slots to communicate, if you ever worked with microcontrollers you would get the similarities with registers.
The idea is that you can either write or read a slot. Slots hold at most 36 bytes of data and go from slot #0 to slot #999.

All messages are plain text in order to simplify the protocol among different programming languages.

### Framing and limits

These rules are exact and apply to every command described below.

|Rule|Value|
|----|-----|
|Request size|At least 4 bytes and at most 40 bytes, not counting the terminator. The only exception is `q`, which may be sent on its own.|
|Slot value size|At most 36 bytes.|
|Unit of measurement|**Bytes, not characters or Unicode code points.** Lengths are never counted in code points, so 36 ASCII characters fit, 18 two-byte characters (`é`) fit, and 9 four-byte emoji fit, but 36 two-byte characters are rejected with `001`.|
|Encoding|The value is an opaque byte string. Ghoti never validates, normalises or transcodes it, and returns exactly the bytes it received.|
|Request terminator|A single `\n` on the `standard` transport, `\r\n` on `telnet`. The `http` transport has no terminator, one command is one request. See [Protocol variants](#protocol-variants).|
|Response terminator|Always a single `\n`, on every transport, including `telnet`.|
|Requests per write|**Exactly one.** See below.|

**One command per TCP write.** Ghoti reads each request with a single read into a fixed buffer and requires the request to end at the end of that read. This has two consequences that clients must respect:

- **Commands cannot be pipelined.** If two commands arrive in the same TCP segment, only the first one is executed and the second is discarded *silently*, with no error response. A client must send one command and wait for its response before sending the next one.
- **A command must not be split across writes.** If a command arrives in two segments, each fragment is answered with a separate `exxx001` parse error. The command is never reassembled.

**Framing responses.** Ghoti batches pending responses and async events, so a single read on the client may return several messages concatenated. Clients must split the incoming stream on `\n` and never assume that one read yields one message. For example, a write to a broadcast slot can arrive as a single segment holding both the async event and the confirmation, one per line:

```
a004HelloWorld
v0042/2/0
```

**Ordering.** Responses to commands sent on the same connection are returned in the order the commands were sent: Ghoti handles a connection's commands one at a time and finishes writing each response before reading the next command. Async events (`a`) originate from other connections and may appear before or after any response, but never in the middle of one. There is no ordering guarantee across different connections.

**Response types.** The first byte of every message identifies its type: `v` (value), `e` (error) or `a` (async event). Clients should treat any other leading byte as unknown, discard the message up to the next `\n`, and continue reading rather than closing the connection; this keeps clients working when new response types are added.

**Retries.** A command may be retried safely only if it is idempotent, and the table below is the authority on which are:

|Command and slot|Safe to retry|
|----------------|-------------|
|`r` on simple memory, timeout memory, broadcast, multicast|Yes|
|`r` on atomic counter|**No**, every read increments the counter|
|`r` on token bucket, leaky bucket|**No**, every read consumes from the bucket|
|`r` on ticker|**No**, every read advances the countdown|
|`w` on simple memory, timeout memory, atomic counter, ticker|Yes, the value is absolute|
|`w` on broadcast, multicast|**No**, every write re-sends the event to every client|
|`s`, `d`|Yes, both are idempotent|
|`u`, `p`|A failed `p` closes the connection, so a retry means opening a new connection and starting the login again|

Because a lost response cannot be told apart from a command that was never executed, a client that needs at-most-once semantics for the non-idempotent commands has to reconcile the state itself, for example by reading the slot back where the slot type allows it.

### Commands

In order to read a slot you can send a read request. That would be the command `r`, then three digits defining the slot number .

`r000`

This will trigger a value response with the information about the slot. The response `v` indicates is a value response, then three digits to determine the slot and up to 36 bytes to define the value.

`v0006396A64C-1C2C-4BFC-B8F1-034758018CAC`

In this example, the slot has a UUID stored.

When a client wants to write a value on a slot, they can use the `w` command:

`w000HelloWorld`

This will write the value `HelloWorld` on the slot `000`. The value can be any byte string with a maximum of 36 bytes.
Same as the read command, the server will return the written value:

`v000HelloWorld`

If there is any issue with a command, the server will return an error with a code that can be used to identify the issue:

`e000008`

In this case, the error code has 3 parts:
- `e` indicates is an error response.
- `000` is the slot number.
- `008` is the error code.

In the case of commands that are not related to a specifc slot, the slot number will be "xxx". For example, if you want to login, the command to enter the password would be `p` and it won't be related to any slot. The response when the password is too short would be:

`exxx003`

Where `e` indicates is an error response, `xxx` is the slot number and `003` is the error code.

To identify the error code, the list of error codes can be found [here](internal/errs/README.md).

In some cases there are messages sent as async events from the server (see broadcast slots), these kind of messages are sent at any time and use the `a` (async) response:

`a2346396A64C-1C2C-4BFC-B8F1-034758018CAC`

Same as the other examples, it would contain the `a` response, then the slot (in this case 234) and the event data (in this case a UUID).

NOTE: Async events can happen at any time.

Some slots manage a group of clients that should receive their events, instead of all the connected clients (see multicast slots). To join that group a client can use the `s` (subscribe) command, and to leave it the `d` (deregister) command, both followed by the three digits identifying the slot:

`s005`

The response works the same way as a read, a non-zero value means the client is now part of the group:

`v0051`

`d005`

`v0050`

Sending `s` or `d` to a slot that does not support groups of clients returns error `010`.

A client can close its own connection cleanly with the `q` (quit) command. It takes no slot number and the server does not answer it, it just closes the connection:

`q`

### Command summary

|Command|Form|Answered with|
|-------|----|-------------|
|`r`|`r` + 3-digit slot|`v` + slot + value, or `e`|
|`w`|`w` + 3-digit slot + value (up to 36 bytes)|`v` + slot + value, or `e`|
|`s`|`s` + 3-digit slot|`v` + slot + `1`, or `e`|
|`d`|`d` + 3-digit slot|`v` + slot + `0`, or `e`|
|`u`|`u` + username|`v` + username, or `e`|
|`p`|`p` + password|`v` + username, or `e`|
|`q`|`q`|Nothing, the connection is closed|

Any other command byte is rejected with `exxx001`. The bytes after the slot number are ignored for every command except `w`, so `r000JUNK` is treated as `r000`.

### Protocol variants

The core protocol is always the same despite the variant selected, but there are different options to use as a transport layer. The following are the available options:
- standard: The protocol works as described in the previous section, it is a plain TCP connection that requires messages to be sent in plain text and terminated with a newline character. This is the default option.
- telnet: This option is the same as the standard option but it allows the use of the telnet protocol to connect to the server. This option is useful when you want to use a telnet client to connect to the server. The main difference is that the messages are terminated with a return of carriage and a newline character, as specified in the standard telnet protocol.
- http: Exposes the server over HTTP. Slots can be read with `GET /<id>` and written with `POST /<id>`, where `<id>` is the 3-digit slot number, so reading slot 0 is `GET /000`. Any other path shape is rejected with `400`. The request body of a `POST` is the value, and a trailing `\n` or `\r\n` in it is stripped. For **broadcast** slots, a `GET` request opens a persistent [Server-Sent Events (SSE)](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events) stream, so the client receives each broadcast event pushed in real time without polling. Which slots are streaming is determined from the configuration at startup, so there is no runtime overhead per request. Authentication uses HTTP Basic Auth.

Line endings are strict and differ per transport:

|Transport|Request terminator|Response terminator|
|---------|------------------|-------------------|
|standard|`\n`. A request ending in `\r\n` is accepted, but the `\r` is kept as part of the value, so `w000abc\r\n` stores `abc\r`. Always send a bare `\n`.|`\n`|
|telnet|`\r\n` only. A request ending in a bare `\n` is rejected with `exxx001`.|`\n`, **not** `\r\n`|
|http|Not applicable, one command per request.|Not applicable|

Over HTTP the response is translated into a status code: `200` with the value as the body, `403` for a permission error, `404` for a slot that is not configured, `503` when the node is not the cluster leader, and `400` for any other error. The `s`, `d`, `u`, `p` and `q` commands have no HTTP equivalent, so multicast groups cannot be joined over this transport.

Example config:

```yaml
protocol: http
```

## Configuration 

Ghoti has 1000 configurable slots that can be used to provide different functions.
Slots are configured through configuration files, if a slot configuration changes Ghoti cannot enforce consistency in the data until the new configuration is propagated.
Clients must know the configuration beforehand in order to use the slots appropriately.

For example, the same Ghoti server can be configured to have the first 3 slots as rate limiters and the next two as multicast signal propagation slots.
This way the applications can use a single server to solve more than one problem. I mean, is already there!

### Simple memory slot

This is the most basic slot where a value can be stored. The value has a maximum of 36 bytes. You can read and write on the value and there are no restrictions.
This slot has also no configuration.

Example config:

```yaml
slot_000:
  kind: simple_memory
```

### Timeout memory slot

This slot is also a memory slot but the main difference with the Simple memory slot is that it has an owner. Only the client that has last written in this slot can write again.
If the owner does not write on this slot for a certain time (timeout), it will lose the ownership and any other client can take over.

The timeout can be configured:

|Config      |Value                               |
|------------|------------------------------------|
|timeout     |Timeout value configured in seconds.|

All clients can read from this slot, but only the owner can write. If any other client tries to write it will fail with error `007`. If there is no owner, the first client that writes becomes the owner.

**The owner is the connection, not the user or the host.** Ownership is tracked by the TCP connection that performed the write, so it is not affected by which user is logged in on it, and two connections from the same process or the same authenticated user are two different owners.

This has consequences a client has to plan for:

- **A lost connection does not release the slot.** Ownership is only released when the timeout expires. A client that reconnects immediately after a drop is a new owner and its writes are rejected with `007` for the remainder of the timeout, so the timeout is effectively the recovery time after a crash. Pick it accordingly.
- **The timeout is only refreshed by writes.** Reading the slot does not extend ownership.
- **Over the `http` transport every request is its own connection**, so no HTTP client can ever be the owner of two consecutive writes. Timeout memory slots are only usable over `standard` and `telnet`.

Example config:
```yaml
slot_001:
  kind: timeout_memory
  timeout: 10
```

### Token bucket limiter

This limiter uses the classic token bucket approach to control the rate of events. Applications can request tokens from the limiter and the limiter will return the number of tokens assigned.
This can be used for example by a distributed fleet of API servers, allowing them to centralize the rate limit for the calls.

The token bucket approach adds a certain number of tokens per period (for example a second), to a bucket. Applications can take tokens from the bucket. After the tokens are depleted, it won't return any more tokens until the next period is reached.

For example, let's say we have an API that provides 100 requests per second, and we also want to allow the application to allow brief spikes in traffic up to 2x the maximum amount.
In this case we can configure our slot with the following:

|Config      |Value |
|------------|------|
|bucket_size |200   |
|period      |second|
|refresh_rate|100   |

The complete configuration options are:

|Config          | Description |
|----------------|-------------|
| bucket_size	 | Max amount of tokens that can be accumulated. |
| period	     | The refresh period for the tokens, it can be: second, minute or hour |
| refresh_rate	 | The number of tokens added on every refresh period. Default: 1 |
| tokens_per_req | This is the number of tokens that are assigned on every request. This is used to reduce the number of calls to the server, applications can have more tokens available to be used, when those are depleted it can ask for more. If the number is not available, the available number will be returned. Default: 1 |

Writes have no effect on this slot. Reads will return the number of tokens (or zero if there are no tokens available).


Example config:
```yaml
slot_002:
  kind: token_bucket
  bucket_size: 100
  period: second
  refresh_rate: 50
  tokens_per_req: 5
```

### Leaky bucket limiter

This limiter works by defining an imaginary bucket that has a leak on it. The idea is that the leak is the rate how those tokens get delivered at a constant rate.

The bucket has a limited capacity, then it would remove tokens at a constant rate (defined by config). Every time we do a request, we put a token in the bucket, if there is enough room in the bucket, then the request will be approved, if there is not enough room the request will be denied. When a bucket is full, it will return 0 to all the requests until a token is leaked from it, then it will have room to receive a new token and so on.

The bucket size allows applications to have a burst of requests but after some time the bucket will fill up and the requests will start at a constant rate. If you don't want to have a burst of requests, you can set the bucket size to 1.

|Config          | Description |
|----------------|-------------|
| bucket_size	 | Max amount of tokens that can be accumulated. |
| refresh_rate	 | The number of milliseconds to wait until a token is leaked. Default: 1000 |

Writes have no effect on this slot. Reads will return 1 if the token was accepted or zero if not.

Example config:
```yaml
slot_003:
  kind: leaky_bucket
  bucket_size: 100
  refresh_rate: 1000
```

### Broadcast signal propagation

Anything sent to this slot is propagated as a message to every connected client, including the one that wrote it. Any client connected to Ghoti at this point will receive the event at least once.
This means that the message could be received more than once.

The message is sent as an async event, the receiving client will receive the message at any time.
The message format is the following:

```
<a000HelloWorld
```

Where `a` is the async event, `000` is the slot number and `HelloWorld` is the message.

The message to be sent has a maximum of 36 bytes, this allows to send an ID or a UUID to all the hosts.

This kind of slot is used to notify other clients about a new event or to propagate a signal.

There is no configuration for this slot beyond selecting the kind:

```yaml
slot_000:
  kind: broadcast
```

This slot will only acknowledge the command when all the messages are sent, so take into account that the more clients connected or the hardest those clients are to reach, it will delay the confirmation. The confirmation contains the following information:

```
v000a/b/c
```
Where:
- `a` is the number of clients that acknowledged the message.
- `b` is the number of clients the message was dispatched to, which is every connection open at that moment, **including the one that wrote the slot**.
- `c` is the number of failures.

Note that `a + c` can be lower than `b`: the server waits up to 200 ms for the acknowledgements and stops counting after that.

Example, with five connections open including the writer:
```
>w000HelloWorld
<a000HelloWorld
<v0003/5/2
```
This means that the message was dispatched to 5 clients, 3 acknowledged it and 2 failed.

The writer receives its own message too, so a client that writes to a broadcast slot must expect the `a` event for its own write before the `v` confirmation.

Writes will propagate the written value to all the connected clients. Reads will read the last written value.

### Multicast signal propagation

Similar to the Broadcast slot but this slot allows to send a message to a specific group of clients, instead of every connected client.

Unlike the Broadcast slot, a client has to join the group before it can receive events. This is done with the `s` (subscribe) command described in the Protocol section. A client that no longer wants to receive events can leave the group with the `d` (deregister) command. Reading the slot with `s`/`d` returns whether the client is now a member: a non-zero value means it is registered, zero means it is not.

Writing to this slot works the same way as the Broadcast slot, but the message is only sent to the clients that are currently registered:

```
>s005
<v0051
>w005HelloWorld
<a005HelloWorld
<v0051/1/0
```

In this example, the client subscribes to slot `005`, then a message is written to it. The subscribed client receives the async event and the write is acknowledged with `1/1/0`, meaning 1 client received it out of 1 that was sent to, with 0 failures.

If a registered client fails to receive `dereg_tries` consecutive messages (for example because it disconnected without deregistering), it is automatically removed from the group.

Reads (`r`) on this slot return the last message written, same as the Broadcast slot.

|Config          | Description |
|----------------|-------------|
| timeout        | Time in milliseconds to wait for a confirmation on the clients that the message was received. Default: 200 |
| dereg_tries    | Number of consecutive failed messages tried on a client until is de-registered. Default: 3 |

Example config:
```yaml
slot_005:
  kind: multicast
  timeout: 200
  dereg_tries: 3
```

### Random signal propagation (TBD)

This signal propagation slot works like the Multicast signal propagation explained before but with a major diference, the message is not sent to all registered clients, but only one. It uses a pseudo-random generator to distribute the messages among the clients.

Same as the Multicast slot, clients join and leave the group with the `s` and `d` commands described in the Protocol section.

It has the same configuration as the previous slot:

|Config          | Description |
|----------------|-------------|
| timeout        | Time in milliseconds to wait for a confirmation on the clients that the message was received. |
| dereg_tries    | Number of consecutive failed messages tried on a client until is de-registered. |


### Ticker (watchdog)

This is a classic slot used in embedded circuits and microcontrollers, the slot contains an integer value, the way this works is that the slot will tick once a second making its value go down by one until it reaches zero.
If any client writes to this slot and sets a value (integer value), it will start decrementing that value once a second until it reaches zero again.

In other words, if a client writes `600` on this slot, then waits 9 minutes and reads the value, the value will be `60`. After one more minute, the value will be zero.

|Config          | Description |
|----------------|-------------|
| initial_value  | Initial value for the ticker. Required, must not be negative. |
| refresh_rate	 | The number of milliseconds per tick. Required, must be at least 1. |

Both options are required and the server refuses to start if either is missing.

Writes must be a non-negative integer in decimal, anything else is rejected with error `007`. The countdown is evaluated lazily on read, so the value only moves when the slot is read.

Example config:

```yaml
slot_003:
  kind: ticker
  initial_value: 600
  refresh_rate: 1000
```

### Atomic counter slot

This slot contains an integer number and allows to increment its value atomically.

You can write a non-negative integer to the slot and it will set the current value to that number. A negative value or anything that is not an integer is rejected with error `007`.

**Reading this slot increments it and returns the value after the increment.** A slot that has just been created returns `1` on the first read, `2` on the second, and so on; after a write of `5` the next read returns `6`. When the counter reaches the maximum value of a signed 64-bit integer it wraps back to `0`.

Since every read has a side effect, a read on this slot must never be retried blindly. See the retry table in the Protocol section.

There is no configuration for this slot beyond selecting the kind:

```yaml
slot_002:
  kind: atomic
```

## Auth

Ghoti allows to have an authentication mechanism to allow different actors to interact only with specific slots. This means that you can configure who access which slots and who is able to read or write on it.

First, you need to define your client services or users on the configuration:

```yaml
users:
  my_service: "my_password"
  other_service: "another_password"
  upstream: "123456"
```

The clients can now login using the `u` and `p` commands:

```
send   > umy_service
receive< vmy_service
send   > pmy_password
receive< vmy_service
```

The server will respond with the `v` value returning the username of the logged in user or `e` if there is an error. It is recommended using this feature only through a secure connection, on a very secure network or through TTL because the passwords will not be encoded.

Now, all the interactions with the server will be throught the autenticated user.

The login has rules that a client has to follow exactly:

- **Both commands must be sent, in order.** `u` only records the name on the connection, it does not check it against the configured users, so `u` on an unknown name still answers with `v` and that name. The credentials are only verified when `p` arrives. A connection that sends `u` and never sends `p` stays unauthenticated.
- **Username and password must both be at least 4 bytes long**, and at most 39. The username must also match `^[a-zA-Z][a-zA-Z0-9_]*$`, so it starts with a letter and continues with letters, numbers or underscores.
- **Any login failure closes the connection.** The server writes the error and then disconnects, so the client has to open a new connection to try again. This applies to a malformed username (`002`), a password that is too short (`003`) and a username and password pair that does not match the configuration (`004`).
- Sending `u` again on an already authenticated connection **drops the authentication**: the connection goes back to anonymous until a matching `p` is accepted.
- Over the `http` transport this exchange does not exist, credentials go in the HTTP Basic Auth header on every request instead.

After defining the users, we can update the slots with the specific permissions:

```yaml
slot_001:
  kind: simple_memory
  users:
    my_service: "r"
    other_service: "w"
    upstream: "a"

slot_002:
  kind: simple_memory
  users:
    my_service: "a"

slot_003:
  kind: simple_memory
```

There are three possible configurations for the access:
- r: read only
- w: write only
- a: all access

With this configuration, the client `my_service` can ready both slots 001 and 002 but can only write on the slot 002.

**IMPORTANT**:
When a slot has no defined list of users, then it will have anonymous access by default. This means that the slot can be accessed by anyone with or without logging in.

For example, the slot 003 in the configuration can be accessed by anyone, even if is not logged in.

## Metrics

Ghoti can optionally collect lightweight runtime metrics (connected clients, requests per second, average request latency) and write them to rotating files in the [Prometheus exposition format](https://prometheus.io/docs/instrumenting/exposition_formats/). Collection uses lock-free atomic counters, so it adds negligible overhead to request handling, and is disabled by default.

To enable it, add a `metrics:` section to the configuration:

```yaml
metrics:
  enabled: true
  output_dir: /var/log/ghoti/metrics  # directory where metric files are written
  rotation: daily                     # "daily" (default) or "hourly"
  retain: 7                           # number of rotation files to keep (default: 7)
  interval: 10                        # seconds between metric snapshots (default: 10)
```

|Config      |Description                                                          |
|------------|----------------------------------------------------------------------|
|enabled     |Enables metrics collection and writing. Default: false.               |
|output_dir  |Directory where metric files are written. Required when enabled.     |
|rotation    |How often a new file is started: `daily` or `hourly`. Default: `daily`.|
|retain      |Number of rotation files to keep; older files are deleted. Default: 7.|
|interval    |Seconds between metric snapshots. Default: 10.                        |

Each snapshot exposes the following metrics:

|Metric                                     |Description                                                  |
|--------------------------------------------|--------------------------------------------------------------|
|`ghoti_connected_clients`                    |Number of currently connected clients (gauge).                |
|`ghoti_requests_per_second`                  |Requests processed per second over the last interval (gauge).|
|`ghoti_request_duration_milliseconds`        |Average request duration in milliseconds over the last interval (gauge).|

## Cluster configuration (Experimental)

Ghoti clusters are created to increment availability, they are not supposed to propagate information to other nodes in order to increase data persistence. When a cluster node fails, another node will take its place but it will start on a clean state without keeping track of the information stored before.

Ghoti does not do replication because it affects performance, and Ghoti does not persist data so there is no real reason to replicate data in the cluster.

Ghoti uses a bully election algorithm for leader election. This is a simple approach that fits well because Ghoti only needs redundancy for availability purposes, not replication. The bully algorithm avoids the overhead of more complex consensus protocols like RAFT, which provide features (such as log replication) that Ghoti does not need.

A 2-node cluster is the suggested default configuration. Why 2? Because there are two main reasons to use the cluster mode:
- One of the main reasons is to be able to perform deployments with minimal downtime. So you can replace one node, then convert it to leader and replace the other one. That generates minimal impact.
- If there is an issue with one of the nodes, the other one can take over. 

If there is something really bad happening (like an issue during a deployment), then the only impact is an increased downtime. If this increased downtime needed to enable a new node is too high, then you can add a third node.

### Routing traffic to the leader

Only the leader of a cluster serves client requests; followers answer every command with a `NOT_LEADER` error. To put a load balancer in front of the cluster, enable the leader endpoint:

```yaml
cluster:
  node: "node1"
  user: "cluster_user"
  pass: "cluster_pass"
  manager:
    type: "join_server"
    addr: "0.0.0.0:2222"
  leader:
    enabled: true
```

This serves `GET /leader` on the cluster manager address, which returns `200` on the leader and `503` on every follower. Point a load balancer health check at it and traffic follows the leader automatically, including after a failover. The endpoint is unauthenticated so that health checks can reach it, and it is disabled by default.

See [cluster.md](cluster.md) for the full cluster documentation.

# Next steps

This list is not exhaustive, but it is a good starting point to understand what is missing and what is planned for the future.
Here are some of the things that are planned for the future:
- Implement missing slots.
- Add benchmark for the performance of the slots.
- Add docker support.

# License

This software is using the Apache-2.0 license.

[Complete license](LICENSE.md)
