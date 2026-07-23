# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.1.0] - 2026-07-23

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
| v0.1.x       | Go 1.23            |

[unreleased]: https://github.com/feymanlee/slog/compare/v0.1.0...HEAD
[v0.1.0]: https://github.com/feymanlee/slog/releases/tag/v0.1.0
