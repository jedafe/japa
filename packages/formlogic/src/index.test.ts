import { describe, it, expect } from 'bun:test';
import {
	evalCondition,
	fieldVisible,
	hiddenStepsByJump,
	type Clause,
	type LogicField,
} from './index';

// Mirrors internal/formlogic/logic_test.go — same cases, same expectations.
describe('evalClause ops (via evalCondition single-clause)', () => {
	const answers: Record<string, unknown> = {
		purpose: 'study',
		pets: ['dog', 'cat'],
		score: 80.0,
		flag: true,
		empty: '',
		emptyarr: [],
	};
	const ev = (cl: Clause, selfKey = '') => evalCondition({ all: [cl] }, answers, selfKey);
	const cases: [string, Clause, boolean][] = [
		['eq hit', { field: 'purpose', op: 'eq', value: 'study' }, true],
		['eq miss', { field: 'purpose', op: 'eq', value: 'work' }, false],
		['eq numeric', { field: 'score', op: 'eq', value: '80' }, true],
		['eq bool', { field: 'flag', op: 'eq', value: 'true' }, true],
		['neq answered diff', { field: 'purpose', op: 'neq', value: 'work' }, true],
		['neq answered same', { field: 'purpose', op: 'neq', value: 'study' }, false],
		['neq unanswered', { field: 'nope', op: 'neq', value: 'x' }, false],
		['in hit', { field: 'purpose', op: 'in', value: ['work', 'study'] }, true],
		['in miss', { field: 'purpose', op: 'in', value: ['work'] }, false],
		['contains hit', { field: 'pets', op: 'contains', value: 'dog' }, true],
		['contains miss', { field: 'pets', op: 'contains', value: 'fish' }, false],
		['contains nonarray', { field: 'purpose', op: 'contains', value: 'x' }, false],
		['answered yes', { field: 'purpose', op: 'answered' }, true],
		['answered empty', { field: 'empty', op: 'answered' }, false],
		['answered emptyarr', { field: 'emptyarr', op: 'answered' }, false],
		['answered missing', { field: 'nope', op: 'answered' }, false],
		['not_answered missing', { field: 'nope', op: 'not_answered' }, true],
		['not_answered present', { field: 'purpose', op: 'not_answered' }, false],
	];
	for (const [name, cl, want] of cases) {
		it(name, () => expect(ev(cl)).toBe(want));
	}
	it('self default (field omitted → owning key)', () =>
		expect(ev({ op: 'eq', value: 'study' }, 'purpose')).toBe(true));
});

describe('evalCondition all/any', () => {
	const answers = { a: 'x', b: 'y' };
	it('all pass', () =>
		expect(
			evalCondition(
				{ all: [{ field: 'a', op: 'eq', value: 'x' }, { field: 'b', op: 'eq', value: 'y' }] },
				answers
			)
		).toBe(true));
	it('all pass + any fails → false', () =>
		expect(
			evalCondition(
				{ all: [{ field: 'a', op: 'eq', value: 'x' }], any: [{ field: 'b', op: 'eq', value: 'z' }] },
				answers
			)
		).toBe(false));
	it('nil condition → true', () => expect(evalCondition(null, answers)).toBe(true));
	it('empty condition → true', () => expect(evalCondition({}, answers)).toBe(true));
	it('malformed clause → false (Go zero-Clause parity)', () => {
		// Go's json.Unmarshal maps a non-object element to a zero Clause{}
		// (swallowing the error) which evaluates false — mirror that here.
		expect(evalCondition({ all: [123 as unknown as Clause] }, answers)).toBe(false);
		expect(evalCondition({ any: [123 as unknown as Clause] }, answers)).toBe(false);
		// null/undefined elements must fail, not throw.
		expect(evalCondition({ all: [null as unknown as Clause] }, answers)).toBe(false);
	});
	it('non-string clause field falls back to selfKey (Go unmarshal-drop parity)', () => {
		// Go drops a non-string field on unmarshal → "" → selfKey.
		expect(
			evalCondition(
				{ all: [{ field: 123, op: 'eq', value: 1 } as unknown as Clause] },
				{ self: 1 },
				'self'
			)
		).toBe(true);
	});
});

describe('fieldVisible', () => {
	it('no config → visible', () => expect(fieldVisible(null, {})).toBe(true));
	it('empty config → visible', () => expect(fieldVisible({}, {})).toBe(true));
	it('failing rule hides', () =>
		expect(
			fieldVisible(
				{ visible_if: { all: [{ field: 'a', op: 'eq', value: 1 }] } },
				{ a: 'nope' }
			)
		).toBe(false));
	it('passing rule shows', () =>
		expect(
			fieldVisible(
				{ visible_if: { all: [{ field: 'a', op: 'eq', value: 1 }] } },
				{ a: 1.0 }
			)
		).toBe(true));
});

