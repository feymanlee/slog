# Structured Logging

This context defines how the library creates logging instances and attaches
runtime extensions without leaking state between instances.

## Language

**Logger**:
A configured logging instance that belongs to a Logger Lineage and carries output behavior, bound attributes, and groups. Package-level logging functions use the Default Logger.

**Logger Lineage**:
A Logger and the derived Loggers created from it with bound attributes or groups. Members of a Logger Lineage share one Logger Module catalog; independently constructed Loggers start separate lineages.

**Default Logger**:
The Logger targeted by package-level logging, configuration, module observation, and module update functions.

**Logger Module**:
A named formatter, handler, or sink installed into exactly one Logger Lineage. Different Logger Lineages may own Logger Modules with the same name without sharing runtime state.
_Avoid_: Plugin

**Module Factory**:
A globally discoverable recipe for creating a Logger Module from configuration. A Module Factory does not own per-Logger runtime state.
