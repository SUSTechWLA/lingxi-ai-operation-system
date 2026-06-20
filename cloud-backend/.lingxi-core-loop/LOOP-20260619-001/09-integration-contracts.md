# 09 Integration Contracts

## Core Contract

- Workflows call external services only through the built-in `external` tool bridge.
- External tool name is passed as `input.parameters.tool`.
- Core passes `task_id`, `node_id`, and stage parameters to external service.
- External service returns JSON; Core stores the response in node output and trace.

## Black-Box Checks Completed

- CLI generated DAGs for all runtime skills.
- Generated DAGs use `external` bridge for executable stages.
- CONTROL nodes are represented as `CONTROL` nodes and are not worker-dispatched.

## Remaining External Work

External services must register tool manifests and endpoints before real media execution.
