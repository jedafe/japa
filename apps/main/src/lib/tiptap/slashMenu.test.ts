import { describe, it, expect } from 'vitest';
import { SLASH_BLOCKS } from '@japaconnect/ui';

describe('SLASH_BLOCKS', () => {
	it('defines heading/list/quote/code/image blocks with i18n label keys', () => {
		const keys = SLASH_BLOCKS.map((b) => b.key);
		expect(keys).toEqual(
			expect.arrayContaining(['heading2', 'bulletList', 'blockquote', 'codeBlock', 'image'])
		);
		for (const b of SLASH_BLOCKS) {
			expect(b.labelKey.startsWith('create.slash.')).toBe(true);
			expect(typeof b.run).toBe('function');
		}
	});
});
