/**
 * §06 conditional form rules — per-field visible_if conditions, per-step
 * visible_if conditions, and forward-only jump rules. Semantics are mirrored
 * 1:1 from internal/formlogic (Go) and japaconnect_core (Dart) — keep all
 * three in sync.
 * Ref: japaconnect-engineering/sections/06-custom-field-builder-form-builder.md (ADR-137)
 * Ref: docs/superpowers/specs/2026-11-09-conditional-form-logic-design.md
 */

export interface Clause {
	/** May be omitted inside jump.when — defaults to the owning field's key. */
	field?: string;
	op: string;
	value?: unknown;
}

export interface Condition {
	all?: Clause[];
	any?: Clause[];
}

export interface JumpRule {
	when?: Condition;
	step: string;
}

export interface FieldLogicConfig {
	visible_if?: Condition | null;
	jump?: JumpRule | null;
}

export interface StepLogicConfig {
	visible_if?: Condition | null;
}

export interface LogicField {
	key: string;
	/** Containing step's identifier; "" = ungrouped. */
	stepKey: string;
	config?: FieldLogicConfig | null;
}

type Answers = Record<string, unknown>;

function isAnswered(v: unknown, ok: boolean): boolean {
	if (!ok || v === null || v === undefined) return false;
	if (typeof v === 'string') return v.trim() !== '';
	if (Array.isArray(v)) return v.length > 0;
	return true;
}

function toBool(v: unknown): boolean | undefined {
	if (typeof v === 'boolean') return v;
	if (typeof v === 'string') {
		switch (v.toLowerCase()) {
			case 'true':
			case '1':
			case 'yes':
			case 'on':
				return true;
			case 'false':
			case '0':
			case 'no':
			case 'off':
				return false;
		}
	}
	return undefined;
}

function toFloat(v: unknown): number | undefined {
	if (typeof v === 'number') return Number.isFinite(v) ? v : undefined;
	if (typeof v === 'string') {
		const f = parseFloat(v.trim());
		return Number.isNaN(f) ? undefined : f;
	}
	return undefined;
}

function scalarEqual(a: unknown, b: unknown): boolean {
	const ab = toBool(a);
	if (ab !== undefined) {
		const bb = toBool(b);
		if (bb !== undefined) return ab === bb;
	}
	const af = toFloat(a);
	if (af !== undefined) {
		const bf = toFloat(b);
		if (bf !== undefined) return af === bf;
	}
	return String(a) === String(b);
}

function evalClause(cl: Clause, answers: Answers, selfKey: string): boolean {
	// Malformed (non-object) clause → false, matching the Go evaluator's
	// zero-value Clause{} — a broken clause must not silently pass.
	if (cl == null || typeof cl !== 'object') return false;
	const key = typeof cl.field === 'string' && cl.field !== '' ? cl.field : selfKey;
	const present = Object.prototype.hasOwnProperty.call(answers, key);
	const v = answers[key];
	const has = isAnswered(v, present);

	switch (cl.op) {
		case 'answered':
			return has;
		case 'not_answered':
			return !has;
	}
	if (!has) return false;

	switch (cl.op) {
		case 'eq':
			return scalarEqual(v, cl.value);
		case 'neq':
			return !scalarEqual(v, cl.value);
		case 'in': {
			const arr = Array.isArray(cl.value) ? cl.value : [];
			return arr.some((item) => scalarEqual(v, item));
		}
		case 'contains': {
			const arr = Array.isArray(v) ? v : [];
			return arr.some((el) => scalarEqual(el, cl.value));
		}
	}
	return false; // unknown op → clause false
}

/**
 * Eval reports whether the condition holds for answers:
 * AND(all) && (no any-group || OR(any)). Absent/malformed → true (fail-open).
 */
export function evalCondition(
	cond: Condition | null | undefined,
	answers: Answers,
	selfKey = ''
): boolean {
	if (cond == null || typeof cond !== 'object') return true;
	if (Array.isArray(cond.all)) {
		for (const cl of cond.all) {
			if (!evalClause(cl, answers, selfKey)) return false;
		}
	}
	if (Array.isArray(cond.any) && cond.any.length > 0) {
		for (const cl of cond.any) {
			if (evalClause(cl, answers, selfKey)) return true;
		}
		return false;
	}
	return true;
}

/**
 * fieldVisible reports whether a field's own visible_if passes.
 * Absent/malformed config → visible (fail-open).
 */
export function fieldVisible(
	config: FieldLogicConfig | null | undefined,
	answers: Answers,
	selfKey = ''
): boolean {
	if (config == null || typeof config !== 'object') return true;
	return evalCondition(config.visible_if, answers, selfKey);
}

/** stepVisible reports whether a step's visible_if passes (nil → visible). */
export function stepVisible(
	config: StepLogicConfig | null | undefined,
	answers: Answers
): boolean {
	if (config == null || typeof config !== 'object') return true;
	return evalCondition(config.visible_if, answers, '');
}

/** visibleFields filters a field list by visible_if against current answers. */
export function visibleFields<
	T extends { key: string; config?: FieldLogicConfig | null }
>(fields: T[], answers: Answers): T[] {
	return fields.filter((f) => fieldVisible(f.config, answers, f.key));
}

/**
 * hiddenStepsByJump returns the step keys hidden by triggered forward-only
 * jumps. stepOrder lists every step identifier in natural order; stepHidden
 * seeds the result with steps hidden by other mechanisms (their own
 * visible_if). A jump fires when its field passes fieldVisible, its
 * containing step is not already hidden, and jump.when holds; an absent or
 * empty when fires iff the owning field is answered (a jump is
 * answer-driven — an unconditional interval skip is an authoring error).
 * Sources are processed in natural step order so a field inside a step
 * hidden by an earlier jump can never fire its own; it hides the open
 * interval (source, target). Backward/self/unknown targets are ignored;
 * multiple triggered jumps union their hidden intervals.
 */
export function hiddenStepsByJump(
	fields: LogicField[],
	stepOrder: string[],
	answers: Answers,
	stepHidden?: ReadonlySet<string>
): Set<string> {
	const idx = new Map<string, number>();
	stepOrder.forEach((k, i) => idx.set(k, i));
	const hidden = new Set<string>(stepHidden ?? []);
	const sorted = [...fields].sort(
		(a, b) => (idx.get(a.stepKey) ?? -1) - (idx.get(b.stepKey) ?? -1)
	);
	for (const f of sorted) {
		if (hidden.has(f.stepKey)) continue;
		if (!fieldVisible(f.config, answers, f.key)) continue;
		const j = f.config?.jump;
		if (j == null || typeof j !== 'object' || !j.step) continue;
		const hasWhen = Boolean(
			(j.when && ((Array.isArray(j.when.all) && j.when.all.length > 0) || (Array.isArray(j.when.any) && j.when.any.length > 0)))
		);
		if (hasWhen
			? !evalCondition(j.when, answers, f.key)
			: !isAnswered(answers[f.key], Object.prototype.hasOwnProperty.call(answers, f.key))) continue;
		const s = idx.get(f.stepKey);
		const t = idx.get(j.step);
		if (s === undefined || t === undefined || t <= s) continue;
		for (let i = s + 1; i < t; i++) hidden.add(stepOrder[i]);
	}
	return hidden;
}
