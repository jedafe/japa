import { describe, it, expect } from 'vitest';
import { resolveView, DEFAULT_VIEW } from './dashboardViews';

describe('dashboard registry', () => {
	it('defaults to overview and resolves known views', () => {
		expect(DEFAULT_VIEW).toBe('overview');
		expect(resolveView('content')?.slug).toBe('content');
		expect(resolveView('nope')).toBeNull();
	});
	it('flags analytics as backend-ready', () => {
		expect(resolveView('analytics')?.backendReady).toBe(true);
	});
});
