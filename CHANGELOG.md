# Changelog

All notable changes to Ghoti are listed here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the versions
follow [Semantic Versioning](https://semver.org/).

The section for a version is used as the body of its GitHub release, so it has
to exist before the tag is pushed. The Release workflow fails otherwise.

## [Unreleased]

## [0.2.0] - 2026-09-19

### Upgrading from 0.1

- **Cluster nodes running 0.1 and 0.2 cannot form a cluster together.** The
  RAFT consensus was replaced with a bully election algorithm and the two
  protocols are not compatible. A cluster has to be stopped and started again
  with every node on 0.2, there is no rolling upgrade. Nothing is persisted,
  so there is no data to migrate.
- The `cluster.node` value now decides the election priority, the highest
  wins. Review the node IDs if you rely on a specific node being the leader.
- The `metrics` section changed. The old file based writer and its keys
  (`output_dir`, `rotation`, `retain`, `interval`) are gone, metrics are now
  served over HTTP for Prometheus at `metrics.addr`. A configuration that
  still has the old keys is rejected at start up so the change is not missed.
- The release archives changed name and format. Linux and macOS builds are
  now `ghoti_<version>_<os>_<arch>.tar.gz` and Windows builds are
  `ghoti_<version>_windows_<arch>.zip`, with the binary at the root of the
  archive instead of inside a folder. Update any script that downloads them.
- Every release now ships with `checksums.txt`, an SBOM per archive, a
  Sigstore signature and a build provenance attestation. See the
  Installation section of the README for how to verify a download.
- `ghoti version` reports the real version, commit and build date. The 0.1
  binaries reported `dev`.

### Added

- HTTP and SSE transport for browser clients, enabled with `protocol: http`
  (#75).
- Multicast slot that propagates a signal to a subset of the connected
  clients (#77).
- Prometheus metrics endpoint served at `metrics.addr` (#68, #94).
- `GET /leader` endpoint on the cluster manager, enabled with
  `cluster.leader.enabled`, so a load balancer can route traffic to the
  leader (#82).
- Message batching, several pending messages to the same client are sent in
  one write (#60).
- Checksums, SBOMs, Sigstore signatures and build provenance on every
  release, and a `CHANGELOG.md` that feeds the release notes.

### Changed

- Leader election uses a bully algorithm instead of RAFT. Ghoti only needs
  redundancy for availability, not log replication, so the simpler protocol
  is enough (#74).
- Error responses include the slot that produced them (#53).
- Viper upgraded, `hashicorp/raft` removed and `prometheus/client_golang`
  added as dependencies (#79).
- Go 1.27 is required to build.

### Fixed

- A node could loop forever while joining a cluster (#90).
- Race conditions on broadcast and on connection close (#88).
- The atomic counter used a read lock where a write lock was needed (#87).
- The slot number parser accepted negative slot IDs (#86).
- Empty messages were not handled (#84).
- The token bucket did not add the tokens correctly on each period (#92).
- Slots use a read-write mutex, reads no longer block each other (#65).
- A failure while starting the server, like an address already in use, is
  now reported instead of ignored (#93).
- The cluster join server did not stop, `Shutdown` was called on a copy of
  the `http.Server` (#55).

### Security

- Passwords are no longer written to the logs, even at `debug` level. The
  `u` and `p` commands are redacted before the message is logged and a failed
  cluster authentication logs the remote address only (#89).

## [0.1.0] - 2025-05-15

First release.

[Unreleased]: https://github.com/dankomiocevic/ghoti/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/dankomiocevic/ghoti/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/dankomiocevic/ghoti/releases/tag/v0.1.0
