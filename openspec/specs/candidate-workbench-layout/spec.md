## Purpose

Define the candidate workbench frontend layout, information hierarchy, responsive behavior, and auxiliary diagnostics boundaries without changing backend interview contracts.

## Requirements

### Requirement: Candidate workflow navigation preserves task focus
The frontend SHALL present candidate workflow navigation in a way that keeps preparation, interview, report, agent, and memory pages reachable without allowing global controls to dominate the main task area.

#### Scenario: Candidate workspace shows workflow navigation
- **WHEN** the user is in the candidate workspace
- **THEN** the interface MUST expose navigation to resume, JD analysis, interview, report, Agent, and memory views

#### Scenario: Page-specific actions stay near their page context
- **WHEN** the user configures an interview mode or starts an interview
- **THEN** the relevant controls MUST be visually associated with the preparation/start workflow rather than presented only as unrelated global settings

#### Scenario: Historical sessions do not block the main workflow
- **WHEN** historical sessions are available
- **THEN** the interface MUST keep session loading reachable without forcing the session list to consume primary screen space needed for the current workflow

### Requirement: Interview page prioritizes active answering
The frontend SHALL make the current question, answer input, prior answers, and feedback the primary default content of the interview page.

#### Scenario: Active question is the primary interview content
- **WHEN** an interview session has a current question
- **THEN** the current question and answer input MUST be easier to find than Agent state, SSE event details, or retrieval diagnostics

#### Scenario: Auxiliary execution state remains available
- **WHEN** a practice session includes WorkingMemory or stream events
- **THEN** the interface MUST keep those details accessible as auxiliary information without removing or changing the underlying data

#### Scenario: Submitting an answer preserves task continuity
- **WHEN** the user submits an answer and the backend is processing
- **THEN** the interface MUST show the pending answer or processing state without hiding the conversation context

### Requirement: Report page presents conclusions before diagnostics
The frontend SHALL present report conclusions and next training actions before low-level backend diagnostics.

#### Scenario: Report summary appears first
- **WHEN** a completed session has a report
- **THEN** the page MUST present overall score and skill breakdown before detailed diagnostics

#### Scenario: Training actions are surfaced before trace details
- **WHEN** the report contains a drill plan or next steps
- **THEN** the page MUST surface actionable training guidance before RAG retrieval trace details

#### Scenario: Backend explainability remains available
- **WHEN** a practice report contains WorkingMemory or retrieval trace data
- **THEN** the interface MUST keep that explainability data accessible without making it the dominant default reading path

### Requirement: Responsive layout preserves primary tasks
The frontend SHALL adapt to narrow screens by preserving access to the primary task before secondary navigation and diagnostics.

#### Scenario: Narrow candidate page keeps main task reachable
- **WHEN** the viewport is narrow
- **THEN** resume editing, JD editing, active answering, or report review content MUST remain reachable without first scrolling through a full desktop-style sidebar

#### Scenario: Narrow interview input remains usable
- **WHEN** the viewport is narrow and the user is on the interview page
- **THEN** the answer input and send action MUST remain usable without overlapping conversation content

#### Scenario: Narrow report modules remain readable
- **WHEN** the viewport is narrow and the report has multiple modules
- **THEN** report sections MUST stack or collapse in a way that avoids horizontal overflow and preserves readable text

### Requirement: Frontend UI optimization preserves backend contracts
The frontend SHALL NOT require backend contract changes for this layout optimization.

#### Scenario: Existing API responses remain sufficient
- **WHEN** the optimized frontend renders resume, JD, interview, report, Agent, memory, or question bank pages
- **THEN** it MUST use existing API responses and tolerate optional fields according to existing frontend types

#### Scenario: Session remains the source of truth
- **WHEN** a session is loaded, updated through REST, or updated through SSE
- **THEN** the frontend MUST continue to treat the backend Session snapshot as the authoritative workflow state

#### Scenario: No frontend-only graph decisions
- **WHEN** the user answers a question or views a report
- **THEN** the frontend MUST NOT decide whether to ask follow-ups, change questions, generate reports, or alter scoring outside the existing backend flow
