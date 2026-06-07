## MODIFIED Requirements

### Requirement: Final report must expose traceable per-question scoring

Completed interview Sessions MUST expose a final report that can be traced back to every answered main question and every answered follow-up.

#### Scenario: Report includes every answered main question

- **WHEN** a completed Session contains answered main question rounds
- **THEN** the final report MUST include one review item for each answered main question
- **AND** each review item MUST include the question id, question text, original answer, final score, hit points, missed points, and suggestion when available

#### Scenario: Report includes answered follow-ups

- **WHEN** an answered round contains answered follow-ups
- **THEN** the final report MUST expose those follow-ups under the corresponding main question review
- **AND** each answered follow-up review MUST include the follow-up question text, original answer, score, and scoring evidence when available

#### Scenario: Unanswered items are not scored as successful answers

- **WHEN** a question or follow-up has no original answer
- **THEN** the final report MUST NOT assign it a successful score
- **AND** it MUST NOT count as an effective answered item in aggregate scoring

#### Scenario: Overall score matches effective per-question scores

- **WHEN** the final report has effective scored main question reviews
- **THEN** `overall_score` MUST be derived from those effective scores using the documented aggregation rule
- **AND** the aggregate MUST be reproducible from the exposed per-question review data

#### Scenario: Frontend exam mode hides internal diagnostic state

- **WHEN** a Session is in `exam` mode
- **THEN** the candidate-facing interview and report UI MUST NOT place question bank controls, Agent state, Graph state, or event debug timeline ahead of the current question or report content

#### Scenario: Frontend practice mode keeps diagnostics secondary

- **WHEN** a Session is in `practice` mode
- **THEN** training aids MAY be visible
- **BUT** backend diagnostic state SHOULD be presented only as secondary diagnostics, not before the active question or per-question report review
