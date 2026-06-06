# graph-state-patch-runner Specification

## Purpose
TBD - created by archiving change add-graph-state-patch-runner. Update Purpose after archive.
## Requirements
### Requirement: Graph 节点必须支持写集声明

Graph MUST support registering nodes with structured write-set metadata while preserving the legacy `AddNode(name, fn)` API.

#### Scenario: legacy AddNode remains compatible

- **WHEN** existing code registers a node through `AddNode(name, fn)`
- **THEN** the graph should compile and execute as before for linear frontiers

#### Scenario: node spec declares writes

- **WHEN** code registers a node with `NodeSpec`
- **THEN** the graph should retain the node name, function and declared write set

### Requirement: Graph runner 必须支持 patch-aware 节点

Graph runner MUST support nodes that return `domain.StatePatch` and apply those patches centrally.

#### Scenario: patch-aware node applies patch

- **WHEN** a patch-aware node returns a non-empty `StatePatch`
- **THEN** runner should apply the patch to Session through `domain.ApplyStatePatch`

#### Scenario: patch apply failure is surfaced

- **WHEN** `domain.ApplyStatePatch` fails
- **THEN** runner should return an error associated with the node

#### Scenario: patch-aware node may apply patch before suspension

- **WHEN** a patch-aware node returns a `StatePatch`
- **AND** returns an error explicitly marked as patch suspension
- **AND** the error wraps `ErrSuspended`
- **THEN** runner should apply the patch before entering the existing suspension flow

#### Scenario: ordinary error does not apply patch

- **WHEN** a patch-aware node returns a `StatePatch`
- **AND** returns an error that is not explicitly marked as patch suspension
- **THEN** runner MUST NOT apply the patch

#### Scenario: interview suspend nodes use explicit patch-on-suspend

- **WHEN** the Interview Graph registers `pick_next` or `probe_ask`
- **THEN** it SHOULD register them as patch-aware nodes
- **AND** the runner should apply their patch before suspension only through explicit patch-on-suspend semantics

#### Scenario: non-suspend interview nodes use runner-level patch

- **WHEN** the Interview Graph registers `retrieve_rag`, `evaluate`, or `report`
- **THEN** it SHOULD register them as patch-aware nodes
- **AND** the runner should apply their `StatePatch`
- **AND** their legacy direct-call constructors should remain compatible

#### Scenario: probe evaluation uses runner-level patch

- **WHEN** the Interview Graph registers `probe_eval`
- **THEN** it SHOULD register it as a patch-aware node
- **AND** the runner should apply its `StatePatch`
- **AND** the legacy direct-call constructor should remain compatible

#### Scenario: critic uses runner-level patch

- **WHEN** the Interview Graph registers `critic`
- **THEN** it SHOULD register it as a patch-aware node
- **AND** the runner should apply its `StatePatch`
- **AND** the legacy direct-call constructor should remain compatible

#### Scenario: refine uses runner-level patch

- **WHEN** the Interview Graph registers `refine`
- **THEN** it SHOULD register it as a patch-aware node
- **AND** the runner should apply its `StatePatch`
- **AND** the legacy direct-call constructor should remain compatible

#### Scenario: update memory uses runner-level patch

- **WHEN** the Interview Graph registers `update_memory`
- **THEN** it SHOULD register it as a patch-aware node
- **AND** the runner should apply its `StatePatch`
- **AND** the legacy direct-call constructor should remain compatible
- **AND** its patch should include both `WorkingMemory` and current round completion when settling a round

### Requirement: 并发 frontier 必须检测写冲突

Graph runner MUST reject unsafe concurrent frontiers before executing their nodes.

#### Scenario: conflicting write sets are rejected

- **WHEN** a concurrent frontier contains two nodes with overlapping writes
- **THEN** runner should return an error before executing those nodes

#### Scenario: legacy node without writes is rejected in concurrent frontier

- **WHEN** a concurrent frontier contains a legacy node without declared writes
- **THEN** runner should reject the frontier before executing those nodes

#### Scenario: disjoint write sets may run concurrently

- **WHEN** all nodes in a concurrent frontier declare disjoint writes
- **THEN** runner may execute the frontier concurrently

### Requirement: 外部接口保持兼容

The change MUST NOT alter HTTP API responses, SSE payloads, Session JSON format, or database schema.

#### Scenario: existing business session format remains unchanged

- **WHEN** graph nodes execute through the new runner
- **THEN** Session JSON should not gain graph write-set or patch metadata fields
