import { describe, it, expect } from 'vitest';
import { buildApiUrl } from './index';

describe('buildApiUrl', () => {
	it('does not double-prefix when base and path both include /api/v1', () => {
		expect(buildApiUrl('/api/v1/ai/models', '/api/v1')).toBe('/api/v1/ai/models');
		expect(buildApiUrl('/api/v1/ai/voices', '/api/v1')).toBe('/api/v1/ai/voices');
	});

	it('adds the /api/v1 prefix for short paths', () => {
		expect(buildApiUrl('/me/profile', '/api/v1')).toBe('/api/v1/me/profile');
	});

	it('appends /api/v1 to a base that lacks it', () => {
		expect(buildApiUrl('/me/profile', 'http://localhost:8080')).toBe(
			'http://localhost:8080/api/v1/me/profile'
		);
	});

	it('deduplicates when the base lacks /api/v1 but the path has it', () => {
		expect(buildApiUrl('/api/v1/ai/models', 'http://localhost:8080')).toBe(
			'http://localhost:8080/api/v1/ai/models'
		);
	});

	it('works with a full origin that already includes /api/v1', () => {
		expect(buildApiUrl('/api/v1/ai/models', 'http://localhost:8080/api/v1')).toBe(
			'http://localhost:8080/api/v1/ai/models'
		);
	});

	it('adds a leading slash to bare paths', () => {
		expect(buildApiUrl('me/profile', '/api/v1')).toBe('/api/v1/me/profile');
	});

	it('strips trailing slashes from the base', () => {
		expect(buildApiUrl('/ai/models', '/api/v1/')).toBe('/api/v1/ai/models');
	});

	it('lets an empty version disable prefix handling', () => {
		expect(buildApiUrl('/control/users', 'http://localhost:8080', { version: '' })).toBe(
			'http://localhost:8080/control/users'
		);
	});
});
