import { describe, it, expect } from 'vitest';
import { load } from './+page.server';

describe('Docs landing page', () => {
	it('redirects to /users', async () => {
		try {
			await load({} as never);
		} catch (e: unknown) {
			expect((e as { status: number }).status).toBe(302);
			expect((e as { location: string }).location).toBe('/users');
		}
	});
});
