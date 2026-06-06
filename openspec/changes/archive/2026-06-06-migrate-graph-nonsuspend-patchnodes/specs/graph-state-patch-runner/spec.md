## MODIFIED Requirements

### Requirement: Graph runner 必须支持 patch-aware 节点

Graph runner MUST support nodes that return `domain.StatePatch` and apply those patches centrally.

#### Scenario: patch-aware node applies patch

- **WHEN** a patch-aware node returns a non-empty `StatePatch`
- **THEN** runner should apply the patch to Session through `domain.ApplyStatePatch`

#### Scenario: patch apply failure is surfaced

- **WHEN** `domain.ApplyStatePatch` fails
- **THEN** runner should return an error associated with the node

#### Scenario: non-suspend interview nodes use runner-level patch

- **WHEN** the Interview Graph registers `retrieve_rag`, `evaluate`, or `report`
- **THEN** it SHOULD register them as patch-aware nodes
- **AND** the runner should apply their `StatePatch`
- **AND** their legacy direct-call constructors should remain compatible

#### Scenario: suspended nodes stay legacy until patch-on-suspend exists

- **WHEN** an interview node returns `ErrSuspended` as part of its successful business flow
- **THEN** it MUST NOT be migrated to runner-level `PatchNode` unless the runner explicitly supports applying patches on suspension
