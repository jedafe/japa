import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import RevisionDiff from './RevisionDiff.svelte';

const getEntityFields = vi.fn(async (_type?: string, _id?: string): Promise<{ ok: true; data: unknown } | { ok: false; error: string }> => ({
	ok: true,
	data: {
		groups: [
			{
				id: 'g1',
				slug: 'costs',
				name: 'Costs',
				fields: [{ id: 'f1', key: 'tuition', label: 'Tuition', field_type: 'number', value: 5000 }]
			}
		],
		pending_count: 1
	}
}));

vi.mock('$lib/api', () => ({
	wikiAdminApi: {
		getEntityFields: (type: string, id: string) => getEntityFields(type, id)
	}
}));

const revision = {
	id: 'rev-1',
	kind: 'fields',
	ops: [
		{
			op: 'set_value',
			entity_type: 'university',
			entity_id: '11111111-2222-3333-4444-555555555555',
			field_key: 'tuition',
			value: 4200,
			source_url: 'https://example.com/source'
		},
		{
			op: 'propose_field',
			entity_class: 'university',
			field: { key: 'dorms', label: 'Dorms', field_type: 'text' }
		}
	],
	target_entity_type: 'university',
	target_entity_id: '11111111-2222-3333-4444-555555555555'
};

describe('RevisionDiff', () => {
	it('renders each op row with its field label', async () => {
		render(RevisionDiff, { revision });
		await waitFor(() => expect(screen.getByText('set_value')).toBeTruthy());
		expect(screen.getByText('propose_field')).toBeTruthy();
		expect(screen.getByText('tuition')).toBeTruthy();
		expect(screen.getByText('dorms')).toBeTruthy();
	});

	it('fetches entity fields and shows current → proposed for set_value', async () => {
		render(RevisionDiff, { revision });
		await waitFor(() => expect(screen.getByText('5000')).toBeTruthy());
		expect(screen.getByText('4200')).toBeTruthy();
		expect(getEntityFields).toHaveBeenCalledWith(
			'university',
			'11111111-2222-3333-4444-555555555555'
		);
	});

	it('shows the proposed value alone when the fields fetch fails', async () => {
		getEntityFields.mockResolvedValueOnce({ ok: false, error: 'nope' });
		render(RevisionDiff, { revision });
		await waitFor(() => expect(getEntityFields).toHaveBeenCalled());
		expect(screen.getByText('4200')).toBeTruthy();
		expect(screen.queryByText('5000')).toBeNull();
	});

	it('renders an empty state when the revision has no ops', () => {
		render(RevisionDiff, { revision: { id: 'rev-2', kind: 'fields', ops: [] } });
		expect(screen.getByText(/no field ops/i)).toBeTruthy();
	});
});
