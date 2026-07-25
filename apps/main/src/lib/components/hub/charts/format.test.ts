import { describe, it, expect } from 'vitest';
import { formatMetric } from './format';

describe('formatMetric', () => {
	it('rounds to the nearest integer', () => {
		expect(formatMetric(1234.56)).toBe('1,235');
		expect(formatMetric(0.4)).toBe('0');
	});
	it('adds thousands separators', () => {
		expect(formatMetric(1000000)).toBe('1,000,000');
		expect(formatMetric(12345)).toBe('12,345');
	});
	it('handles zero and negative numbers', () => {
		expect(formatMetric(0)).toBe('0');
		expect(formatMetric(-1234)).toBe('-1,234');
	});
});
