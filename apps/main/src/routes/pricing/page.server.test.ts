import { describe, it, expect, vi } from 'vitest';
import { load } from './+page.server';

// The Go backend wraps every payload in a {"data": ...} envelope, and
// $lib/server/api's apiFetch returns the whole parsed body as `data`.
vi.mock('$lib/server/api', () => ({
	apiFetch: vi.fn().mockResolvedValue({
		ok: true,
		data: {
			data: [
				{
					id: 'free',
					name: 'Free',
					description: 'Get started',
					features: [{ text: 'Community forum access', included: true }],
					monthly: { amount: 0, currency: 'NGN' },
					yearly: { amount: 0, currency: 'NGN' },
					highlighted: false
				}
			]
		}
	})
}));

function makeEvent() {
	return {
		cookies: { get: () => undefined }
	} as unknown as Parameters<typeof load>[0];
}

describe('pricing +page.server load', () => {
	it('unwraps the API envelope so plans is an array', async () => {
		const result = (await load(makeEvent())) as { plans: Array<{ id: string }> };
		expect(Array.isArray(result.plans)).toBe(true);
		expect(result.plans).toHaveLength(1);
		expect(result.plans[0].id).toBe('free');
	});
});
