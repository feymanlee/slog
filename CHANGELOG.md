# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.2.1] - 2026-07-29

### Fixed

- Isolated builder output defaults from process-wide state while preserving
  explicit text and JSON overrides.
- Kept text and JSON handlers available so runtime output modes can be
  disabled and re-enabled safely.
- Rejected nil factories and typed-nil modules during module registration.
- Waited for asynchronous log-file maintenance during close and hardened
  cleanup after compression failures.

### Quality

- Added contract tests for fatal exits, builders, fanout, module registration,
  writers, rate limiting, DLP masking, and structured attributes.
- Added a Go 1.23/stable CI matrix with race detection, randomized test order,
  a coverage floor, and Coveralls reporting.

## [v0.2.0] - 2026-07-29

### Added

- Six logging levels: Trace, Debug, Info, Warn, Error, and Fatal.
- Text and JSON output with optional terminal colors and source locations.
- DLP masking for 36 sensitive-data types, including struct tag support.
- Logger Lineage module ownership with isolated installation, configuration,
  diagnostics, and formatter snapshots.
- Runtime level/output/DLP controls and context attribute propagation.
- Log subscriptions with drop-oldest, drop-newest, and bounded-blocking
  backpressure policies plus subscriber statistics.
- Formatter, Logfmt, GELF, network, Syslog, Webhook, and fanout output modules.
- File rotation, compression, rate limiting, tiered pools, LRU caches, and
  xxhash-based cache keys.

### Changed

- The canonical Go module path is now `github.com/feymanlee/slog`.
- Formatter module updates now replace configuration atomically and preserve
  the previous formatter set when configuration fails.
- Module integrations use typed provider interfaces; handler and sink module
  delivery remains deferred until the async output lifecycle is unified.

### Quality

- Requires Go 1.23 or later.
- Full package tests and race detection pass before release.

---

## Version Policy

This project follows [Semantic Versioning](https://semver.org/):

- **MAJOR**: Incompatible API changes
- **MINOR**: Backwards-compatible new features
- **PATCH**: Backwards-compatible bug fixes

### Go Version Support

| slog Version | Minimum Go Version |
| ------------ | ------------------ |
| v0.2.x       | Go 1.23            |

[unreleased]: https://github.com/feymanlee/slog/compare/v0.2.1...HEAD
[v0.2.1]: https://github.com/feymanlee/slog/releases/tag/v0.2.1
[v0.2.0]: https://github.com/feymanlee/slog/releases/tag/v0.2.0
