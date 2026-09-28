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
	it('requires >=1 item for a list', () => {
		expect(evaluatePublish({ ...base, formKind: 'list', itemCount: 0 }).reason).toBe('create.gate.list_items');
		expect(evaluatePublish({ ...base, formKind: 'list', itemCount: 1 }).ok).toBe(true);
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
	it('matches the whole /studio surface', () => {
		expect(isExceptionRoute('/(app)/studio/create')).toBe(true);
		expect(isExceptionRoute('/(app)/studio/videos')).toBe(true);
		expect(isExceptionRoute('/(app)/studio/prompts')).toBe(true);
		expect(isExceptionRoute('/(app)/studio/contents')).toBe(true);
		expect(isExceptionRoute('/(app)/studio')).toBe(true);
		expect(isExceptionRoute('/(app)/create')).toBe(true);
	});
	it('matches CPT single routes including comparisons', () => {
		expect(isExceptionRoute('/(app)/comparisons/[slug]')).toBe(true);
		expect(isExceptionRoute('/(app)/story/[slug]')).toBe(true);
	});
	it('matches the video single route (§106)', () => {
		expect(isExceptionRoute('/(app)/video/[slug]')).toBe(true);
	});
	it('matches the list single route (§14 list amendment)', () => {
		expect(isExceptionRoute('/(app)/list/[slug]')).toBe(true);
	});
	it('does not match the home route', () => {
		expect(isExceptionRoute('/(app)')).toBe(false);
		expect(isExceptionRoute(null)).toBe(false);
	});
});
