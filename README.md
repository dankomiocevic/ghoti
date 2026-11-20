# ![Logo](images/logo.png) Ghoti

[![Go Reference](https://pkg.go.dev/badge/github.com/dankomiocevic/ghoti.svg)](https://pkg.go.dev/github.com/dankomiocevic/ghoti)
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Fdankomiocevic%2Fghoti.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2Fdankomiocevic%2Fghoti?ref=badge_shield)
[![Codecov](https://img.shields.io/codecov/c/github/dankomiocevic/ghoti)](https://app.codecov.io/gh/dankomiocevic/ghoti)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/dankomiocevic/ghoti/badge)](https://securityscorecards.dev/viewer/?uri=github.com/dankomiocevic/ghoti)

Ghoti is a single-binary, in-memory server that gives a fleet of services a few shared primitives: rate limiters, counters, leases and signals. You configure up to 1000 numbered slots in a YAML file, give each one a behaviour, and talk to them over a plain-text protocol that any language, or `nc`, can speak. All state lives in memory and is lost on restart, by design.

## What you can use it for

- **One rate limit shared by many instances.** Every API server takes tokens from the same [token bucket](#token-bucket-limiter) or [leaky bucket](#leaky-bucket-limiter) slot, so the limit holds no matter how many instances are running.
- **Keeping one worker in charge.** A [timeout memory](#timeout-memory-slot) slot only accepts writes from the connection that holds it. The holder renews it by writing again; once it stops, another client can take over after the timeout. Ownership is forgotten if Ghoti restarts and there is no fencing token, so treat it as a best-effort lease, not a lock that protects data.
- **Increasing numbers without coordination.** Every read of an [atomic counter](#atomic-counter-slot) increments it and returns the new value, which works for sequence numbers, job IDs or shared counts. The counter starts again from zero after a restart.
- **Pushing a signal to running processes.** A write to a [broadcast](#broadcast-signal-propagation) slot is sent to every connected client. A [multicast](#multicast-signal-propagation) slot sends it only to clients that subscribed. Browsers can receive broadcasts over Server-Sent Events. Delivery is at most once.

There are also a [simple memory](#simple-memory-slot) slot for short shared values and a [ticker](#ticker-watchdog) slot that counts down, which works as a watchdog.

## Performance

Ghoti is benchmarked with [ghoti-bench](https://github.com/dankomiocevic/ghoti-bench). It is an external client that drives Ghoti only through its public protocol, validates every response, and records closed-loop throughput and latency histograms. The scenarios and the method are described in its README. Published results are not available yet. Until they are, this README makes no throughput or latency claims.

## Quick start

Download a release, shown here for Linux on x86-64. For other platforms, pick the matching archive on the [releases page](https://github.com/dankomiocevic/ghoti/releases), or see [Installation](#installation).

```sh
curl -LO https://github.com/dankomiocevic/ghoti/releases/download/v0.3.0/ghoti_0.3.0_linux_amd64.tar.gz
tar -xzf ghoti_0.3.0_linux_amd64.tar.gz
```

Ghoti needs a `config.yaml` in the current directory, in `$HOME/.ghoti` or in `/etc/ghoti`. Create one with four slots:

```sh
cat > config.yaml <<'EOF'
slot_000:
  kind: simple_memory
slot_001:
  kind: atomic
slot_002:
  kind: token_bucket
  bucket_size: 2
  refresh_rate: 2
  period: minute
slot_003:
  kind: broadcast
EOF
```

Start the server. It listens on `localhost:9090` by default:

```sh
./ghoti run
```

In a second terminal, open a connection and type one command per line, waiting for each answer:

```sh
nc localhost 9090
```

|You type|Ghoti answers|What happened|
|--------|-------------|-------------|
|`w000hello`|`v000hello`|Stored `hello` in slot 000|
|`r000`|`v000hello`|Read it back|
|`r001`|`v0011`|The counter was incremented to 1|
|`r001`|`v0012`|And to 2|
|`r002`|`v0021`|The rate limiter granted a token|
|`r002`|`v0021`|And a second one|
|`r002`|`v0020`|The bucket is empty: denied until the next minute|

To see a signal, open a third terminal with `nc localhost 9090` and leave it idle. Then type `w003deploy-42` in the second terminal. Both connections receive the async event `a003deploy-42`, and the writer also gets `v0032/2/0`: the event was sent to 2 connections, 2 confirmed it and none failed. Press `Ctrl+C` to close `nc`, and stop the server the same way. Its state is gone with it.

## Scope and maturity

|Area|Status|
|----|------|
|Slot kinds: simple memory, timeout memory, token bucket, leaky bucket, ticker, atomic counter, broadcast, multicast|Implemented|
|Transports: plain TCP (`standard`), `telnet`, and `http` with Server-Sent Events|Implemented|
|Users and per-slot read/write permissions|Implemented. Passwords travel in cleartext, see below.|
|Prometheus metrics endpoint, connection limits|Implemented|
|Cluster mode: leader election for availability, no replication|Experimental|
|Random signal propagation slot|Planned|
|Docker image|Planned|

Plan for these properties before depending on Ghoti:

- **State is volatile.** Slot values, counters, bucket levels, lease ownership and multicast subscriptions exist only in memory. They are lost when the server restarts, and a node that becomes cluster leader starts empty. Use Ghoti for state your system can rebuild or do without. If Ghoti is unavailable, your system should degrade, not fail.
- **No TLS.** No listener encrypts traffic, including logins. Terminate TLS in a load balancer or reverse proxy in front of Ghoti, or keep the traffic on a trusted network. See [SECURITY.md](SECURITY.md).
- **Signals are at most once.** Broadcast and multicast make one delivery attempt per client and never retry. They are not a queue.
- **Clustering adds failover, not durability.** Only the leader serves requests, there is no quorum, and split-brain is possible during a network partition. See [cluster.md](cluster.md).
- **Configuration is read at start-up.** Changing slots requires a restart, which also clears their state. Clients must know the slot layout in advance.

## Documentation

This README is the reference documentation:

- [Installation](#installation) and [verifying a download](#verifying-a-download)
- [Protocol](#protocol): [framing and limits](#framing-and-limits), [commands](#commands), [command summary](#command-summary), [transports](#protocol-variants)
- [Configuration](#configuration): [server settings](#server-settings), the slot kinds, [connection limits](#connection-limits)
- [Auth](#auth): users and per-slot permissions
- [Metrics](#metrics): the Prometheus endpoint
- [Cluster configuration (Experimental)](#cluster-configuration-experimental), with the full design and failure scenarios in [cluster.md](cluster.md)

Related documents: [error codes](internal/errs/README.md), [security model](SECURITY.md), [changelog](CHANGELOG.md), and the [benchmark methodology](https://github.com/dankomiocevic/ghoti-bench).

## Installation

Every release on the [releases page](https://github.com/dankomiocevic/ghoti/releases) has one archive per platform, `ghoti_<version>_<os>_<arch>.tar.gz` for Linux and macOS and `ghoti_<version>_windows_<arch>.zip` for Windows. The archive has the `ghoti` binary at its root together with the README, the license and the changelog.

```sh
curl -LO https://github.com/dankomiocevic/ghoti/releases/download/v0.3.0/ghoti_0.3.0_linux_amd64.tar.gz
tar -xzf ghoti_0.3.0_linux_amd64.tar.gz
./ghoti version
```

Building from source needs the Go version listed in `go.mod`:

```sh
go install github.com/dankomiocevic/ghoti/cmd/ghoti@latest
```

### Verifying a download

Each release also ships with a `checksums.txt`, an SPDX SBOM next to every archive (`<archive>.sbom.json`), a Sigstore signature of the checksums file and a build provenance attestation for every archive. They let you check that what you downloaded is what the Release workflow built from the tag.

The checksum catches a corrupted or tampered download:

```sh
curl -LO https://github.com/dankomiocevic/ghoti/releases/download/v0.3.0/checksums.txt
sha256sum --ignore-missing -c checksums.txt
```

The signature proves that `checksums.txt` itself was produced by the Release workflow of this repository. It is a keyless [Sigstore](https://www.sigstore.dev/) signature, so the check is against the workflow identity rather than a key you have to trust. The signature, the certificate and the transparency log entry come together in one bundle file. It needs [cosign](https://github.com/sigstore/cosign) v3:

```sh
curl -LO https://github.com/dankomiocevic/ghoti/releases/download/v0.3.0/checksums.txt.sigstore.json
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/dankomiocevic/ghoti/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
```

The provenance attestation ties an archive to the exact commit and workflow run that built it. It is checked with the [GitHub CLI](https://cli.github.com/) and does not need any other file:

```sh
gh attestation verify ghoti_0.3.0_linux_amd64.tar.gz --repo dankomiocevic/ghoti
```

The SBOM lists the Go modules linked into the binary, for scanners like [grype](https://github.com/anchore/grype) or for your own inventory.

## Protocol

Ghoti uses slots to communicate, if you ever worked with microcontrollers you would get the similarities with registers.
The idea is that you can either write or read a slot. Slots hold at most 36 bytes of data and go from slot #0 to slot #999.

All messages are plain text in order to simplify the protocol among different programming languages.

### Framing and limits

These rules are exact and apply to every command described below.

|Rule|Value|
|----|-----|
|Request size|At least 4 bytes and at most 40 bytes, not counting the terminator. The only exception is `q`, which is exactly one byte on its own.|
|Slot value size|At most 36 bytes.|
|Slot number|Exactly three ASCII digits, `000` to `999`. Signs, spaces and padding are rejected with `exxx001`, so `r-01`, `r+01` and `r 01` are all errors.|
|Trailing bytes|Only `w` takes bytes after the slot number. For `r`, `s` and `d` the request ends at the third digit, and anything after it is rejected with `exxx001`.|
|Unit of measurement|**Bytes, not characters or Unicode code points.** Lengths are never counted in code points, so 36 ASCII characters fit, 18 two-byte characters (`é`) fit, and 9 four-byte emoji fit, but 36 two-byte characters are rejected with `001`.|
|Encoding|The value is an opaque byte string. Ghoti never validates, normalises or transcodes it, and returns exactly the bytes it received.|
|Request terminator|A single `\n` on the `standard` transport, `\r\n` on `telnet`. The `http` transport has no terminator, one command is one request. See [Protocol variants](#protocol-variants).|
|Response terminator|Always a single `\n`, on every transport, including `telnet`.|
|Requests in flight|**One.** Send a command, wait for its response, then send the next one. See below.|

**One command at a time.** A client must send a command and wait for its response before sending the next one. Ghoti handles a connection's commands sequentially, so this keeps a strict one request, one response relation and is the only supported way to use a connection. Sending further commands before the response arrives is not supported, and Ghoti may reject it in the future.

**Framing requests.** TCP is a byte stream and does not preserve write boundaries, so Ghoti does not assume that one read holds exactly one request. A request is the bytes up to and including its terminator, wherever the segment boundaries fall:

- **A command may arrive in pieces.** Bytes received before the terminator are kept until the rest of the request arrives, so a command that reaches the server in several segments is reassembled and answered once. A fragment on its own is not answered; Ghoti waits for its terminator.
- **Over-long lines are rejected.** A request longer than the limit above is answered with `exxx001` and the rest of that line is discarded up to its terminator, so the next request on the connection is read cleanly.

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
|`r` on ticker|Yes, the value is computed from the elapsed time, so reading does not change it|
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

A client can close its own connection cleanly with the `q` (quit) command. It takes no slot number and no arguments, so the request is exactly one byte and anything after it is rejected with `exxx001`. The server does not answer it, it just closes the connection:

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

Any other command byte is rejected with `exxx001`, and so is a slot number that is not exactly three ASCII digits. Only `w` takes bytes after the slot number, so `r000JUNK` is rejected with `exxx001` rather than treated as `r000`.

### Protocol variants

The core protocol is always the same despite the variant selected, but there are different options to use as a transport layer. The following are the available options:
- standard: The protocol works as described in the previous section, it is a plain TCP connection that requires messages to be sent in plain text and terminated with a newline character. This is the default option.
- telnet: This option is the same as the standard option but it allows the use of the telnet protocol to connect to the server. This option is useful when you want to use a telnet client to connect to the server. The main difference is that the messages are terminated with a return of carriage and a newline character, as specified in the standard telnet protocol.
- http: Exposes the server over HTTP. Slots can be read with `GET /<id>` and written with `POST /<id>`, where `<id>` is exactly three ASCII digits, so reading slot 0 is `GET /000`. Any other path shape, including `/+00`, `/-01` and `/1`, is rejected with `400`. The request body of a `POST` is the value, and a trailing `\n` or `\r\n` in it is stripped. For **broadcast** slots, a `GET` request opens a persistent [Server-Sent Events (SSE)](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events) stream, so the client receives each broadcast event pushed in real time without polling. Which slots are streaming is determined from the configuration at startup, so there is no runtime overhead per request. Authentication uses HTTP Basic Auth.

Line endings are strict and differ per transport:

|Transport|Request terminator|Response terminator|
|---------|------------------|-------------------|
|standard|`\n` only. The `\r` of a `\r\n` ending is kept as part of the request, so `w000abc\r\n` stores `abc\r` and `r000\r\n` is rejected with `exxx001` as a read with trailing bytes. Always send a bare `\n`.|`\n`|
|telnet|`\r\n` only. A request ending in a bare `\n` is rejected with `exxx001`.|`\n`, **not** `\r\n`|
|http|Not applicable, one command per request.|Not applicable|

Over HTTP the response is translated into a status code: `200` with the value as the body, `403` for a permission error, `404` for a slot that is not configured, `503` when the node is not the cluster leader, and `400` for any other error. The `s`, `d`, `u`, `p` and `q` commands have no HTTP equivalent, so multicast groups cannot be joined over this transport.

The HTTP server is bounded so that a slow or idle client cannot hold resources for long: a client has 5 seconds to send its request headers and 10 seconds for the whole request, responses are written within 10 seconds, an idle keep-alive connection is closed after 60 seconds, and request headers are capped at 8 KiB. Every command is a single short request, so well-behaved clients never notice these limits.

SSE streams are the deliberate exception: a stream stays open for as long as the client keeps reading it, with no read or write timeout. In exchange the stream follows two rules:

- **Heartbeats.** When nothing has been written to a stream for 15 seconds the server sends an SSE comment line (`: keepalive`), which SSE clients ignore. The check runs every 15 seconds, so a quiet stream carries a comment at least every 30 seconds and a busy one carries none. It keeps proxies from closing an idle stream and lets the server notice a client that went away without closing the connection.
- **Backpressure.** A subscriber must keep reading. Each event is written with a 200 ms deadline, the same the `standard` transport uses, so once a subscriber's socket buffers are full because it stopped reading, the next event or heartbeat fails and the subscriber is disconnected. The `a` event it missed is counted as an error in the `received/sent/errors` response to the writer, exactly like a stalled TCP subscriber.

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

### Server settings

The configuration is a YAML file named `config.yaml`, looked up in `/etc/ghoti`, then `$HOME/.ghoti`, then the current directory. If no file is found, the server exits with an error, it does not start with defaults. The file is only read at start-up, so a change needs a restart, which also clears every slot.

|Config|Description|
|------|-----------|
|addr|`host:port` the server listens on. Default: `localhost:9090`. Can also be set with `ghoti run --addr`.|
|protocol|Transport: `standard` (default), `telnet` or `http`. See [Protocol variants](#protocol-variants).|
|max_connections|Maximum open client connections. Default: `0`, no cap. See [Connection limits](#connection-limits).|
|log.level|`debug`, `info` (default), `warn` or `error`.|
|log.format|`text` (default) or `json`.|

Settings with a single value can also be set from the environment with the `GHOTI_` prefix, upper case and with `.` replaced by `_`, for example `GHOTI_ADDR`, `GHOTI_LOG_LEVEL` or `GHOTI_CLUSTER_PASS`. The environment overrides the file. Use this to keep credentials out of the file, see [SECURITY.md](SECURITY.md). Slots are only read from the file.

Slots are configured as `slot_000` to `slot_999`, each with a `kind` and the options of that kind, described below.

**An invalid slot definition is skipped silently.** If a slot has an unknown `kind`, a missing required option or an out-of-range value, the server still starts, logs nothing about it, and that slot answers every command with error `005` (MISSING_SLOT), as if it was not configured. After changing the configuration, read each slot once to check that it exists.

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

### Multicast signal propagation

Similar to the Broadcast slot but this slot allows to send a message to a specific group of clients, instead of every connected client.

Unlike the Broadcast slot, a client has to join the group before it can receive events. This is done with the `s` (subscribe) command described in the Protocol section. A client that no longer wants to receive events can leave the group with the `d` (deregister) command. The response to `s` and `d` says whether the client is now a member: a non-zero value means it is registered, zero means it is not.

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

### Random signal propagation (planned, not implemented)

This slot kind does not exist yet: a slot configured with it is skipped, see [Server settings](#server-settings). The plan is a slot that works like the Multicast slot, but sends each message to only one of the registered clients, picked pseudo-randomly, with the same `s`/`d` commands and the same `timeout` and `dereg_tries` options.

### Ticker (watchdog)

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

### Connection limits

By default Ghoti accepts as many client connections as the operating system allows. `max_connections` caps how many may be open at once, on every transport:

```yaml
max_connections: 1000
```

When the cap is reached, new connections are not refused: they complete the TCP handshake and wait in the kernel's accept queue until a connection closes, so a client sees a connect that succeeds and a first response that is delayed. Over `http` this also counts SSE subscribers, since each stream holds a connection for as long as it is open. A value of `0`, the default, means no cap, and a negative value is a configuration error.

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

The server will respond with the `v` value returning the username of the logged in user or `e` if there is an error.

> **The password travels as cleartext.** Ghoti does not implement TLS on any
> of its listeners, so the `p` command sends the password in the clear and
> anybody observing the traffic can read it. Terminate TLS in a reverse proxy
> in front of the server, or keep the traffic on loopback or an equivalent
> trusted transport. See [SECURITY.md](SECURITY.md) for the full security
> model and for how to inject credentials from the environment instead of
> committing them to a configuration file.

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

Ghoti can optionally expose runtime metrics for [Prometheus](https://prometheus.io/) to scrape. When enabled, a dedicated HTTP listener serves `GET /metrics` in the Prometheus text exposition format. Instrumentation uses lock-free counters, so it adds negligible overhead to request handling, and is disabled by default.

To enable it, add a `metrics:` section to the configuration:

```yaml
metrics:
  enabled: true
  addr: "127.0.0.1:9100"   # host:port where GET /metrics is served
```

|Config      |Description                                                          |
|------------|----------------------------------------------------------------------|
|enabled     |Enables metrics collection and the `/metrics` endpoint. Default: false.|
|addr        |Address (`host:port`) the metrics HTTP server listens on. Required when enabled.|

The endpoint is unauthenticated, so bind it to a private interface or firewall it as you would any Prometheus target.

Point Prometheus at it with a standard scrape job:

```yaml
scrape_configs:
  - job_name: ghoti
    static_configs:
      - targets: ["127.0.0.1:9100"]
```

The following Ghoti-specific metrics are exposed, in addition to the standard Go runtime (`go_*`) and process (`process_*`) collectors:

|Metric                                  |Type      |Description                                                  |
|-----------------------------------------|----------|--------------------------------------------------------------|
|`ghoti_connected_clients`                 |gauge     |Number of currently connected clients.                        |
|`ghoti_requests_total`                    |counter   |Total number of requests processed. Use `rate()` for requests per second.|
|`ghoti_request_duration_seconds`          |histogram |Request processing duration in seconds, with buckets from 50µs to 1s. Use `histogram_quantile()` for tail latency.|

For example, requests per second and p99 latency over the last five minutes:

```
rate(ghoti_requests_total[5m])
histogram_quantile(0.99, rate(ghoti_request_duration_seconds_bucket[5m]))
```

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

## Contributing

Issues and pull requests are welcome on [GitHub](https://github.com/dankomiocevic/ghoti). Building needs the Go version listed in `go.mod`. `make help` lists every target, the main ones are:

```sh
make build   # binary in .dist/
make test    # all tests
make lint    # golangci-lint with .golangci.yaml
```

Pull requests run the same lint and test checks in CI. Report security issues privately as described in [SECURITY.md](SECURITY.md), not as public issues. Planned work is listed in [Scope and maturity](#scope-and-maturity).

## License

Ghoti is licensed under the Apache License 2.0, see [LICENSE](LICENSE).
