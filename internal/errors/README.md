# Ghoti error codes

This package contains the list of error codes that Ghoti uses to identify failures on commands sent.
The decision to use error codes instead of a more verbose message is related to the core values of Ghoti, more specifically the latency and throughput requirements. This makes all the messages an responses very small.

# How error messages work

The error messages will contain the error command `e` followed by 3 digits specifying the error code.

```
e001
```

It could also contain extra arguments if something is important to share with the client. 
For example, when Ghoti is configured to work in a cluster it has a leader node and peer nodes. The only node that can answer to commands is the leader node, then if you try to send a write command on a peer node, it will return an error.
In this case, the error will be followed by the name of the node that is the leader:

```
e000node1
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
e000nodeA
```

This example shows that the `nodeA` is the node that should be contacted instead. Depending on how the cluster was created and how the client was configured, this information will be available for the client to identify the correct address for the node.

## 001: PARSE_ERROR

Error when parsing the message.

This means that the received message does not follow the communication protocol. Please review the main README or Documentation to identify valid messages.

## 002: WRONG_USER

The user is empty or contains invalid characters.

This could be because the username was empty or that the username contains special characters. The username can only contain letters, numbers or underscore.

## 003: WRONG_PASS

The password is empty.

This could be because the password sent is an empty string or because there was a invalid username defined before.

## 004: WRONG_LOGIN

There is no username and password matching.

There is no user and password in the configuration that matches the login information being sent.

## 005: MISSING_SLOT

There is no slot specified in the command.

The command was sent with an invalid slot defined.

## 006: WRITE_PERMISSION

The user does not have permission to write in this slot.

The requested slot doesn't have write permissions enabled for the current logged in user. If there is no logged in user, then the slot has not open-write permissions.

## 007: WRITE_FAILED

The write operation on this slot failed.

Depending on the type of slot, the write operation can fail because of multiple reasons.

## 008: READ_PERMISSION

The user does not have permission to read in this slot.

The requested slot doesn't have read permissions enabled for the current logged in user. If there is no logged in user, then the slot has not open-read permissions.

## 009: WRONG_FORMAT

The message sent does not have the right format.

The message that was received does not match the expectations for a valid message, for more information please refer to the documentation or the repository README to check the message formatting rules.

The user does not have permission to read in this slot.

The requested slot doesn't have read permissions enabled for the current logged in user. If there is no logged in user, then the slot has not open-read permissions.
