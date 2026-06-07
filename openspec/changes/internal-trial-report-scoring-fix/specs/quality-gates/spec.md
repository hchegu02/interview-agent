## MODIFIED Requirements

### Requirement: Verification gates must detect incomplete report scoring evidence

Local verification gates for internal trial readiness MUST detect final reports that are missing per-question scoring evidence.

#### Scenario: Missing answered question in report fails verification

- **WHEN** a verification fixture contains an answered main question round
- **AND** the final report omits that answered question from per-question review data
- **THEN** the verification gate MUST fail

#### Scenario: Missing original answer in report review fails verification

- **WHEN** a report review item represents an answered question or answered follow-up
- **AND** the review item does not expose the original answer
- **THEN** the verification gate MUST fail

#### Scenario: Missing per-question score evidence fails verification

- **WHEN** a report review item participates in scoring
- **AND** it lacks score, hit/missed point evidence, or suggestion where the source evaluation has those fields
- **THEN** the verification gate MUST fail

#### Scenario: Aggregate score mismatch fails verification

- **WHEN** `overall_score` cannot be reproduced from the effective per-question review scores
- **THEN** the verification gate MUST fail
