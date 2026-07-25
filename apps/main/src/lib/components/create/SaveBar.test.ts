import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import SaveBar from './SaveBar.svelte';

describe('SaveBar', () => {
	it('disables Publish when canPublish is false and shows the reason', () => {
		const { getByRole, getByText } = render(SaveBar, {
			props: {
				status: 'saved', savedAt: Date.now(), canPublish: false,
				publishReason: 'create.gate.space', onSaveDraft: () => {}, onPublish: () => {}
			}
		});
		expect((getByRole('button', { name: /publish/i }) as HTMLButtonElement).disabled).toBe(true);
		// reason is rendered (as its i18n key text via the test t() fallback)
		expect(getByText(/space/i)).toBeTruthy();
	});
	it('enables Publish when canPublish is true', () => {
		const { getByRole } = render(SaveBar, {
			props: { status: 'saved', savedAt: Date.now(), canPublish: true, publishReason: null, onSaveDraft: () => {}, onPublish: () => {} }
		});
		expect((getByRole('button', { name: /publish/i }) as HTMLButtonElement).disabled).toBe(false);
	});
});
