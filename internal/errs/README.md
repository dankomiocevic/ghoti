# Ghoti error codes

This package contains the list of error codes that Ghoti uses to identify failures on commands sent.
The decision to use error codes instead of a more verbose message is related to the core values of Ghoti, more specifically the latency and throughput requirements. This makes all the messages an responses very small.

# How error messages work

The error messages will contain the error command `e` followed by 3 digits specifying the slot and three more digits to include the error code.
For example, for slot 135 and error 006, you would see:

```
e135006
```

Some cases are not related to a specific slot like for example error 001 that is a parsing error. In that case, instead of a slot number, you will see `xxx` as the slot number:

```
exxx001
```

It could also contain extra arguments if something is important to share with the client. 
For example, when Ghoti is configured to work in a cluster it has a leader node and peer nodes. The only node that can answer to commands is the leader node, then if you try to send a write command on a peer node, it will return an error.
In this case, the error will be followed by the name of the node that is the leader:

```
exxx000node1
```

This way the client can identify which node is the one that it should be contacting to execute commands.

# Error codes list

This is the main list of error codes, but, be aware that this is not only a documentation, it is the actual source of truth for the error codes.

Every error code that has a section on this README will be statically loaded from the application and used as a reference.
This is done because error codes are very important and maintaining a documentation and the code in sync is always a source of issues.

Because of this, the Go code that returns the codes for the errors is reading the information directly from this file in compile time.

Each following subsection represents an error code, the code is followed by the error name and the description.

## 000: NOT_LEADER

This node is not a leader, please contact the leader node to execute commands.

When running in cluster mode, the only node that can be used to read and write is the leader node.
When the client tries to write on a peer/follower node, the node will return this error to notify that it cannot execute the command and that the command should be sent to the leader node.
The error also contains the name of the leader node.

```
exxx000nodeA
```

This example shows that the `nodeA` is the node that should be contacted instead. Depending on how the cluster was created and how the client was configured, this information will be available for the client to identify the correct address for the node.

## 001: PARSE_ERROR

Error when parsing the message.

This means that the received message does not follow the communication protocol. Please review the main README or Documentation to identify valid messages. The cases that produce it are:

- The message is not terminated correctly for the transport in use: a missing `\n` on `standard`, or a missing `\r\n` on `telnet`.
- The message is shorter than 4 bytes, `q` excepted.
- The message is longer than 40 bytes, not counting the terminator. This is measured in **bytes**, so a value of multi-byte characters reaches the limit sooner than its character count suggests.
- The command byte is not one of the supported commands.
- The three bytes after the command are not a decimal number, for commands that address a slot.

This error is never associated with a slot, so it is always returned as `exxx001`.

## 002: WRONG_USER

The user is too short or contains invalid characters.

This could be because the username is shorter than 4 bytes, or that the username contains special characters. The username must start with a letter and can only continue with letters, numbers or underscore, which is the regular expression `^[a-zA-Z][a-zA-Z0-9_]*$`.

The connection is closed after this error.

## 003: WRONG_PASS

The password is too short.

This could be because the password sent is shorter than 4 bytes, or because there was an invalid username defined before.

The connection is closed after this error.

## 004: WRONG_LOGIN

There is no username and password matching.

There is no user and password in the configuration that matches the login information being sent.

The connection is closed after this error.

## 005: MISSING_SLOT

The slot addressed by the command is not configured.

The slot number is syntactically valid and within range, but no slot of that number is defined in the configuration. Unlike the login errors, this response carries the requested slot number rather than `xxx`, for example `e999005`.

## 006: WRITE_PERMISSION

The user does not have permission to write in this slot.

The requested slot doesn't have write permissions enabled for the current logged in user. If there is no logged in user, then the slot has not open-write permissions.

## 007: WRITE_FAILED

The write operation on this slot failed.

Depending on the kind of slot, the write operation can fail because of multiple reasons. The ones currently returned are:

- On a `timeout_memory` slot, the writing connection is not the owner and the owner's timeout has not expired yet.
- On an `atomic` or `ticker` slot, the value written is not a non-negative decimal integer.

Note that this is not a permission error: `006` means the logged in user is not allowed to write the slot at all, while `007` means the write was allowed but the slot rejected it.

## 008: READ_PERMISSION

The user does not have permission to read in this slot.

The requested slot doesn't have read permissions enabled for the current logged in user. If there is no logged in user, then the slot has not open-read permissions.

## 009: WRONG_FORMAT

The message sent does not have the right format.

The message that was received does not match the expectations for a valid message, for more information please refer to the documentation or the repository README to check the message formatting rules.

This code is reserved and is not currently returned by the server: malformed messages are reported as `001` instead. It is kept so that the code stays stable if the two cases are ever separated.

## 010: UNSUPPORTED_COMMAND

The command sent is not supported by this slot.

The `s` (subscribe) and `d` (deregister) commands are only supported by slots that manage a group of clients, such as the multicast slot. Sending them to any other kind of slot returns this error.
