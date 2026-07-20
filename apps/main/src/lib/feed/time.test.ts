import { describe, it, expect } from 'vitest';
import { relativeTime } from './time';

describe('relativeTime', () => {
	const now = new Date('2026-07-12T09:03:55Z');

	it('parses the backend non-ISO "+0100 WAT" format', () => {
		// 08:03:54.888616 +0100 == 07:03:54.888616Z, ~2h (floored) before 09:03:55Z.
		// Kept sub-second precision to prove the parser handles the microsecond fraction.
		expect(relativeTime('2026-07-12 08:03:54.888616 +0100 WAT', now)).toBe('2h');
	});

	it('parses ISO input', () => {
		expect(relativeTime('2026-07-12T08:33:55Z', now)).toBe('30m');
	});

	it('returns "now" for very recent times', () => {
		expect(relativeTime('2026-07-12T09:03:30Z', now)).toBe('now');
	});

	it('formats days and weeks', () => {
		expect(relativeTime('2026-07-09T09:03:55Z', now)).toBe('3d');
		expect(relativeTime('2026-06-25T09:03:55Z', now)).toBe('2w');
	});

	it('returns empty string for unparseable input', () => {
		expect(relativeTime('', now)).toBe('');
		expect(relativeTime('not a date', now)).toBe('');
	});
});
