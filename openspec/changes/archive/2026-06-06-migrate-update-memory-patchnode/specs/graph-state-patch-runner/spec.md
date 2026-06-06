## MODIFIED Requirements

### Requirement: Graph runner 必须支持 patch-aware 节点

Graph runner MUST support nodes that return `domain.StatePatch` and apply those patches centrally.

#### Scenario: update memory uses runner-level patch

- **WHEN** the Interview Graph registers `update_memory`
- **THEN** it SHOULD register it as a patch-aware node
- **AND** the runner should apply its `StatePatch`
- **AND** the legacy direct-call constructor should remain compatible
- **AND** its patch should include both `WorkingMemory` and current round completion when settling a round
