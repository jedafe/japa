<script lang="ts">
	// Ref: docs/superpowers/plans/2026-09-11-sources-directory-v2-plan.md
	// Source block toggle. POST /sources/{slug}/block, DELETE /sources/{slug}/block.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';

	interface Props {
		slug: string;
		initial?: boolean;
		onChange?: (isBlocked: boolean) => void;
	}

	let { slug, initial = false, onChange }: Props = $props();

	// svelte-ignore state_referenced_locally
	let isBlocked = $state(initial);
	let loading = $state(false);
	let error = $state('');

	async function toggle() {
		if (loading) return;
		loading = true;
		error = '';
		const method = isBlocked ? 'delete' : 'post';
		try {
			const res = await api[method]<{ is_blocked: boolean }>(`/sources/${slug}/block`);
			isBlocked = res.data?.is_blocked ?? !isBlocked;
			onChange?.(isBlocked);
		} catch (err) {
			error = err instanceof Error ? err.message : t('sources.block.error', 'Block failed');
		} finally {
			loading = false;
		}
	}
</script>

<button
	onclick={toggle}
	disabled={loading}
	class="rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition hover:bg-[var(--color-surface-hover)] disabled:opacity-50"
>
	{isBlocked
		? t('sources.block.unblock', 'Unblock source')
		: t('sources.block.block', 'Block source')}
</button>

{#if error}
	<p class="mt-1 text-xs text-red-600">{error}</p>
{/if}
