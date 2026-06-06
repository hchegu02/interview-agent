## MODIFIED Requirements

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
