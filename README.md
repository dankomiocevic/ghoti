# ![Logo](images/logo.png) Ghoti

[![Go Reference](https://pkg.go.dev/badge/github.com/dankomiocevic/ghoti.svg)](https://pkg.go.dev/github.com/dankomiocevic/ghoti)
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Fdankomiocevic%2Fghoti.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2Fdankomiocevic%2Fghoti?ref=badge_shield)
[![Go Report](https://goreportcard.com/badge/github.com/dankomiocevic/ghoti)](https://goreportcard.com/report/github.com/dankomiocevic/ghoti)
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
The idea is that you can either write or read a slot. Slots cannot have more than 36 characters of data and go from slot #0 to slot #999.

All messages are plain text in order to simplify the protocol among different programming languages.

In order to read a slot you can send a read request. That would be the command `r`, then three digits defining the slot number .

`r000`

This will trigger a value response with the information about the slot. The response `v` indicates is a value response, then three digits to determine the slot and up to 36 characters to define the value.

`v0006396A64C-1C2C-4BFC-B8F1-034758018CAC`

In this example, the slot has a UUID stored.

When a client wants to write a value on a slot, they can use the `w` command:

`w000HelloWorld`

This will write the value `HelloWorld` on the slot `000`. The value can be any string with a maximum of 36 characters.
Same as the read command, the server will return the written value:

`v000HelloWorld`

If there is any issue with a command, the server will return an error with a code that can be used to identify the issue:

`e000009`

In this case, the error code has 3 parts:
- `e` indicates is an error response.
- `000` is the slot number.
- `009` is the error code.

In the case of commands that are not related to a specifc slot, the slot number will be "xxx". For example, if you want to login, the command to enter the password would be `p` and it won't be related to any slot. The response when the password is empty would be:

`exxx003`

Where `e` indicates is an error response, `xxx` is the slot number and `009` is the error code.:w

To identify the error code, the list of error codes can be found [here](internal/errors/README.md).

In some cases there are messages sent as async events from the server (see broadcast slots), these kind of messages are sent at any time and use the `a` (async) response:

`a2346396A64C-1C2C-4BFC-B8F1-034758018CAC`

Same as the other examples, it would contain the `a` response, then the slot (in this case 234) and the event data (in this case a UUID).

NOTE: Async events can happen at any time.

### Protocol variants

The core protocol is always the same despite the variant selected, but there are different options to use as a transport layer. The following are the available options:
- standard: The protocol works as described in the previous section, it is a plain TCP connection that requires messages to be sent in plain text and terminated with a newline character. This is the default option.
- telnet: This option is the same as the standard option but it allows the use of the telnet protocol to connect to the server. This option is useful when you want to use a telnet client to connect to the server. The main difference is that the messages are terminated with a return of carriage and a newline character, as specified in the standard telnet protocol.

Example config:

```yaml
protocol: telnet
```

## Configuration 

Ghoti has 1000 configurable slots that can be used to provide different functions.
Slots are configured through configuration files, if a slot configuration changes Ghoti cannot enforce consistency in the data until the new configuration is propagated.
Clients must know the configuration beforehand in order to use the slots appropriately.

For example, the same Ghoti server can be configured to have the first 3 slots as rate limiters and the next two as multicast signal propagation slots.
This way the applications can use a single server to solve more than one problem. I mean, is already there!

### Simple memory slot

This is the most basic slot where a value can be stored. The value has a maximum of 36 characters. You can read and write on the value and there are no restrictions.
This slot has also no configuration.

Example config:

```yaml
slot_000:
  type: simple_memory
```

### Timeout memory slot

This slot is also a memory slot but the main difference with the Simple memory slot is that it has an owner. Only the client that has last written in this slot can write again.
If the owner does not write on this slot for a certain time (timeout), it will lose the ownership and any other client can take over.

The timeout can be configured:

|Config      |Value                               |
|------------|------------------------------------|
|timeout     |Timeout value configured in seconds.|

All clients can read from this slot, but only the owner can write. If any other client tries to write it will fail. If there is no owner, the first client that writes becomes the owner.

Example config:
```yaml
slot_001:
  type: timeout_memory
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
  type: token_bucket
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
  type: leaky_bucket
  bucket_size: 100
  refresh_rate: 1000
```

### Broadcast signal propagation

Anything sent to this slot is propagated as a message to all the other clients. Any client connected to Ghoti at this point will receive the event at least once.
This means that the message could be received more than once.

The message is sent as an async event, the receiving client will receive the message at any time.
The message format is the following:

```
>a000HelloWorld
```

Where `a` is the async event, `000` is the slot number and `HelloWorld` is the message.

The message to be sent has a maximum of 36 characters, this allows to send an ID or a UUID to all the hosts.

This kind of slot is used to notify other clients about a new event or to propagate a signal.

There is no configuration for this slot.

This slot will only acknowledge the command when all the messages are sent, so take into account that the more clients connected or the hardest those clients are to reach, it will delay the confirmation. The confirmation contains the following information:

```
v000a/b/c
```
Where:
- `a` is the number of clients that received the message.
- `b` is the number of clients that are connected.
- `c` is the number of failures.

Example:
```
>w001HelloWorld
<a001HelloWorld
<v0003/5/2
```
This means that the message was sent to 5 clients, 3 received it and 2 failed.

Writes will propagate the written value to all other clients. Reads will read the last written value.

### Multicast signal propagation (TBD)

Similar to the Broadcast slot but this slot allows to send a message to a specific group of clients. This type of multicast **requires 2 consecutive slots:**
- Register/Deregister: This slot allows a client to register or deregister from the multicast. If the client is registered, it will receive the events. To register a client can write a value on this slot, to deregister it can write zero. If a client reads this slot, then a non-zero value means the client is already registered and a zero value means is not.
- Message: This will send a message the same way as the Broadcast slot but it will only send it to registered clients. Writes will propagate the written value to all other clients. Reads will read the last written value.

|Config          | Description |
|----------------|-------------|
| timeout        | Time to wait for a confirmation on the clients that the message was received. |
| dereg_tries    | Number of messages that are tried on a client until is de-registered. |


### Random signal propagation (TBD)

This signal propagation slot works like the Multicast signal propagation explained before but with a major diference, the message is not sent to all registered clients, but only one. It uses a pseudo-random generator to distribute the messages among the clients.

It also **requires 2 slots**:
- Register/Deregister: This slot allows a client to register or deregister from the multicast. If the client is registered, it will receive the events. To register a client can write a value on this slot, to deregister it can write zero. If a client reads this slot, then a non-zero value means the client is already registered and a zero value means is not.
- Message: This will send a message the same way as the Broadcast slot but it will only send it to registered clients. Writes will propagate the written value to all other clients. Reads will read the last written value.

It has the same configuration as the previous slot:

|Config          | Description |
|----------------|-------------|
| timeout        | Time to wait for a confirmation on the clients that the message was received. |
| dereg_tries    | Number of messages that are tried on a client until is de-registered. |


### Ticker (watchdog)

This is a classic slot used in embedded circuits and microcontrollers, the slot contains an integer value, the way this works is that the slot will tick once a second making its value go down by one until it reaches zero.
If any client writes to this slot and sets a value (integer value), it will start decrementing that value once a second until it reaches zero again.

In other words, if a client writes `600` on this slot, then waits 9 minutes and reads the value, the value will be `60`. After one more minute, the value will be zero.

|Config          | Description |
|----------------|-------------|
| initial_value  | Initial value for the ticker. Default: 0 |
| refresh_rate	 | The number of milliseconds per tick. Default: 1000 |

Example config:

```yaml
slot_003:
  type: ticker
  initial_value: 600
  refresh_rate: 1000
```

### Atomic counter slot

This slot contains an integer number and allows to increment or decrement its value. Only one process can increment or decrement the value at a time.

To increment, you need to write a positive integer number, to decrement, a negative integer number.

There is no configuration needed for this slot.

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

## Cluster configuration (Experimental)

Ghoti clusters are created to increment availability, they are not supposed to propagate information to other nodes in order to increase data persistence. When a cluster node fails, another node will take its place but it will start on a clean state without keeping track of the information stored before.

Ghoti does not do replication because it affects performance, and Ghoti does not persist data so there is no real reason to replicate data in the cluster.

In order to simplify the implementation and having a working version, I am using a [RAFT algorithm implementation](https://github.com/hashicorp/raft). But, I am not really happy with the outcome because I am not using most of the features like replication and I would like to have a 2 node cluster as the suggested approach.
I am planning on replacing this algorithm with a simpler one.

Why I want a 2-node cluster as default? Because Ghoti only needs to have redundancy for availability purposes, it doesn't need replication. These algorithms have a lot of extra features that are not really needed. I am using this implementation because I know it is stable and I want to be sure the rest of the software is fully tested before working on this.

But why 2? Because there are two main reasons to use the cluster mode:
- One of the main reasons is to be able to perform deployments with minimal downtime. So you can replace one node, then convert it to leader and replace the other one. That generates minimal impact.
- If there is an issue with one of the nodes, the other one can take over. 

If there is something really bad happening (like an issue during a deployment), then the only impact is an increased downtime. If this increased downtime needed to enable a new node is too high, then you can add a third node.

# Next steps

This list is not exhaustive, but it is a good starting point to understand what is missing and what is planned for the future.
Here are some of the things that are planned for the future:
- Add support for WebSockets.
- Replace the Cluster implementation with a simpler one.
- Add metrics and monitoring (Prometheus, OpenTelemetry, etc).
- Implement missing slots.
- Add benchmark for the performance of the slots.
- Add docker support.

# License

This software is using the Apache-2.0 license.

[Complete license](LICENSE.md)
