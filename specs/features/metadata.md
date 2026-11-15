---
id: META
status: implemented
version: V1
depends_on: [CFG]
modules: [internal/app/metadata, internal/infra/providers]
---
# Metadata

Goal: build `models-metadata.json` from API metadata plus user metadata, honestly and safely.

## Rules

- META-1 `models.json` MAY hold optional user metadata per model (all fields optional).
- META-2 MUST fetch API metadata through the provider adapter where the adapter supports it.
- META-3 MUST generate `models-metadata.json` by merging API and user metadata per field; the user value ALWAYS wins.
- META-4 Missing values MUST stay unknown (absent or null). MUST NOT invent capabilities, speed, or pricing.
- META-5 A model without metadate MUST NOT be added to any strategy, show a warning, and continue.
- META-6 `models-metadata.json` MUST NOT contain API keys (FILE-3).
- META-7 MUST write it atomically (FILE-2).
- META-8 If a fetch fails for a model, MUST keep that model's last known API metadata, show a warning, and continue. MUST NOT invent values.
- META-9 MUST generate the file at startup when absent. Later regeneration happens only via `/refresh` (REFRESH).
- META-10 Metadata refresh MUST NOT lose or alter user metadata.
- META-11 MUST write the source of the metadata (`api` or `user`)

## Clarifications

- Metadata fields: `capabilities` (list of strings), `speed` (string), `pricing`.
- `pricing` is `{ "inputPer1M": number, "outputPer1M": number }`, USD per million tokens. Each price is optional; `0` means free, an absent value means unknown. Negative prices are invalid (user file: config problem; provider data: treated as a failed fetch, META-8).
- Merge (META-3) is per field; `pricing` is one field, so a user pricing block replaces the API block as a whole.
- META-11: `models-metadata.json` records `sources` per known field (`capabilities`, `speed`, `pricing`), each `api` or `user`. Unknown fields have no source.

## Out of scope

Automatic model discovery. Learning from usage. Periodic background refresh.

## Acceptance criteria

- AC1 (META-3): API says `speed: slow`, user says `speed: fast`; output has `fast`.
- AC2 (META-4,5): model with no API and no user metadata appears with unknown fields.
- AC3 (META-6): output file contains no key strings.
- AC4 (META-8): adapter throws for one model; others still written; warning shown.
- AC5 (META-10): regenerate twice; user values unchanged.

## Open questions

None.
