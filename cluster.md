# Cluster Implementation

Ghoti clusters provide high availability through leader election. Only the leader node processes client commands; follower nodes reject requests and redirect clients to the current leader. Ghoti does **not** replicate data between nodes — when a node takes over as leader, it starts with a clean state.

This document describes the cluster architecture, configuration, and behavior under failure scenarios.

## Architecture Overview

### Bully Election Algorithm

Ghoti uses a [bully election algorithm](https://en.wikipedia.org/wiki/Bully_algorithm) for leader election. The core rule is simple: **the node with the highest lexicographic ID wins the election**.

The algorithm works as follows:

1. When a node detects the leader is unreachable (via heartbeat failure), it starts an election.
2. The node sends an election message to all peers with a **higher** ID.
3. If any higher-ID peer responds, the initiating node backs off and waits for that peer to declare itself leader.
4. If no higher-ID peer responds, the node declares itself leader and sends a coordinator message to all peers.
5. When a node receives a coordinator message, it accepts the sender as the new leader.
6. When a node receives an election message from a lower-ID peer, it responds OK and starts its own election (since it has a higher ID).

### Components

The cluster implementation consists of these components:

- **`Cluster` interface** — defines the contract: `Start`, `Join`, `Remove`, `IsLeader`, `GetLeader`, `Shutdown`.
- **`BullyCluster`** — implements the `Cluster` interface with bully election logic, peer tracking, heartbeat monitoring, and election coordination.
- **`MembershipManager`** — abstraction for cluster membership. Currently the only implementation is `joinServer`.
- **`joinServer`** — an HTTP server running on each node that handles join requests, node removal, election messages, coordinator announcements, and heartbeat checks.
- **`EmptyCluster`** — a no-op implementation used when clustering is disabled. Always reports itself as leader, allowing the server to operate in standalone mode.

### Communication

All inter-node communication happens over HTTP. Each node runs a `joinServer` that exposes the following endpoints:

| Endpoint        | Method | Purpose                                                      |
|-----------------|--------|--------------------------------------------------------------|
| `/join`         | POST   | Register a new node in the cluster                           |
| `/remove`       | POST   | Remove a node from the cluster (leader only)                 |
| `/election`     | POST   | Bully election message — "I am starting an election"         |
| `/coordinator`  | POST   | Leader announcement — "I am the new leader"                  |
| `/heartbeat`    | GET    | Health check — used by followers to verify the leader is up  |

All endpoints (except `/heartbeat`) require HTTP Basic Authentication with the cluster credentials.

### Leader-Only Operations

When a client sends a command to a follower node, the server responds with a `NOT_LEADER` error followed by the current leader's node ID. This allows clients to redirect their requests to the active leader.

## Configuration

Cluster configuration is provided via the Ghoti config file (YAML). The cluster section is optional — if omitted, the server runs in standalone mode.

### Configuration Fields

| Field                  | Required | Description                                                                 |
|------------------------|----------|-----------------------------------------------------------------------------|
| `cluster.node`         | Yes      | Unique node identifier (max 20 characters). Used for election comparison.   |
| `cluster.bind`         | No       | Bind address for the Ghoti data server. Defaults to `localhost:25873`.       |
| `cluster.user`         | Yes      | Username for inter-node authentication (min 4 characters).                  |
| `cluster.pass`         | Yes      | Password for inter-node authentication (min 4 characters).                  |
| `cluster.manager.type` | Yes      | Membership manager type. Currently only `join_server` is supported.         |
| `cluster.manager.addr` | Yes      | Address where this node's cluster management HTTP server listens.           |
| `cluster.manager.join` | No       | Address of an existing node to join. Leave empty to bootstrap a new cluster.|

### Example: Bootstrap Node (First Node)

```yaml
cluster:
  node: "node1"
  bind: "localhost:25873"
  user: "cluster_user"
  pass: "cluster_pass"
  manager:
    type: "join_server"
    addr: "localhost:2222"
```

When `cluster.manager.join` is not set, the node bootstraps as leader of a new single-node cluster.

### Example: Joining Node

```yaml
cluster:
  node: "node2"
  bind: "localhost:25874"
  user: "cluster_user"
  pass: "cluster_pass"
  manager:
    type: "join_server"
    addr: "localhost:2223"
    join: "localhost:2222"
```

The `join` field points to the manager address of any existing cluster member. On startup, the joining node sends a `/join` request and receives the current peer list and leader identity.

### Node ID and Election Priority

The node ID (`cluster.node`) directly determines election outcomes. Node IDs are compared **lexicographically** — the node with the highest string value wins. For example:

- `"node3"` beats `"node2"` beats `"node1"`
- `"z"` beats `"a"`
- `"node10"` beats `"node1"` (but also beats `"node9"` — be careful with numeric suffixes and lexicographic ordering)

Choose node IDs deliberately if you want predictable failover behavior.

## Cluster Lifecycle

### Starting a Cluster

1. Start the first node **without** `cluster.manager.join`. It bootstraps as leader.
2. Start additional nodes **with** `cluster.manager.join` pointing to any existing node's manager address.
3. Each joining node receives the full peer list and leader identity from the cluster.
4. The existing node notifies all other peers about the new node.

### Heartbeat Monitoring

Follower nodes check the leader's health every **2 seconds** by sending a GET request to the leader's `/heartbeat` endpoint. If the heartbeat fails (connection error or non-200 response), the follower starts a new election.

The heartbeat HTTP client has a **2-second timeout**, so a leader that is slow to respond will also trigger an election.

### Leader Failover

When the leader goes down:

1. One or more followers detect the heartbeat failure.
2. Each detecting follower starts a bully election.
3. The follower sends election messages to all peers with higher IDs.
4. If a higher-ID peer responds, the follower backs off.
5. The highest-ID reachable node declares itself leader and broadcasts a coordinator message.
6. All nodes update their leader reference.

### Graceful Shutdown

When `Shutdown()` is called on a node, it sets `isUp = false` (causing heartbeat to return 503), closes the join server, and waits for background goroutines to finish. If the leader shuts down gracefully, followers will detect the heartbeat failure and elect a new leader.

## Failure Scenarios

### Leader Crashes

**What happens:** Followers detect the leader's heartbeat failure within approximately 2-4 seconds (one heartbeat interval plus the HTTP timeout). A bully election starts and the highest-ID surviving node becomes the new leader.

**Impact:** Client requests to the crashed leader fail. Requests to follower nodes get a `NOT_LEADER` response with the old leader's ID, which is also unreachable. Once the election completes, followers point to the new leader and clients can reconnect. Data stored on the crashed leader is lost since Ghoti does not replicate data.

### Follower Crashes

**What happens:** The cluster continues operating normally. The leader does not actively monitor followers — it only tracks them in its peer list. Client requests continue to be served by the leader.

**Impact:** Reduced redundancy. If the leader subsequently fails, there are fewer nodes available for election. The crashed follower's entry remains in the peer list until explicitly removed via `/remove`.

### Network Partition — Full Split

**Scenario:** The network splits so that no nodes can communicate with each other.

**What happens:** Each follower detects heartbeat failure and starts an election. Since no higher-ID peers respond (they're unreachable), every node declares itself leader.

**Impact:** This is a **split-brain scenario**. Multiple nodes believe they are the leader and will accept client writes independently. Since Ghoti does not replicate data, each partition operates independently. When the network heals, the nodes will still believe they are leaders. This state persists until a heartbeat detects a leader conflict and triggers a new election, or until the cluster is manually reconfigured. **Data written during the split to non-surviving leaders will be lost.**

### Network Partition — Partial Split

**Scenario:** Some nodes can communicate with each other but not with the leader.

**What happens:** Nodes that cannot reach the leader will start elections among themselves. The highest-ID node in that partition becomes leader of that partition. Meanwhile, the original leader continues operating with whatever nodes can still reach it.

**Impact:** Similar to a full split — multiple leaders can exist simultaneously. Clients connected to different partitions will write to different leaders. When connectivity is restored, the bully algorithm will eventually resolve to a single leader (the one with the highest ID), but data written to the losing partition's leader is not transferred.

### Network Partition — Nodes Reachable by Clients but Not by Each Other

**Scenario:** Clients can reach all nodes, but the nodes cannot communicate with each other (inter-node network failure).

**What happens:** Every follower detects heartbeat failure and elects itself leader (since no higher-ID peers respond). All nodes become independent leaders.

**Impact:** This is the worst-case split-brain. Clients may write to any node, but the data is isolated to each node. When inter-node communication is restored, elections will resolve to a single leader, and data on the other nodes becomes inaccessible. Clients should be designed to handle `NOT_LEADER` responses and reconnect to the correct leader once the cluster stabilizes.

### Node Joins with a Higher ID

**Scenario:** A new node joins the cluster with an ID that is lexicographically higher than the current leader.

**What happens:** The new node joins as a follower and accepts the current leader. It does **not** immediately trigger an election — it simply adds the peers and sets the leader as told by the join response. The current leader remains unchanged.

**However**, if the current leader subsequently fails, the new higher-ID node will win the election and become the new leader. This is the expected bully algorithm behavior.

**Impact:** No disruption on join. The higher-ID node only takes over if an election occurs.

### Failing Node Rejoins with a Higher ID

**Scenario:** A node that was previously the leader crashes, and then rejoins the cluster with the same (or higher) ID.

**What happens:** When it rejoins, it contacts an existing node via `/join`, receives the current peer list and the current leader's identity, and becomes a follower. It does **not** automatically reclaim leadership just because it has a higher ID.

**However**, if any subsequent election occurs (e.g., the current leader becomes unreachable), the rejoined node will win the election due to its higher ID.

**Impact:** The rejoined node starts with a clean state — any data it held before crashing is lost. If it later becomes leader through an election, it serves requests with an empty state.

### All Nodes Restart Simultaneously

**Scenario:** All cluster nodes go down and come back up at the same time.

**What happens:** The node configured without `cluster.manager.join` (the bootstrap node) starts as leader. Other nodes attempt to join via their configured join address. If the bootstrap node is not yet ready when others try to join, those join requests will fail and the joining nodes will not start.

**Impact:** The cluster reforms, but all data is lost. Startup order matters — the bootstrap node must be available before joining nodes attempt to connect. There is no automatic retry on join failure.

### Heartbeat False Positive (Slow Leader)

**Scenario:** The leader is under heavy load and responds to heartbeat requests slowly (greater than 2 seconds).

**What happens:** Followers interpret the timeout as leader failure and start an election. If the leader is still technically alive, it may receive election or coordinator messages. The highest-ID node (which could still be the slow leader) will win the election.

**Impact:** Unnecessary leader elections cause brief disruption. If the slow leader wins re-election, the cluster returns to the same state after a short period of instability. During the election, client requests to followers may receive stale leader information.

### Authentication Mismatch

**Scenario:** A node tries to join with different `cluster.user` or `cluster.pass` credentials than the existing cluster.

**What happens:** All inter-node HTTP requests use Basic Authentication. The join request (and subsequent election/coordinator messages) will be rejected with a 400 status code. The node will fail to join the cluster.

**Impact:** The misconfigured node cannot participate in the cluster. No security breach occurs — the cluster simply rejects unauthenticated requests.

## Limitations

- **No data replication.** When a new leader is elected, it starts with an empty state. This is by design — Ghoti prioritizes availability and performance over data durability.
- **No automatic rejoin retry.** If a joining node cannot reach its configured join address, it fails immediately without retrying.
- **No split-brain detection.** The bully algorithm does not detect or resolve split-brain scenarios. When partitions heal, elections will eventually converge, but data loss is possible.
- **Lexicographic ID ordering.** Node IDs are compared as strings, not numbers. `"node10"` > `"node2"` is false (`"1" < "2"`), which can be surprising. Use zero-padded IDs (e.g., `"node01"`, `"node02"`) or consistent naming to avoid this.
- **No quorum requirement.** A single node can declare itself leader if it cannot reach any higher-ID peers. This makes the system available under partitions but vulnerable to split-brain.
- **Stale peer list.** Crashed nodes remain in the peer list until explicitly removed via the `/remove` endpoint on the leader. This means election messages are sent to unreachable nodes, adding latency to the election process.
- **Fixed timeouts.** The heartbeat interval (2 seconds), heartbeat timeout (2 seconds), and election timeout (3 seconds) are hardcoded and cannot be configured.
