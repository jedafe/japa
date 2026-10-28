<script lang="ts">
	import { t } from '$lib/i18n';
	import { changePassword } from '$lib/api/hub';
	import { validatePassword } from '../security';

	let current = $state('');
	let next = $state('');
	let confirm = $state('');
	let error = $state<string | null>(null);
	let success = $state(false);
	let saving = $state(false);

	async function submit() {
		error = null;
		success = false;
		const v = validatePassword(current, next, confirm);
		if (!v.ok) {
			error = t(v.reason ?? 'settings.security.err_weak');
			return;
		}
		saving = true;
		const res = await changePassword(current, next);
		saving = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		success = true;
		current = '';
		next = '';
		confirm = '';
		// §02: the backend revokes all of the user's refresh tokens and the
		// caller's access-token JTI on password change — this session is
		// already dead, so staying on the page leaves every subsequent API
		// call 401ing. Send the user through login and back here.
		setTimeout(() => {
			window.location.assign(`/login?redirect=${encodeURIComponent('/settings/security')}`);
		}, 1500);
	}
</script>

<form class="space-y-3" onsubmit={(e) => { e.preventDefault(); submit(); }}>
	<label class="block">
		<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.security.current')}</span>
		<input
			type="password"
			aria-describedby="cp-current-hint"
			class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
			value={current}
			oninput={(e) => (current = e.currentTarget.value)}
		/>
		<span id="cp-current-hint" class="mt-1 block text-xs text-[var(--color-muted)]">
			{t('settings.security.current_hint', 'Your existing account password.')}
		</span>
	</label>
	<label class="block">
		<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.security.new')}</span>
		<input
			type="password"
			aria-describedby="cp-new-hint"
			class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
			value={next}
			oninput={(e) => (next = e.currentTarget.value)}
		/>
		<span id="cp-new-hint" class="mt-1 block text-xs text-[var(--color-muted)]">
			{t('settings.security.new_hint', 'At least 8 characters with upper & lower case letters and a number.')}
		</span>
	</label>
	<label class="block">
		<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.security.confirm')}</span>
		<input
			type="password"
			aria-describedby="cp-confirm-hint"
			class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
			value={confirm}
			oninput={(e) => (confirm = e.currentTarget.value)}
		/>
		<span id="cp-confirm-hint" class="mt-1 block text-xs text-[var(--color-muted)]">
			{t('settings.security.confirm_hint', 'Re-enter your new password exactly.')}
		</span>
	</label>
	{#if error}<p class="text-xs text-[var(--color-danger,#dc2626)]">{error}</p>{/if}
	{#if success}<p class="text-xs text-green-600">{t('settings.security.changed')}</p>{/if}
	<button
		type="submit"
		disabled={saving}
		class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
	>
		{saving ? t('settings.security.saving') : t('settings.security.change')}
	</button>
</form>
