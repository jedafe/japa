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
	}
</script>

<form class="space-y-3" onsubmit={(e) => { e.preventDefault(); submit(); }}>
	<label class="block">
		<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.security.current')}</span>
		<input
			type="password"
			class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
			value={current}
			oninput={(e) => (current = e.currentTarget.value)}
		/>
	</label>
	<label class="block">
		<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.security.new')}</span>
		<input
			type="password"
			class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
			value={next}
			oninput={(e) => (next = e.currentTarget.value)}
		/>
	</label>
	<label class="block">
		<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.security.confirm')}</span>
		<input
			type="password"
			class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
			value={confirm}
			oninput={(e) => (confirm = e.currentTarget.value)}
		/>
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
