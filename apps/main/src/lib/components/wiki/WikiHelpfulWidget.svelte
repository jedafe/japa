<script lang="ts">
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md (Reader UX)
	// "Was this page helpful?" — per-member up/down votes with aggregate
	// counts. Re-clicking the active vote retracts it (server-side toggle).
	// Guests see a sign-in hint.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { ThumbsUp, ThumbsDown } from 'lucide-svelte';
	import type { WikiHelpful } from '$lib/types';

	let {
		spaceSlug,
		pageSlug,
		helpful,
		authed
	}: {
		spaceSlug: string;
		pageSlug: string;
		helpful: WikiHelpful | null | undefined;
		authed: boolean;
	} = $props();

	let votes = $state<WikiHelpful>({ up: 0, down: 0, my_vote: null });
	let loading = $state(false);
	let errorKey = $state('');

	// Keep the local copy in sync when the page data refreshes (navigation
	// to another wiki page reuses this component).
	$effect(() => {
		votes = { up: helpful?.up ?? 0, down: helpful?.down ?? 0, my_vote: helpful?.my_vote ?? null };
	});

	async function vote(v: 'up' | 'down') {
		if (loading || !authed) return;
		loading = true;
		errorKey = '';
		const res = await api.post<WikiHelpful>(
			`/wiki/${encodeURIComponent(spaceSlug)}/${encodeURIComponent(pageSlug)}/feedback`,
			{ vote: v }
		);
		loading = false;
		if (res.data) {
			votes = { up: res.data.up ?? 0, down: res.data.down ?? 0, my_vote: res.data.my_vote ?? null };
		} else {
			errorKey = res.error?.code ?? 'failed';
		}
	}

	const selected = $derived(votes.my_vote);
</script>

<section
	class="mt-8 flex flex-wrap items-center gap-3 rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3"
	aria-label={t('wiki.helpful_label', 'Was this page helpful?')}
>
	<p class="text-sm font-medium text-[var(--color-text)]">
		{t('wiki.helpful_label', 'Was this page helpful?')}
	</p>
	<div class="flex items-center gap-2">
		<button
			type="button"
			disabled={!authed || loading}
			onclick={() => vote('up')}
			aria-pressed={selected === 'up'}
			class="inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-60 {selected ===
			'up'
				? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
				: 'border-[var(--color-border)] text-[var(--color-text)] hover:border-[var(--color-primary)] hover:text-[var(--color-primary)]'}"
		>
			<ThumbsUp size={14} />
			{t('wiki.helpful_yes', 'Yes')}
			{#if votes.up > 0}
				<span class="text-[var(--color-muted)]">{votes.up}</span>
			{/if}
		</button>
		<button
			type="button"
			disabled={!authed || loading}
			onclick={() => vote('down')}
			aria-pressed={selected === 'down'}
			class="inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-60 {selected ===
			'down'
				? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
				: 'border-[var(--color-border)] text-[var(--color-text)] hover:border-[var(--color-primary)] hover:text-[var(--color-primary)]'}"
		>
			<ThumbsDown size={14} />
			{t('wiki.helpful_no', 'No')}
			{#if votes.down > 0}
				<span class="text-[var(--color-muted)]">{votes.down}</span>
			{/if}
		</button>
	</div>
	{#if !authed}
		<a href="/login" class="text-xs text-[var(--color-muted)] hover:text-[var(--color-primary)]">
			{t('wiki.helpful_signin', 'Sign in to vote')}
		</a>
	{:else if errorKey}
		<span class="text-xs text-[var(--color-danger)]" role="alert">
			{t('wiki.helpful_error', 'Could not save your vote. Please try again.')}
		</span>
	{/if}
</section>
