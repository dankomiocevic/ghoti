# Protocol

[Ghoti](../README.md) reference: [Protocol](protocol.md) · [Configuration](configuration.md) · [Authentication](authentication.md) · [Metrics](metrics.md) · [Cluster](../cluster.md)

Ghoti uses slots to communicate, if you ever worked with microcontrollers you would get the similarities with registers.
The idea is that you can either write or read a slot. Slots hold at most 36 bytes of data and go from slot #0 to slot #999.

All messages are plain text in order to simplify the protocol among different programming languages.

## Framing and limits

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

## Commands

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

To identify the error code, the list of error codes can be found [here](../internal/errs/README.md).

In some cases there are messages sent as async events from the server (see [broadcast slots](configuration.md#broadcast-signal-propagation)), these kind of messages are sent at any time and use the `a` (async) response:

`a2346396A64C-1C2C-4BFC-B8F1-034758018CAC`

Same as the other examples, it would contain the `a` response, then the slot (in this case 234) and the event data (in this case a UUID).

NOTE: Async events can happen at any time.

Some slots manage a group of clients that should receive their events, instead of all the connected clients (see [multicast slots](configuration.md#multicast-signal-propagation)). To join that group a client can use the `s` (subscribe) command, and to leave it the `d` (deregister) command, both followed by the three digits identifying the slot:

`s005`

The response works the same way as a read, a non-zero value means the client is now part of the group:

`v0051`

`d005`

`v0050`

Sending `s` or `d` to a slot that does not support groups of clients returns error `010`.

A client can close its own connection cleanly with the `q` (quit) command. It takes no slot number and no arguments, so the request is exactly one byte and anything after it is rejected with `exxx001`. The server does not answer it, it just closes the connection:

`q`

## Command summary

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

## Protocol variants

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
