import { describe, it, expect } from 'vitest';
import { evaluatePublish, statusLabel, isExceptionRoute } from './gate';

describe('evaluatePublish', () => {
	const base = { title: 'Hi', primarySpaceId: 's1', requiredFieldsFilled: true, formKind: 'standard' as const, itemCount: 0, dimensionCount: 0 };
	it('passes a valid standard post', () => {
		expect(evaluatePublish({ ...base, formKind: 'standard' }).ok).toBe(true);
	});
	it('blocks with no title', () => {
		expect(evaluatePublish({ ...base, title: '  ' }).reason).toBe('create.gate.title');
	});
	it('blocks with no space', () => {
		expect(evaluatePublish({ ...base, primarySpaceId: null }).reason).toBe('create.gate.space');
	});
	it('blocks when required custom fields are unfilled', () => {
		expect(evaluatePublish({ ...base, requiredFieldsFilled: false }).reason).toBe('create.gate.fields');
	});
	it('requires >=2 items and >=1 dimension for a comparison', () => {
		expect(evaluatePublish({ ...base, formKind: 'comparison', itemCount: 1, dimensionCount: 1 }).reason).toBe('create.gate.items');
		expect(evaluatePublish({ ...base, formKind: 'comparison', itemCount: 2, dimensionCount: 0 }).reason).toBe('create.gate.dimensions');
		expect(evaluatePublish({ ...base, formKind: 'comparison', itemCount: 2, dimensionCount: 1 }).ok).toBe(true);
	});
});

describe('statusLabel', () => {
	it('maps each status to a key', () => {
		expect(statusLabel('saving')).toBe('create.status.saving');
		expect(statusLabel('saved')).toBe('create.status.saved');
		expect(statusLabel('error')).toBe('create.status.error');
		expect(statusLabel('scheduled')).toBe('create.status.scheduled');
	});
});

describe('isExceptionRoute', () => {
	it('matches the create route', () => {
		expect(isExceptionRoute('/(app)/create')).toBe(true);
	});
	it('does not match the home route', () => {
		expect(isExceptionRoute('/(app)')).toBe(false);
		expect(isExceptionRoute(null)).toBe(false);
	});
});
