# ![Logo](images/logo.png) Ghoti

[![Go Reference](https://pkg.go.dev/badge/github.com/dankomiocevic/ghoti.svg)](https://pkg.go.dev/github.com/dankomiocevic/ghoti)
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Fdankomiocevic%2Fghoti.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2Fdankomiocevic%2Fghoti?ref=badge_shield)
[![Codecov](https://img.shields.io/codecov/c/github/dankomiocevic/ghoti)](https://app.codecov.io/gh/dankomiocevic/ghoti)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/dankomiocevic/ghoti/badge)](https://securityscorecards.dev/viewer/?uri=github.com/dankomiocevic/ghoti)

**How much of your code exists only to make your servers agree with each other?**

Shared rate limits, a single active worker, counters, "deploy now" signals: Ghoti keeps that state in one small server you can talk to with `nc`. It serves over 134,000 reads per second on a 2-core t4g.micro, and forgets everything on restart, by design.

Examples and the full documentation are at [ghoti.miocevic.com](https://ghoti.miocevic.com).

## What you can use it for

- **One rate limit shared by many instances.** Every API server takes tokens from the same [token bucket](reference/configuration.md#token-bucket-limiter) or [leaky bucket](reference/configuration.md#leaky-bucket-limiter) slot, so the limit holds no matter how many instances are running.
- **Keeping one worker in charge.** A [timeout memory](reference/configuration.md#timeout-memory-slot) slot only accepts writes from the connection that holds it. The holder renews it by writing again; once it stops, another client can take over after the timeout. Ownership is forgotten if Ghoti restarts and there is no fencing token, so treat it as a best-effort lease, not a lock that protects data.
- **Shared sequence numbers.** Every read of an [atomic counter](reference/configuration.md#atomic-counter-slot) increments it and returns the new value, which works for sequence numbers, job IDs or shared counts. The counter starts again from zero after a restart.
- **Pushing a signal to running processes.** A write to a [broadcast](reference/configuration.md#broadcast-signal-propagation) slot is sent to every connected client. A [multicast](reference/configuration.md#multicast-signal-propagation) slot sends it only to clients that subscribed. Browsers can receive broadcasts over Server-Sent Events. Delivery is at most once.

There are also a [simple memory](reference/configuration.md#simple-memory-slot) slot for short shared values and a [ticker](reference/configuration.md#ticker-watchdog) slot that counts down, which works as a watchdog.

## Performance

Ghoti v0.3.0 benchmarked with [ghoti-bench](https://github.com/dankomiocevic/ghoti-bench), reading a 36-byte value from a simple memory slot:

|Connections|Requests per second|p50|p99|p99.9|Server CPU|
|--:|--:|--:|--:|--:|--:|
|1|3,359|0.290 ms|0.372 ms|0.597 ms|6%|
|100|134,343|0.600 ms|3.541 ms|7.123 ms|82%|
|1,000|112,647|8.376 ms|18.694 ms|26.231 ms|89%|

- **Hardware.** The server is an AWS t4g.micro (2 Graviton2 cores, 1 GiB). The load comes from a c7g.xlarge in the same subnet in us-west-2.
- **Workload.** Every connection stays open and keeps exactly one request in flight, as the protocol requires. The 100 and 1,000 connection rows are the median of 3 runs of 5 minutes each.
- **What the latency includes.** Latency is the round trip measured on the client, including the network between the two instances. Ghoti's own histogram, which excludes the network, averages 7.6 µs per memory read.
- **1,000 connections.** At 1,000 connections the server CPU is close to saturated, so most of the latency is time spent waiting for a turn.
- **Other slot kinds.** Atomic counters and token buckets sustain the same load, even when every connection uses the same slot: 127,594 to 139,552 requests per second at 100 connections, and 110,026 to 113,173 at 1,000.
- **Correctness.** All 21 runs finished with no errors, and every counter and token bucket returned exactly what it should have.

The [full report](https://github.com/dankomiocevic/ghoti-bench/blob/main/RESULTS.md) has every scenario, the correctness checks and the commands to reproduce the runs. Expect a different round trip on other networks; the time Ghoti spends inside the server does not depend on the network.

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

More info in [Ghoti overview])(https://ghoti.miocevic.com/docs/)

## Documentation

The website at [ghoti.miocevic.com](https://ghoti.miocevic.com) has examples and the documentation in one place. The same reference documentation also lives in this repository, in [`reference/`](reference/):

- <a name="protocol"></a><a name="framing-and-limits"></a><a name="commands"></a><a name="command-summary"></a><a name="protocol-variants"></a>**[Protocol](reference/protocol.md):** [framing and limits](reference/protocol.md#framing-and-limits), which commands are [safe to retry](reference/protocol.md#framing-and-limits), [commands](reference/protocol.md#commands), the [command summary](reference/protocol.md#command-summary) and the [transports](reference/protocol.md#protocol-variants): `standard`, `telnet` and `http` with Server-Sent Events.
- <a name="configuration"></a><a name="server-settings"></a><a name="simple-memory-slot"></a><a name="timeout-memory-slot"></a><a name="token-bucket-limiter"></a><a name="leaky-bucket-limiter"></a><a name="broadcast-signal-propagation"></a><a name="multicast-signal-propagation"></a><a name="ticker-watchdog"></a><a name="atomic-counter-slot"></a><a name="connection-limits"></a>**[Configuration](reference/configuration.md):** [server settings](reference/configuration.md#server-settings) and environment variables, every slot kind ([simple memory](reference/configuration.md#simple-memory-slot), [timeout memory](reference/configuration.md#timeout-memory-slot), [token bucket](reference/configuration.md#token-bucket-limiter), [leaky bucket](reference/configuration.md#leaky-bucket-limiter), [broadcast](reference/configuration.md#broadcast-signal-propagation), [multicast](reference/configuration.md#multicast-signal-propagation), [ticker](reference/configuration.md#ticker-watchdog), [atomic counter](reference/configuration.md#atomic-counter-slot)) and [connection limits](reference/configuration.md#connection-limits).
- <a name="auth"></a>**[Authentication](reference/authentication.md):** users, the login commands and per-slot read/write permissions.
- <a name="metrics"></a>**[Metrics](reference/metrics.md):** the Prometheus endpoint and the metrics it exposes.
- <a name="cluster-configuration-experimental"></a><a name="routing-traffic-to-the-leader"></a>**[Cluster (experimental)](cluster.md):** leader election, configuration, [routing traffic to the leader](cluster.md#leader-endpoint-for-load-balancers) and [failure scenarios](cluster.md#failure-scenarios).
- **[Error codes](internal/errs/README.md)**, the [security model](SECURITY.md) and the [changelog](CHANGELOG.md).

To study how distributed coordination breaks, the [cluster failure scenarios](cluster.md#failure-scenarios) walk through leader crashes, network partitions and split-brain, and explain what each one costs a client.

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

More info on [Getting started](https://ghoti.miocevic.com/docs/getting-started/).

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
