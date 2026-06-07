# Tasks

- [x] Add normalized content-key helpers for question-bank dedupe.
- [x] Extend generation quality gates to reject candidates duplicated against existing question-bank content.
- [x] Guard staged import / commit so duplicate content within a job or against existing active questions is not written.
- [x] Preserve duplicate rejection reasons in generation job or import item review metadata.
- [x] Add focused tests for generation duplicate rejection, import commit duplicate blocking, and existing review policy compatibility.
- [x] Run `go test ./internal/questionbank -count=1`.
- [x] Update change documentation if implementation changes behavior beyond this design.
