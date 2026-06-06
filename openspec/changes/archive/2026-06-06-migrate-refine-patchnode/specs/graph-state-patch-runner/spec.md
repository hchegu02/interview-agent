## MODIFIED Requirements

### Requirement: Graph runner 必须支持 patch-aware 节点

Graph runner MUST support nodes that return `domain.StatePatch` and apply those patches centrally.

#### Scenario: refine uses runner-level patch

- **WHEN** the Interview Graph registers `refine`
- **THEN** it SHOULD register it as a patch-aware node
- **AND** the runner should apply its `StatePatch`
- **AND** the legacy direct-call constructor should remain compatible
