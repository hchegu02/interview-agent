# Design

## Approach

The import pipeline keeps the existing parse -> validate -> stage shape. LLM enrichment is inserted between parse and stage for local question-bank imports.

## Data Flow

1. Parse uploaded question-bank file into `questionbank.Item`.
2. Detect items with missing metadata.
3. If no LLM model is configured, skip enrichment and keep legacy defaults.
4. If an LLM model is configured, request JSON-only enrichment for incomplete items.
5. Merge enriched data conservatively:
   - never replace `id`
   - never replace `content`
   - only fill missing metadata fields
6. Normalize, validate, and stage items.

## Compatibility

Existing imports without LLM continue to work. Existing complete imports are not rewritten by the LLM. The commit path remains explicit and unchanged.

## Risk

The main risk is LLM mismatch between returned items and original items. The implementation matches by `id`, falls back to `content`, and preserves the original identity fields.
