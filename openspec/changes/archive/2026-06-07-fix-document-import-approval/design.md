# Design

## Fix

`ImportStore.UpdateItemReviews` will continue to be the single store boundary used by item-level and batch review actions. Its behavior will be tightened:

- `review_status=accepted` sets `review_status='accepted'`.
- For valid items whose stored agent review status is `needs_human_review`, accepting also sets agent review status to `auto_approved`.
- `review_status=rejected` sets `review_status='rejected'` and agent review status to `rejected` for valid items.
- Existing `auto_approved` items remain approved.
- Structured imports with empty agent review status continue to work.

PG stores agent review metadata inside the existing item metadata map, so the fix updates that metadata JSON in place without changing schema. Memory store mirrors the same semantics for tests and local no-PG mode.

## Data Flow

`POST /api/question-bank/imports/:id/items/review`
-> `ImportService.ReviewItems` or `ReviewAllValidItems`
-> `ImportStore.UpdateItemReviews`
-> staging item review state and agent review state update
-> `ImportService.Commit`
-> `importItemAccepted`
-> `Writer.Upsert`
-> optional embedding write.

## Risk

The fix deliberately only modifies valid staging items. Invalid generated items remain blocked. Rejecting an item is stronger than before because it also persists agent rejection metadata, matching the existing commit policy.
