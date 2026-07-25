import { describe, it, expect } from 'vitest';
import { suggestSlug } from './slug';

describe('suggestSlug', () => {
	it('lowercases and hyphenates', () => {
		expect(suggestSlug('Hello World')).toBe('hello-world');
	});
	it('strips punctuation and collapses separators', () => {
		expect(suggestSlug('  Study in the UK: A Guide!! ')).toBe('study-in-the-uk-a-guide');
	});
	it('keeps digits and drops diacritics-adjacent symbols', () => {
		expect(suggestSlug('Top 10 — Universities')).toBe('top-10-universities');
	});
	it('returns empty string for empty/symbol-only input', () => {
		expect(suggestSlug('   ')).toBe('');
		expect(suggestSlug('!!!')).toBe('');
	});
	it('truncates to 80 chars without a trailing hyphen', () => {
		const long = 'a'.repeat(100);
		const out = suggestSlug(long);
		expect(out.length).toBeLessThanOrEqual(80);
		expect(out.endsWith('-')).toBe(false);
	});
});
