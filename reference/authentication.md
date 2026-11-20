# Authentication

[Ghoti](../README.md) reference: [Protocol](protocol.md) · [Configuration](configuration.md) · [Authentication](authentication.md) · [Metrics](metrics.md) · [Cluster](../cluster.md)

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
> trusted transport. See [SECURITY.md](../SECURITY.md) for the full security
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
