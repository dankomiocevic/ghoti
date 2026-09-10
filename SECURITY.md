# Security Policy

## Reporting a Vulnerability

Please report security vulnerabilities **privately**. Do not open a public
issue, and do not disclose the problem in a pull request, a discussion or any
other public channel before a fix is available.

Use GitHub's private vulnerability reporting to open a report:

<https://github.com/dankomiocevic/ghoti/security/advisories/new>

Include as much of the following as you can:

- The version, commit or tag affected.
- The configuration needed to reproduce it, with any credential replaced.
- A description of the impact and, if you have one, a proof of concept.
- Whether the issue is already public anywhere.

You should get an acknowledgement within 7 days. Once the report is
confirmed, a fix and an advisory are prepared together and you are credited
in the advisory unless you prefer to stay anonymous.

Please do not include real credentials, production log excerpts or packet
captures that carry secrets in a report. Redact them first.

## Supported Versions

Security fixes are applied to the latest released version. There is no
backport of fixes to older releases.

## Security Model

Ghoti is designed to run **inside a trusted network boundary**. Understanding
what it does and does not protect is necessary to deploy it safely.

### There is no transport encryption

None of the listeners implement TLS:

| Listener | Protocol | Encryption |
|---|---|---|
| Server (`addr`) | Ghoti TCP / Telnet | none |
| Server (`addr`, `protocol: http`) | HTTP and SSE | none |
| Cluster manager (`cluster.manager.addr`) | HTTP with Basic auth | none |
| Cluster bind (`cluster.bind`) | inter node traffic | none |

This has direct consequences:

- The `p` command sends the password as **cleartext bytes** on the wire.
- Cluster traffic authenticates with **HTTP Basic**, which is only a Base64
  encoding of the shared credentials and offers no confidentiality
  ([RFC 7617](https://datatracker.ietf.org/doc/html/rfc7617)). It is not
  secure without TLS.

Anybody able to observe the traffic can recover every credential. This
includes passive capture on the network path, a load balancer terminating
connections in front of Ghoti, and any hop between nodes of a cluster.

### Deploying safely

**TLS is a mandatory part of any deployment that is not entirely local.**
Ghoti does not provide it, so it has to be provided around it:

- Terminate TLS in a reverse proxy or a service mesh sidecar in front of
  every listener, and make sure the plaintext hop between the proxy and
  Ghoti stays on loopback.
- For inter node cluster traffic, use mutual TLS between the nodes, a VPN,
  or an equivalent encrypted transport. The cluster credentials are a shared
  secret for the whole cluster, so a single observed request compromises
  every node.
- Never expose a plaintext listener to an untrusted network.

### Listeners bind to loopback by default

The defaults are deliberately closed:

| Setting | Default |
|---|---|
| `addr` | `localhost:9090` |
| `cluster.bind` | `localhost:25873` |

Changing these to a routable address exposes an unencrypted service. Only do
it behind one of the encrypted transports described above, and prefer binding
to a specific private interface over `0.0.0.0`.

### The `/leader` endpoint is unauthenticated

`cluster.leader.enabled` serves `GET /leader` without authentication, because
load balancer health checks cannot send credentials. It discloses whether the
node is the leader and the current leader's node ID. It is opt-in and should
only be reachable from the load balancer.

## Credentials and Logging

Credential values are never written to the logs. Specifically:

- The message parser redacts the value of the `u` and `p` commands before
  logging the received message, so a password is never logged even at the
  `debug` level.
- A failed cluster authentication logs the remote address only. It never logs
  the supplied credentials, and never the configured ones.

These properties are covered by tests in `internal/server/redaction_test.go`
and `internal/cluster/redaction_test.go`. Any change that logs a credential
value should fail them. When adding a log line that touches authentication,
log the identifier and the remote address, never the secret.

## Keeping Credentials Out of the Repository

Ghoti reads its configuration with [Viper](https://github.com/spf13/viper).
Every configuration key can also be supplied as an environment variable
prefixed with `GHOTI_`, with dots and dashes replaced by underscores, so a
credential does not have to be written into a YAML file that risks being
committed:

```sh
export GHOTI_CLUSTER_USER=cluster_node
export GHOTI_CLUSTER_PASS="$(cat /run/secrets/ghoti_cluster_pass)"
```

The surrounding structure still has to exist in the configuration file, the
environment supplies the value. One exception: the `users` block is read as a
map, and map entries cannot be injected from the environment, so per user
passwords still have to come from the configuration file. Keep that file
outside the repository.

Prefer, in this order:

1. A secret manager that injects the value into the environment or into a
   file mounted at runtime.
2. Environment variables, as shown above.
3. A configuration file **outside** the repository, owned by the service user
   and with `0600` permissions.

If you must use a configuration file, make sure it is listed in
`.gitignore` and never committed. A credential that reached a git history has
to be treated as compromised and rotated, removing it from the history is not
enough.
