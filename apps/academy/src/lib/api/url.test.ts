import { describe, it, expect } from 'vitest';
import { buildApiUrl } from '@japaconnect/api-url';

describe('buildApiUrl', () => {
	it('does not double-prefix /api/v1 when the path already includes it', () => {
		expect(buildApiUrl('/api/v1/ai/models', '/api/v1')).toBe('/api/v1/ai/models');
		expect(buildApiUrl('/api/v1/ai/voices', '/api/v1')).toBe('/api/v1/ai/voices');
	});

	it('adds the /api/v1 prefix for short paths', () => {
		expect(buildApiUrl('/me/profile', '/api/v1')).toBe('/api/v1/me/profile');
	});

	it('works with a full backend origin that already includes /api/v1', () => {
		expect(buildApiUrl('/api/v1/ai/models', 'http://localhost:8080/api/v1')).toBe(
			'http://localhost:8080/api/v1/ai/models'
		);
	});

	it('preserves /api/v1 in path when base does not include it', () => {
		expect(buildApiUrl('/api/v1/ai/models', 'http://localhost:8080')).toBe(
			'http://localhost:8080/api/v1/ai/models'
		);
	});

	it('adds a leading slash to bare paths', () => {
		expect(buildApiUrl('me/profile', '/api/v1')).toBe('/api/v1/me/profile');
	});

	it('strips trailing slashes from the base', () => {
		expect(buildApiUrl('/ai/models', '/api/v1/')).toBe('/api/v1/ai/models');
	});
});
