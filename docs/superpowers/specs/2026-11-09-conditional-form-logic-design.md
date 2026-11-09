# Conditional Form Logic (visibility rules + goto jumps) — Design

Date: 2026-11-09 · Status: approved (user Q&A resolved)

## Context

`/tools/readiness` (§71, ADR-071) is a `forms` row with `form_type='assessment'`.
Questions are `custom_fields` whose `config` JSONB holds
`{weight, dimension, option_scores}`; steps are derived client-side by grouping
`config.dimension`. Standard multi-step forms use `form_steps` rows instead.
Today every field renders statically — there is no way for a question (or
step) to appear only when a previous answer warrants it, and no way to jump to
a later step based on a choice.

This design adds one declarative condition primitive — `visible_if` — applied
at field level and step level, plus explicit forward-only `jump` rules on
fields. It applies to **all §06 forms**, not just the assessment. Skipped
questions are fully excluded from the score (numerator and denominator).

Approved decisions: branching = field visibility + option-driven follow-ups +
step jumps; scope = all §06 forms; skipped questions excluded from scoring;
jump conflicts resolve as union-of-intervals.

Correction (found during implementation planning): `publicFieldDTOs` in
`internal/assessment/handlers.go` already whitelists field config to
`{dimension}` — `option_scores` never leaves the server today, so there is no
leak to fix. The actual work inverts: `visible_if` and `jump` must be **added**
to that whitelist (and to `scoring_config.step_rules` in the public response)
so clients can evaluate them.

## Goals

- A question can be shown/hidden by a rule over prior answers
  (`purpose=study` → reveal `admission_status`, `study_funding`).
- A step can be shown/hidden by a rule (`purpose=tourism` → hide the
  `professional` step).
- A choice can jump the user forward to a later step (`purpose=study` → land
  directly on the `study` step), hiding the steps in between.
- Identical evaluation semantics on web (TS), mobile (Dart), and server (Go).
- Server enforces the same rules: hidden fields are not required, not
  option-validated, and never scored — a crafted payload cannot smuggle
  scores for questions the user never saw.
- Everything is data (`config` JSONB); no new API routes.

## Non-goals

- Arbitrary non-linear *reordering* of the step rail (back-edges, cycles) —
  jumps are forward-only.
- Visibility logic on entity-scoped custom fields (space/tag/profile fields).
  The keys are ignored harmlessly there; only fields attached to forms
  participate.
- A dedicated rules table — rules live in existing JSONB config columns.

## Rule grammar

```jsonc
Condition = { "all": [Clause, …], "any": [Clause, …] }
Clause    = { "field": "<field_key>", "op": "<op>", "value": <any> }
```

- Both `all` and `any` present → `AND(∀ all, ∃ any)`. Only one present →
  that group's result. Neither present → condition true.
- Ops: `eq` | `neq` | `in` | `contains` | `answered` | `not_answered`.
- `value` is required except for `answered`/`not_answered`.
- `eq` normalizes booleans (`true`/`"true"`) and numeric strings before
  comparing. `in` = answer ∈ value array. `contains` = answer is an array
  containing value (multiselect); a non-array answer → false. `neq` =
  *answered and* ≠ — it does not fire on an unanswered field.
- Clause `value`s compare against the **canonical option value** for
  select/multiselect sources. Server-side, the eval view normalizes
  submitted labels → values via the existing `NormalizeChoiceValue`
  (embed clients submit option *labels*; assessment clients submit
  *values*). Admin-authored options store `value == label`, so this is a
  no-op for every field the form builder creates — it only matters for
  seeded fields with distinct pairs, and keeps server/client eval
  identical. The admin rule builder picks values from the field's raw
  `{value,label}` option defs (new `option_defs` key on the admin field
  JSON; the public `options` list stays label-only).
- A clause references the **answers map only**, never the referenced field's
  own visibility. Chains (hidden field revealing another field) work: a field
  whose `visible_if` passes can itself gate deeper fields. There is no
  recursion, so cycles are impossible.
- An unanswered referenced field makes every op false except `not_answered`.
- Absent or malformed `visible_if`/`jump` → treated as absent (fail-open:
  field visible, no jump). Old clients on new forms degrade to "extra
  question shown", never "questions missing".
- Inside `jump.when`, a clause may omit `field` — it defaults to the field
  carrying the rule (authoring shorthand for "when this answer is X").

## Storage

| Location | Content | Migration |
|---|---|---|
| `custom_fields.config.visible_if` | `Condition` — field show/hide | none |
| `custom_fields.config.jump` | `{when: Condition, step: "<target>"}` | none |
| `form_steps.config` (new `jsonb NOT NULL DEFAULT '{}'`) | `visible_if` for standard multi-step steps | yes — up/down pair, mirrors `form_submission_triggers.config` |
| `forms.scoring_config.step_rules.<dimension>.visible_if` | `Condition` — assessment step hide (steps are dimension-derived, no `form_steps` rows exist) | none |

