# Logger Module Ownership Design

## Context

`Logger.Use` localizes `extensions`, then registers the supplied Logger Module in
the process-wide `modules.Registry` and in multiple instance-local collections.
The global registry rejects duplicate names, so two independent Loggers cannot
reliably install separate Logger Modules with the same name. Configuration,
diagnostics, and execution can also observe different collections.

The relevant domain terms are defined in `CONTEXT.md`: Logger, Default Logger,
Logger Lineage, Logger Module, and Module Factory.

## Goals

- Give each Logger Lineage sole ownership of its Logger Module instances.
- Allow independent Logger Lineages to install Logger Modules with the same name.
- Give module installation, updates, ordering, and diagnostics one source of truth.
- Preserve existing exported interfaces and legacy `modules.Registry` behavior.
- Make the Logger interface the test surface for module behavior.

## Non-goals

- Do not activate `TypeHandler` or `TypeSink` delivery in this change. Their
  delivery and lifecycle belong to the later asynchronous-output deepening.
- Do not redesign Logger construction, DLP, subscriptions, or async execution.
- Do not remove or change the exported `modules.Registry` interface.
- Do not introduce an exported catalog interface.

## Chosen approach

Add one private, Logger-owned module catalog per Logger Lineage. The catalog is a
deep in-process module that owns runtime Logger Module instances, name lookup,
installation order, formatter snapshots, configuration updates, and diagnostic
snapshots.

Alternative approaches were rejected:

- Cloning `modules.Registry` per Logger would retain its mixed factory and
  instance responsibilities and remain shallow.
- Partitioning a global registry by Logger ID would avoid name collisions but
  keep instance ownership and tests coupled to global state.

## Ownership model

- Every independently constructed Logger starts a new Logger Lineage and module
  catalog.
- `With`, `WithGroup`, `WithContext`, and other derived Loggers remain in the
  same Logger Lineage and share the catalog.
- Different Logger Lineages may install Logger Modules with the same name.
- A duplicate name in one Logger Lineage is rejected.
- The Default Logger owns the catalog observed by package-level functions.

## Module shape

The implementation adds a private `loggerLineage` that holds a private
`moduleCatalog`. `Logger` and `eHandler` retain a reference to the lineage so
derived Loggers and their handlers observe the same catalog.

The catalog owns:

- a name index for duplicate detection and updates;
- an ordered list for stable diagnostics;
- a formatter snapshot rebuilt after successful formatter installation or
  configuration;
- synchronization needed for concurrent logging, updates, and diagnostics.

The catalog does not own Module Factories. The existing process-wide
`modules.Registry` continues to own factories and explicitly registered legacy
global module instances.

`extensions` retains DLP and runtime formatter registration, but no longer owns
`moduleRegistry`, `registeredModules`, or `moduleIndex`.

## Data flow

### Installation

1. `Logger.UseWithError` validates the Logger and Logger Module.
2. It registers the Logger Module in the current lineage catalog only.
3. The catalog rejects a duplicate name before changing its state.
4. For a formatter module, the catalog rebuilds its formatter snapshot.
5. Every `eHandler` in the lineage reads that snapshot while transforming
   attributes.

`Logger.Use` preserves its chainable behavior by discarding the returned error.

### Configuration

A new `(*Logger).UpdateModuleConfig(name, config)` operation updates only the
current Logger Lineage. The catalog calls the Logger Module's existing
`Configure` operation without holding the catalog lock. Updates to the same
Logger Module are serialized by the catalog. On success, the catalog refreshes
the formatter snapshot.

The built-in formatter adapter constructs a replacement formatter list and
swaps it into place only after configuration succeeds. Updating that adapter
therefore replaces its previous formatter configuration rather than appending
to it. Third-party Logger Modules retain responsibility for the transactional
behavior of their own `Configure` implementations.

The existing package-level `UpdateModuleConfig` first updates the Default
Logger. It falls back to `modules.UpdateModuleConfig` only when the Default
Logger does not own a Logger Module with that name. A real configuration error
is returned directly and never hidden by fallback behavior.

### Observation

- `(*Logger).Diagnostics` observes only its Logger Lineage.
- `RegisteredModules` observes only the Default Logger.
- `CollectModuleDiagnostics` observes only the Default Logger.
- Legacy callers can continue to inspect `modules.Registry` directly.

## Error semantics

- Nil Logger or invalid Logger Module inputs return an error from
  `UseWithError`.
- Duplicate names in one Logger Lineage return a duplicate-name error.
- Updating an absent Logger Module returns a distinct not-found error so the
  package-level compatibility fallback can identify that case precisely.
- Logger Module configuration errors propagate unchanged.
- Formatter snapshots change only after successful installation or
  configuration.
- Built-in formatter configuration remains unchanged when replacement
  configuration fails.

No new error is exposed solely to create another seam. Exported sentinel errors
may be added only when callers need `errors.Is` to distinguish not-found from a
configuration failure.

## Compatibility

- Existing constructors and package-level logging functions keep their current
  signatures.
- Existing `Logger.Use`, `Logger.UseWithError`, `UseModule`, and
  `UseModuleWithError` signatures remain unchanged.
- The exported `modules.Registry` interface and package-level functions remain
  available with their existing legacy-global meaning.
- Runtime formatter registration through `RegisterFormatter` remains separate
  from formatter Logger Modules.
- Existing global DLP inheritance behavior remains unchanged.

The intentional behavior fix is that independent Logger Lineages no longer
conflict when they install same-named Logger Modules.

## Testing

Tests exercise behavior through Logger interfaces:

- independent Loggers install and execute same-named formatter modules with
  different configurations;
- one Logger Lineage rejects a duplicate name;
- installing or updating a formatter through a derived Logger affects its
  parent and siblings in the same lineage;
- independent Logger Lineages remain unaffected;
- instance configuration updates stay local;
- built-in formatter updates replace prior configuration instead of
  accumulating formatters;
- package-level updates target the Default Logger and fall back only on
  not-found;
- instance and package-level diagnostics observe the documented catalogs;
- concurrent logging, diagnostics, and formatter updates are race-free.

Verification commands:

```sh
go test ./...
go test -race ./...
```

## Deferred work

`TypeHandler` and `TypeSink` currently do not enter Logger delivery through
`Logger.Use`; `ApplyModulesToHandler` has no callers. This design records that
gap without making the ownership change also alter delivery behavior. The
asynchronous-output deepening will define one delivery lifecycle and its test
surface for these Logger Modules.
