## MODIFIED Requirements

### Requirement: Graph runner 必须支持 patch-aware 节点

Graph runner MUST support nodes that return `domain.StatePatch` and apply those patches centrally.

#### Scenario: patch-aware node applies patch

- **WHEN** a patch-aware node returns a non-empty `StatePatch`
- **THEN** runner should apply the patch to Session through `domain.ApplyStatePatch`

#### Scenario: patch apply failure is surfaced

- **WHEN** `domain.ApplyStatePatch` fails
- **THEN** runner should return an error associated with the node

#### Scenario: probe evaluation uses runner-level patch

- **WHEN** the Interview Graph registers `probe_eval`
- **THEN** it SHOULD register it as a patch-aware node
- **AND** the runner should apply its `StatePatch`
- **AND** the legacy direct-call constructor should remain compatible
