import { describe, it, expect } from 'vitest';
import { matchesConfirmation } from './danger';

describe('matchesConfirmation', () => {
	it('matches the exact username (trimmed)', () => {
		expect(matchesConfirmation('  jane ', 'jane')).toBe(true);
	});
	it('is case-sensitive and rejects near-misses', () => {
		expect(matchesConfirmation('Jane', 'jane')).toBe(false);
		expect(matchesConfirmation('jan', 'jane')).toBe(false);
		expect(matchesConfirmation('', 'jane')).toBe(false);
	});
});
