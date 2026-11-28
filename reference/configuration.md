# Configuration

[Ghoti](../README.md) reference: [Protocol](protocol.md) · [Configuration](configuration.md) · [Authentication](authentication.md) · [Metrics](metrics.md) · [Cluster](../cluster.md)

Ghoti has 1000 configurable slots that can be used to provide different functions.
Slots are configured through configuration files, if a slot configuration changes Ghoti cannot enforce consistency in the data until the new configuration is propagated.
Clients must know the configuration beforehand in order to use the slots appropriately.

For example, the same Ghoti server can be configured to have the first 3 slots as rate limiters and the next two as multicast signal propagation slots.
This way the applications can use a single server to solve more than one problem. I mean, is already there!

## Server settings

The configuration is a YAML file named `config.yaml`, looked up in `/etc/ghoti`, then `$HOME/.ghoti`, then the current directory. If no file is found, the server exits with an error, it does not start with defaults. To use a file somewhere else, pass its path with `ghoti run --config /path/to/file.yaml`. When `--config` is set, the default locations are not searched, so a wrong path makes the server exit instead of picking up another `config.yaml` by accident. The file is read as YAML whatever its extension is. The file is only read at start-up, so a change needs a restart, which also clears every slot.

|Config|Description|
|------|-----------|
|addr|`host:port` the server listens on. Default: `localhost:9090`. Can also be set with `ghoti run --addr`.|
|protocol|Transport: `standard` (default), `telnet` or `http`. See [Protocol variants](protocol.md#protocol-variants).|
|max_connections|Maximum open client connections. Default: `0`, no cap. See [Connection limits](#connection-limits).|
|log.level|`debug`, `info` (default), `warn` or `error`.|
|log.format|`text` (default) or `json`.|

Settings with a single value can also be set from the environment with the `GHOTI_` prefix, upper case and with `.` replaced by `_`, for example `GHOTI_ADDR`, `GHOTI_LOG_LEVEL` or `GHOTI_CLUSTER_PASS`. The environment overrides the file. Use this to keep credentials out of the file, see [SECURITY.md](../SECURITY.md). Slots are only read from the file.

Slots are configured as `slot_000` to `slot_999`, each with a `kind` and the options of that kind, described below.

**An invalid slot definition is skipped silently.** If a slot has an unknown `kind`, a missing required option or an out-of-range value, the server still starts, logs nothing about it, and that slot answers every command with error `005` (MISSING_SLOT), as if it was not configured. After changing the configuration, read each slot once to check that it exists.

## Simple memory slot

This is the most basic slot where a value can be stored. The value has a maximum of 36 bytes. You can read and write on the value and there are no restrictions.
This slot has also no configuration.

Example config:

```yaml
slot_000:
  kind: simple_memory
```

## Timeout memory slot

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

## Token bucket limiter

This limiter uses the classic token bucket approach to control the rate of events. Applications can request tokens from the limiter and the limiter will return the number of tokens assigned.
This can be used for example by a distributed fleet of API servers, allowing them to centralize the rate limit for the calls.

The token bucket approach adds a certain number of tokens per period (for example a second), to a bucket. Applications can take tokens from the bucket. After the tokens are depleted, it won't return any more tokens until the next period is reached.
Tokens keep accumulating while the bucket is idle: if several periods go by without any request, `refresh_rate` tokens are credited for each one of them, up to `bucket_size`. The bucket starts with `refresh_rate` tokens (one period's worth), not a full bucket.

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

## Leaky bucket limiter

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

## Broadcast signal propagation

Anything sent to this slot is propagated as a message to every connected client, including the one that wrote it. Delivery is at most once: the server makes a single attempt per client and does not retry the ones that fail, so a client that is slow, disconnecting or whose queue is full will miss the event. Like the ephemeral Pub/Sub in Redis or Core NATS, this slot is not a durable queue and gives no delivery guarantee.

The response to the write reports the outcome of the attempt as `received/sent/errors`, so the writer can tell how many clients confirmed the event.

The message is sent as an async event, the receiving client will receive the message at any time.
The message format is the following:

```
a000HelloWorld
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

## Multicast signal propagation

Similar to the Broadcast slot but this slot allows to send a message to a specific group of clients, instead of every connected client.

Unlike the Broadcast slot, a client has to join the group before it can receive events. This is done with the `s` (subscribe) command described in the [protocol reference](protocol.md#commands). A client that no longer wants to receive events can leave the group with the `d` (deregister) command. The response to `s` and `d` says whether the client is now a member: a non-zero value means it is registered, zero means it is not.

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

## Random signal propagation (planned, not implemented)

This slot kind does not exist yet: a slot configured with it is skipped, see [Server settings](#server-settings). The plan is a slot that works like the Multicast slot, but sends each message to only one of the registered clients, picked pseudo-randomly, with the same `s`/`d` commands and the same `timeout` and `dereg_tries` options.

## Ticker (watchdog)

This is a classic slot used in embedded circuits and microcontrollers. The slot contains an integer value that goes down by one every tick, `refresh_rate` milliseconds, until it reaches zero.
If any client writes a value to this slot, the countdown starts again from that value.

In other words, with `refresh_rate: 1000`, if a client writes `600` on this slot, then waits 9 minutes and reads the value, the value will be `60`. After one more minute, the value will be zero.

|Config          | Description |
|----------------|-------------|
| initial_value  | Initial value for the ticker. Required, must not be negative. |
| refresh_rate	 | The number of milliseconds per tick. Required, must be at least 1. |

Both options are required. If either is missing or invalid the slot is skipped and answers `005`, see [Server settings](#server-settings).

Writes must be a non-negative integer in decimal, anything else is rejected with error `007`. Ticks are aligned to multiples of `refresh_rate` on the server clock, so the first tick after a write can come up to one tick early. The value is computed from the elapsed time whenever the slot is read, so reading it does not change the countdown.

Example config:

```yaml
slot_003:
  kind: ticker
  initial_value: 600
  refresh_rate: 1000
```

## Atomic counter slot

This slot contains an integer number and allows to increment its value atomically.

You can write a non-negative integer to the slot and it will set the current value to that number. A negative value or anything that is not an integer is rejected with error `007`.

**Reading this slot increments it and returns the value after the increment.** A slot that has just been created returns `1` on the first read, `2` on the second, and so on; after a write of `5` the next read returns `6`. When the counter reaches the maximum value of a signed 64-bit integer it wraps back to `0`.

Since every read has a side effect, a read on this slot must never be retried blindly. See the retry table in the [protocol reference](protocol.md#framing-and-limits).

There is no configuration for this slot beyond selecting the kind:

```yaml
slot_002:
  kind: atomic
```

## Connection limits

By default Ghoti accepts as many client connections as the operating system allows. `max_connections` caps how many may be open at once, on every transport:

```yaml
max_connections: 1000
```

When the cap is reached, new connections are not refused: they complete the TCP handshake and wait in the kernel's accept queue until a connection closes, so a client sees a connect that succeeds and a first response that is delayed. Over `http` this also counts SSE subscribers, since each stream holds a connection for as long as it is open. A value of `0`, the default, means no cap, and a negative value is a configuration error.