`jump.step` targets: a dimension name for assessment forms; a `form_steps.id`
for standard forms.

Option-driven follow-ups are **authoring sugar only**: the admin field/option
editor can show "reveals: X, Y" per option, but storage stays `visible_if` on
the child field (`{all:[{field, op: eq|contains, value}]}`). No second
mechanism.

## Evaluation semantics

```
visibleFields  = fields.filter(f => eval(f.config.visible_if, answers))
stepVisible(s) = eval(step.visible_if, answers)            // explicit rule
              && visibleFields(s).length > 0               // auto-hide empty steps
jumpHidden     = union over triggered jumps of open intervals (S_idx, T_idx)
visibleSteps   = naturalOrder.filter(s => stepVisible(s) && s ∉ jumpHidden)
```

- Field visibility and step visibility both evaluate against the current
  answers; order of evaluation does not matter (no recursion).
- Jumps: for each **visible** field (one that passes `visible_if`) in natural
  order, if `eval(jump.when, answers)` then resolve `(S → T)` where S is the
  step containing the field and T the target step index. A hidden field can
  never fire a jump — its `when` isn't evaluated. T must be **strictly after** S; backward/self/unknown targets
  are ignored at runtime and warned at save time. All triggered jumps apply;
  hidden intervals merge (union). A jump whose target lies inside an already
  hidden interval still applies after T — chains work.
- A jump never force-shows its target: the target's own `visible_if` still
  applies.
- `is_multi_step=false` forms: field `visible_if` applies; no steps/jumps.

## Backend changes (Go)

New leaf package **`internal/formlogic`** — pure functions, no I/O, no deps
beyond stdlib:

```go
type Clause    struct{ Field, Op string; Value any }
type Condition struct{ All, Any []Clause }
type JumpRule  struct{ When Condition; Step string }
Eval(cond, answers) bool
VisibleFields(fields, answers) []Field          // field = {key, config}
ResolveJumps(fields, steps, answers) (hidden map[int]bool)
```

`FieldScoringConfig` gains `VisibleIf *Condition json:"visible_if,omitempty"`
and `Jump *JumpRule json:"jump,omitempty"` — the same shape is reused for
non-assessment field configs.

Applied at three points:

1. **`customfields.Handlers.validateFormSubmission`**
   (`internal/customfields/handlers.go`) — after the existing RBAC visibility
   filter (CF-S6; the two axes compose — a field must pass both), build a
   canonical key→value *view* via the existing label→key lookup (browser
   submissions arrive label-keyed; the stored `field_values` keep their
   submitted key style — canonicalization is for evaluation only), evaluate
   visibility, then: hidden fields are skipped by the required check, and
   their submitted entries (matched by label or key) are dropped from
   `field_values` so the stored snapshot is the visible-path truth.
2. **`assessment.SubmitAssessment`** (`internal/assessment/service.go`) —
   `missingRequiredFields` and `invalidOptionAnswers` run over the
   server-evaluated visible field set only.
