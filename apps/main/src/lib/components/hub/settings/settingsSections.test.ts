import { describe, it, expect } from 'vitest';
import { resolveSection, sectionsByGroup, SETTINGS_SECTIONS } from './settingsSections';

describe('settings registry', () => {
	it('resolves a known slug and returns null for unknown', () => {
		expect(resolveSection('profile')?.slug).toBe('profile');
		expect(resolveSection('nope')).toBeNull();
	});
	it('flags devices backend-ready', () => {
		expect(resolveSection('devices')?.backendReady).toBe(true);
	});
	it('marks profile/account/privacy backend-ready', () => {
		for (const s of ['profile', 'account', 'privacy']) {
			expect(resolveSection(s)?.backendReady).toBe(true);
		}
	});
	it('groups sections and puts danger last', () => {
		const groups = sectionsByGroup().map(([g]) => g);
		expect(groups[groups.length - 1]).toBe('danger');
	});
	it('every section has an i18n label key', () => {
		for (const s of SETTINGS_SECTIONS) expect(s.labelKey.startsWith('settings.nav.')).toBe(true);
	});
});
