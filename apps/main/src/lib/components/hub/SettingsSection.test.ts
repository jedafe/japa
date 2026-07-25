import { render } from '@testing-library/svelte';
import { it, expect } from 'vitest';
import SettingsSection from './SettingsSection.svelte';
import { DirtyForm } from './useDirtyForm.svelte';
import type { Snippet } from 'svelte';

type Res = { data: unknown; error: { message: string; status: number } | null; status: number };

const noopChildren = undefined as unknown as Snippet;

it('hides the save footer until the form is dirty', async () => {
	const form = new DirtyForm<Record<string, unknown>>(
		{ name: 'Jane' },
		async () => ({ data: {}, error: null, status: 200 }) as Res
	);
	const { queryByRole, rerender } = render(SettingsSection, {
		props: { titleKey: 'settings.nav.profile', form, children: noopChildren }
	});
	expect(queryByRole('button', { name: /save/i })).toBeNull();
	form.set({ name: 'Janet' });
	await rerender({ titleKey: 'settings.nav.profile', form, children: noopChildren });
	expect(queryByRole('button', { name: /save/i })).not.toBeNull();
});