3. **`assessment.computeScore`** — skip fields whose `visible_if` fails given
   the submission answers (defense-in-depth: stale client-side answers and
   crafted payloads cannot score hidden questions; skipped ⇒ excluded from
   both numerator and denominator, matching today's unanswered behavior).
4. **Field config write path (new)** — `fieldRequest` gains
   `Config *json.RawMessage` (PATCH three-state: absent = untouched, present =
   full config replace); `UpdateCustomField`/`CreateCustomField` SQL gain a
   `config` param (sqlc regen). Today no API can write field config —
   weights/`option_scores` are migration-seeded only — so this path is
   required for the admin rule editor (and finally lets admins tune scoring
   config without migrations). The admin form-GET `fieldJSON` starts
   returning the raw `config` object so the editor can merge keys.

5. **Serialization** — `visible_if`/`jump` ride inside the existing `config`
   JSONB to clients. Two whitelist points need extending:
   `publicFieldDTOs` (assessment) ships `config: {dimension}` today → add
   `visible_if`, `jump` (keeping `weight`/`option_scores` private); the public
   assessment response ships `scoring_config: {max_score, thresholds}` → add
   `step_rules`. The embed/generic-form field serializer gains the same
   `config: {visible_if, jump}` whitelist; `form_steps.config` is added to
   step payloads. **No new routes.**

## Client changes

- **`packages/formlogic`** (new, `@japaconnect/formlogic`) — TS port of the
  evaluator (`evalCondition`, `visibleFields`, `visibleSteps`, `resolveJumps`).
  Consumed by `apps/main` and `apps/admin`.
- **`apps/main …/tools/readiness/+page.svelte`** — `visibleFields` drives the
  existing dimension-grouping derived; hidden steps leave the rail;
  `stepComplete`/`requiredFields`/progress count visible only; `currentStep`
  is clamped when a step disappears; submit still sends the full `answers`
  map (server canonicalizes).
- **`apps/main …/forms/[slug]/+page.svelte`** — same evaluator for field
  visibility + `form_steps` visibility/jumps.
- **Mobile** — Dart port in `packages/japaconnect_core`, applied in
  `readiness_screen.dart` (`_stepOrder`/`_fieldsForStep`/`_isStepComplete` and
  submit-time required check).
- **Admin `/forms/[formId]`** — field editor gains Visibility (field → op →
  value typed by source-field type) and Jump (when → step select) editors;
  option editor shows a derived "reveals" list; the Assessment-steps card
  edits `scoring_config.step_rules`; standard step rows edit
  `form_steps.config.visible_if`. Save-time warnings: unknown field refs,
  backward/self jumps, jumps to always-hidden steps. JSON editing stays
  available via the existing `JsonValueEditor` path.
- **Stale answers**: kept client-side so re-showing a field restores input;
  dropped server-side at submit (canonical stored snapshot).

## Edge cases

| Case | Behavior |
|---|---|
| Required + hidden | Not required (client + server agree) |
| Hidden field, stale answer in payload | Dropped at validation; never scored |
| All fields of a step hidden | Step auto-hidden even without a step rule |
| Jump inside a hidden interval | Still applies after its target (chains) |
| Jump to backward/self/unknown step | Ignored at runtime; warned at save |
| Rule references unknown field key | Clause false (treated as unanswered) |
| Malformed `visible_if`/`jump` JSON | Treated as absent (fail-open) |
| Draft restore / retake | Answers re-evaluate; rules re-apply cleanly |
| CF-S6 RBAC-hidden fields | Excluded before answer-driven eval (both must pass) |
| Standard-form submission keyed by label | Canonicalized to field keys before eval |
| Select answer submitted as label | Normalized label→value before eval |

## JEOS + migrations

Per Rule 1 (spec before code):

- `sections/06-custom-field-builder-form-builder.md` — condition grammar,
  `visible_if`/`jump` semantics, `form_steps.config`, evaluation rules.
- `sections/71-readiness-assessment.md` — `scoring_config.step_rules`,
  skip-exclusion semantics for scoring.
- New **ADR-137** — decision record (visibility+goto over flow-graph DSL;
  forward-only jumps; shared tri-language evaluator).
- Migration `NNNNNN_form_steps_config` — `ALTER TABLE form_steps ADD COLUMN
  config jsonb NOT NULL DEFAULT '{}'` (+ down drops it).
- No new API routes; catalogs unchanged.

## Testing

- Go (`internal/formlogic`, `internal/customfields`, `internal/assessment`):
  table tests — every op, `all`/`any` combos, unanswered refs, malformed
  rules, jump intervals/union/chains, hidden-required submit acceptance,
  hidden-answer score exclusion, public-whitelist regression (`visible_if`/
  `jump` ship; `weight`/`option_scores` stay private).
- TS (`packages/formlogic`): parity tests driven by a **shared JSON fixture**
  of `(rules, answers) → expected` mirrored by the Go suite so evaluators
  cannot drift.
- Dart (`japaconnect_core`): unit tests over the same vectors.
- Manual: `purpose=study` → `professional` step hidden, `admission_status` /
  `study_funding` revealed, jump lands on the study step; submit → result
  excludes skipped dimensions.

## File map (implementation checklist)

| Surface | Files |
|---|---|
| Go evaluator | `internal/formlogic/` (new) |
| Validation | `internal/customfields/handlers.go` |
| Submit + score | `internal/assessment/service.go`, `internal/assessment/handlers.go` |
| Steps payload | `internal/customfields/handlers.go` (step serialization), `internal/db/queries/` if config read needs a sqlc regen |
| Migration | `migrations/NNNNNN_form_steps_config.{up,down}.sql` |
| TS evaluator | `packages/formlogic/` (new) |
| Readiness UI | `apps/main/src/routes/(app)/tools/readiness/+page.svelte` |
| Generic form UI | `apps/main/src/routes/(app)/forms/[slug]/+page.svelte` |
| Admin builder | `apps/admin/src/routes/forms/[formId]/+page.svelte` |
| Mobile | `apps/mobile/lib/features/tools/readiness/screens/readiness_screen.dart`, `packages/japaconnect_core` |
| JEOS | `sections/06-…`, `sections/71-…`, `adr/ADR-137-…` |
