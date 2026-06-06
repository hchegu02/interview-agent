## MODIFIED Requirements

### Requirement: Graph runner 必须支持 patch-aware 节点

Graph runner MUST support nodes that return `domain.StatePatch` and apply those patches centrally.

#### Scenario: difficulty update uses runner-level patch

- **WHEN** the Interview Graph registers `update_difficulty`
- **THEN** it SHOULD register it as a patch-aware node
- **AND** the runner should apply its `StatePatch`
- **AND** the legacy direct-call constructor should remain compatible

#### Scenario: reflection check uses runner-level patch

- **WHEN** the Interview Graph registers `reflection_check`
- **THEN** it SHOULD register it as a patch-aware node
- **AND** the runner should apply its `StatePatch`
- **AND** the legacy direct-call constructor should remain compatible

### Requirement: Graph checkpoint must include patch summary

Graph checkpoint MUST record a compact summary of applied patches for observability and verification.

#### Scenario: patch summary is recorded after successful apply

- **WHEN** a patch-aware node applies a `StatePatch` successfully
- **THEN** checkpoint data MUST include the node name, declared writes, affected round id and written fields
- **AND** checkpoint data MUST NOT store the full patch as a replay source

#### Scenario: suspend-with-patch records patch summary

- **WHEN** a patch-aware node applies a `StatePatch` before returning a suspension
- **THEN** the suspended checkpoint MUST include the patch summary
- **AND** the summary MUST mark the patch as suspended

### Requirement: cumulative nodes must be idempotent

Cumulative Interview Graph nodes MUST avoid applying the same round-level effect more than once.

#### Scenario: repeated cumulative node execution is skipped

- **WHEN** a cumulative node sees an already applied idempotency key for the current round and node
- **THEN** it MUST avoid applying duplicate cumulative effects
- **AND** it MUST preserve compatibility with existing sessions that have no marker

#### Scenario: cumulative node records marker on first apply

- **WHEN** `update_memory`, `update_difficulty`, or `reflection_check` applies a round-level cumulative effect
- **THEN** it MUST record a node-level marker in current-session working memory
- **AND** the marker key SHOULD identify both node and round
