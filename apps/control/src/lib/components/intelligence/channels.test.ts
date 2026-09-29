import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import ChannelsTable from './ChannelsTable.svelte';
import ChannelEditor from './ChannelEditor.svelte';

const apiMock = vi.fn().mockResolvedValue({ ok: true, data: [] });

vi.mock('$lib/api', () => ({
	api: (...args: unknown[]) => apiMock(...args)
}));

vi.mock('$lib/stores/auth.svelte', () => ({
	hasAnyPermission: vi.fn(() => true)
}));

const source = { id: 'src-1', name: 'DAAD', enabled: true };

describe('ChannelsTable.svelte', () => {
	it('lists channels for the source when opened', async () => {
		render(ChannelsTable, { props: { open: true, source } });
		expect(apiMock).toHaveBeenCalledWith('/api/v1/admin/intel/sources/src-1/channels');
	});

	it('shows the empty state when the source has no channels', async () => {
		render(ChannelsTable, { props: { open: true, source } });
		expect(
			await screen.findByText('No channels yet — add one or re-probe the source to auto-detect them.')
		).toBeTruthy();
	});

	it('renders the re-probe action', async () => {
		render(ChannelsTable, { props: { open: true, source } });
		expect(await screen.findByText('Re-probe')).toBeTruthy();
	});
});

describe('ChannelEditor.svelte', () => {
	it('renders the add-channel dialog fields', () => {
		render(ChannelEditor, {
			props: {
				open: true,
				pipelineOptions: ['extract', 'curate', 'hybrid'],
				entityTypeOptions: ['scholarship', 'job'],
				onSubmit: vi.fn()
			}
		});
		expect(screen.getByText('Add Channel')).toBeTruthy();
		expect(screen.getByText('Entry URL')).toBeTruthy();
		expect(screen.getByText('Pipeline')).toBeTruthy();
		expect(screen.getByText('Relevance mode')).toBeTruthy();
	});

	it('shows relevance topics and URL patterns in filtered mode', () => {
		render(ChannelEditor, {
			props: {
				open: true,
				channel: { id: 'ch-1', relevance_mode: 'filtered', relevance_topics: ['job', 'career'] },
				onSubmit: vi.fn()
			}
		});
		expect(screen.getByLabelText(/Relevance topics/)).toBeTruthy();
		expect(screen.getByLabelText(/Relevance URL patterns/)).toBeTruthy();
	});

	it('rejects invalid provider_config JSON instead of submitting', async () => {
		const onSubmit = vi.fn();
		render(ChannelEditor, {
			props: { open: true, onSubmit }
		});
		const textarea = screen.getByLabelText(/Provider config/);
		await fireEvent.input(textarea, { target: { value: '{bad json' } });
		await fireEvent.input(screen.getByLabelText(/Label/), { target: { value: 'Feed' } });
		await fireEvent.input(screen.getByLabelText(/Entry URL/), { target: { value: 'https://x.test/feed' } });
		// Submit button stays disabled while the JSON is invalid only after
		// validation runs — trigger it via the submit handler path.
		const btn = screen.getByText('Create Channel');
		await fireEvent.click(btn);
		expect(onSubmit).not.toHaveBeenCalled();
	});
});
