import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import SaveBar from './SaveBar.svelte';

const botAuthors = [
	{ id: 'bot-1', label: 'Japa Bot', sublabel: '@japabot' },
	{ id: 'bot-2', label: 'News Bot', sublabel: '@newsbot' }
];

describe('SaveBar', () => {
	it('disables Publish when canPublish is false and shows the reason', () => {
		const { getByRole, getByText } = render(SaveBar, {
			props: {
				status: 'saved', savedAt: Date.now(), canPublish: false,
				publishReason: 'create.gate.space',
				orgs: [], selectedOrgId: null, onSelectOrg: () => {},
				onSaveDraft: () => {}, onPublish: () => {}
			}
		});
		expect((getByRole('button', { name: /publish/i }) as HTMLButtonElement).disabled).toBe(true);
		// reason is rendered (as its i18n key text via the test t() fallback)
		expect(getByText(/space/i)).toBeTruthy();
	});
	it('enables Publish when canPublish is true', () => {
		const { getByRole } = render(SaveBar, {
			props: {
				status: 'saved', savedAt: Date.now(), canPublish: true, publishReason: null,
				orgs: [], selectedOrgId: null, onSelectOrg: () => {},
				onSaveDraft: () => {}, onPublish: () => {}
			}
		});
		expect((getByRole('button', { name: /publish/i }) as HTMLButtonElement).disabled).toBe(false);
	});

	it('hides the persona picker until post-as-persona is selected', () => {
		const { getAllByRole } = render(SaveBar, {
			props: {
				status: 'saved', savedAt: Date.now(), canPublish: true, publishReason: null,
				orgs: [], selectedOrgId: null, onSelectOrg: () => {},
				botAuthors, selectedAuthorId: null, onSelectAuthor: () => {},
				personaMode: false,
				onSaveDraft: () => {}, onPublish: () => {}
			}
		});
		// Only the unified post-as combobox renders — the persona picker is
		// gated behind persona mode.
		expect(getAllByRole('combobox')).toHaveLength(1);
	});

	it('shows the persona picker in persona mode', () => {
		const { getAllByRole, getByText } = render(SaveBar, {
			props: {
				status: 'saved', savedAt: Date.now(), canPublish: true, publishReason: null,
				orgs: [], selectedOrgId: null, onSelectOrg: () => {},
				botAuthors, selectedAuthorId: 'bot-1', onSelectAuthor: () => {},
				personaMode: true,
				onSaveDraft: () => {}, onPublish: () => {}
			}
		});
		expect(getAllByRole('combobox')).toHaveLength(2);
		expect(getByText('Post as a persona')).toBeTruthy();
		expect(getByText('Japa Bot')).toBeTruthy();
	});

	it('selecting "Post as a persona" enters persona mode and clears the org', async () => {
		const onPersonaMode = vi.fn();
		const onSelectOrg = vi.fn();
		const { getAllByRole, findByRole } = render(SaveBar, {
			props: {
				status: 'saved', savedAt: Date.now(), canPublish: true, publishReason: null,
				orgs: [{ id: 'org-1', name: 'Acme Org', slug: 'acme-org', member_role: 'member' }],
				selectedOrgId: null, onSelectOrg,
				botAuthors, selectedAuthorId: null, onSelectAuthor: () => {},
				personaMode: false, onPersonaMode,
				onSaveDraft: () => {}, onPublish: () => {}
			}
		});
		const [postAs] = getAllByRole('combobox');
		await fireEvent.focus(postAs);
		const personaOption = await findByRole('option', { name: /post as a persona/i });
		await fireEvent.mouseDown(personaOption);
		expect(onPersonaMode).toHaveBeenCalledWith(true);
		expect(onSelectOrg).toHaveBeenCalledWith(null);
	});
});