describe('hiddenStepsByJump', () => {
	const mk = (key: string, stepKey: string, config: string): LogicField => ({
		key,
		stepKey,
		config: JSON.parse(config),
	});
	const order = ['s1', 's2', 's3', 's4', 's5'];

	it('basic forward jump s1→s4 hides s2,s3', () => {
		const fields = [
			mk('f1', 's1', `{"jump":{"when":{"any":[{"op":"eq","value":"go"}]},"step":"s4"}}`),
			mk('f2', 's2', `{}`),
		];
		const h = hiddenStepsByJump(fields, order, { f1: 'go' });
		expect([...h].sort()).toEqual(['s2', 's3']);
	});

	it('untriggered jump hides nothing', () => {
		const fields = [mk('f1', 's1', `{"jump":{"when":{"any":[{"op":"eq","value":"go"}]},"step":"s4"}}`)];
		expect(hiddenStepsByJump(fields, order, { f1: 'stop' }).size).toBe(0);
	});

	it('backward/self/unknown targets ignored', () => {
		const fields = [
			mk('f3', 's3', `{"jump":{"when":{"any":[{"op":"eq","value":1}]},"step":"s1"}}`),
			mk('f4', 's3', `{"jump":{"when":{"any":[{"op":"eq","value":1}]},"step":"s3"}}`),
			mk('f5', 's3', `{"jump":{"when":{"any":[{"op":"eq","value":1}]},"step":"ghost"}}`),
		];
		expect(hiddenStepsByJump(fields, order, { f3: 1, f4: 1, f5: 1 }).size).toBe(0);
	});

	it('hidden field cannot fire a jump', () => {
		const fields = [
			mk('f6', 's1', `{"visible_if":{"all":[{"field":"gate","op":"eq","value":0}]},"jump":{"when":{"any":[{"op":"eq","value":1}]},"step":"s3"}}`),
		];
		expect(hiddenStepsByJump(fields, order, { f6: 1 }).size).toBe(0);
	});

	it('field in a jump-hidden step cannot fire', () => {
		const fields = [
			mk('j1', 's1', `{"jump":{"when":{"any":[{"op":"eq","value":"a"}]},"step":"s4"}}`),
			mk('j2', 's2', `{"jump":{"when":{"any":[{"op":"eq","value":"b"}]},"step":"s5"}}`),
			mk('j3', 's4', `{"jump":{"when":{"any":[{"op":"eq","value":"c"}]},"step":"s5"}}`),
		];
		// j2's step s2 is hidden by j1's jump → suppressed; s4 stays visible.
		const h = hiddenStepsByJump(fields, order, { j1: 'a', j2: 'b', j3: 'c' });
		expect([...h].sort()).toEqual(['s2', 's3']);
	});

	it('union of intervals from visible sources', () => {
		const fields = [
			mk('j1', 's1', `{"jump":{"when":{"any":[{"op":"eq","value":"a"}]},"step":"s3"}}`),
			mk('j2', 's2', `{"jump":{"when":{"any":[{"op":"eq","value":"b"}]},"step":"s5"}}`),
			mk('j3', 's3', `{"jump":{"when":{"any":[{"op":"eq","value":"c"}]},"step":"s5"}}`),
		];
		const h = hiddenStepsByJump(fields, order, { j1: 'a', j2: 'b', j3: 'c' });
		expect([...h].sort()).toEqual(['s2', 's4']);
	});

	it('seeded stepHidden suppresses its fields\' jumps', () => {
		const fields = [
			mk('j1', 's1', `{"jump":{"when":{"any":[{"op":"eq","value":"a"}]},"step":"s3"}}`),
			mk('j3', 's3', `{"jump":{"when":{"any":[{"op":"eq","value":"c"}]},"step":"s5"}}`),
		];
		const h = hiddenStepsByJump(fields, order, { j1: 'a', j3: 'c' }, new Set(['s3']));
		expect([...h].sort()).toEqual(['s2', 's3']);
	});

	it('when-less jump fires iff the owning field is answered', () => {
		const fields = [mk('u1', 's1', `{"jump":{"step":"s3"}}`)];
		expect([...hiddenStepsByJump(fields, order, { u1: 'x' })].sort()).toEqual(['s2']);
		expect(hiddenStepsByJump(fields, order, {}).size).toBe(0);
		expect(hiddenStepsByJump(fields, order, { u1: '' }).size).toBe(0);
	});
});
