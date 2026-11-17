import { describe, it, expect } from 'vitest';
import { load } from './+page';

// §126 remote-follow landing — the ?uri= param must only forward
// acct:/https: actor identifiers downstream; javascript:, data:, http:,
// and bare handles are all refused before the visitor's instance is
// asked to resolve them.
describe('authorize_interaction uri filter', () => {
	const at = (q: string) =>
		load({ url: new URL(`https://japa.to/authorize_interaction${q}`) } as never);

	it('accepts acct: and https: actor identifiers', async () => {
		expect((await at('?uri=acct:alice@japa.to')).uri).toBe('acct:alice@japa.to');
		expect((await at('?uri=https://japa.to/users/alice')).uri).toBe('https://japa.to/users/alice');
	});

	it('refuses http: and non-web schemes', async () => {
		expect((await at('?uri=http://japa.to/users/alice')).uri).toBe('');
		expect((await at('?uri=javascript:alert(1)')).uri).toBe('');
		expect((await at('?uri=data:text/html,x')).uri).toBe('');
		expect((await at('?uri=ftp://host/x')).uri).toBe('');
	});

	it('refuses bare handles and empty params', async () => {
		expect((await at('?uri=alice@japa.to')).uri).toBe('');
		expect((await at('?uri=')).uri).toBe('');
		expect((await at('')).uri).toBe('');
	});
});
