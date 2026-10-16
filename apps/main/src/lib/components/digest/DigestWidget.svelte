<script lang="ts">
	// DigestWidget — surfaces the latest community digest on the Space home
	// page (§84, ADR-092). Fetches /digest/latest?dest={spaceId} and renders
	// a compact card with a link to the full digest page.
	import { page } from '$app/state';
	import { t, i18n } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { Newspaper, ChevronRight } from 'lucide-svelte';

	type DigestEdition = {
		id: string;
		space_id?: string;
		cadence: string;
		status: string;
		scheduled_for?: string;
		published_at?: string;
		created_at: string;
		content?: unknown;
	};

	interface Props {
		spaceId: string;
		// When provided from SSR (e.g. the Space page), the widget renders
		// without a client-side fetch, eliminating the 404 console noise when
		// no digest has been published yet.
		digest?: DigestEdition | null;
	}

	let { spaceId, digest: ssrDigest }: Props = $props();

	let clientDigest = $state<DigestEdition | null | undefined>(undefined);
	let clientResolved = $state(false);
	let errored = $state(false);

	const enabled = $derived(!!page.data.features?.['digest.enabled']);
	const digest = $derived(ssrDigest !== undefined ? ssrDigest : clientDigest);
	// When SSR provides the digest we can render immediately; otherwise wait
	// for the client-side fetch to resolve.
	const loading = $derived(ssrDigest === undefined && !clientResolved);

	// $effect keyed on spaceId — the component persists across client-side
	// space navigation, so a mount-only fetch would show the previous
	// space's digest (or none) on the new page. Client-only, same as onMount.
	$effect(() => {
		const id = spaceId;
		clientDigest = undefined;
		errored = false;
		if (!enabled || ssrDigest !== undefined) {
			// Feature off, or SSR provided the digest — no client fetch needed.
			clientResolved = true;
			return;
		}
		clientResolved = false;
		let cancelled = false;
		(async () => {
			try {
				const res = await api.get<DigestEdition>(`/digest/latest?dest=${encodeURIComponent(id)}`);
				if (cancelled) return;
				if (res.data) {
					clientDigest = res.data;
				} else if (res.error?.code === 'no_digest' || res.status === 404) {
					// No published digest for this destination yet — not a real error.
					clientDigest = null;
				} else if (res.error) {
					errored = true;
				}
			} catch {
				if (!cancelled) errored = true;
			} finally {
				if (!cancelled) clientResolved = true;
			}
		})();
		return () => { cancelled = true; };
	});
</script>

<!-- Render nothing when the digest feature flag is off — the SSR payload
     carries digest=null regardless of the flag, so without this gate the
     widget showed a dead "No digest available yet" card on every space
     (audit fix, mirrors the AchievementWidget hide-when-disabled pattern). -->
{#if !enabled}
	<!-- feature disabled -->
{:else if loading}
	<div class="animate-pulse rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="h-4 w-1/3 rounded bg-[var(--color-border)]"></div>
		<div class="mt-3 h-3 w-2/3 rounded bg-[var(--color-border)]"></div>
	</div>
{:else if digest}
	<a
		href="/digest/{digest.id}"
		class="block rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:bg-[var(--color-surface-subtle)]"
	>
		<div class="flex items-center justify-between">
			<div class="flex items-center gap-2">
				<Newspaper size={18} class="text-[var(--color-primary)]" />
				<span class="text-sm font-semibold text-[var(--color-foreground)]">
					{t('digest.widget_title', 'Community Digest')}
				</span>
			</div>
			<ChevronRight size={16} class="text-[var(--color-muted)]" />
		</div>
		<p class="mt-2 text-xs text-[var(--color-text-subtle)]">
			{digest.cadence === 'weekly'
				? t('digest.cadence_weekly', 'Weekly')
				: t('digest.cadence_monthly', 'Monthly')}
			{#if digest.published_at}
				· {new Date(digest.published_at).toLocaleDateString(i18n.locale)}
			{/if}
		</p>
	</a>
{:else if !errored}
	<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="flex items-center gap-2">
			<Newspaper size={18} class="text-[var(--color-muted)]" />
			<span class="text-sm font-semibold text-[var(--color-foreground)]">
				{t('digest.widget_title', 'Community Digest')}
			</span>
		</div>
		<p class="mt-2 text-xs text-[var(--color-text-subtle)]">
			{t('digest.widget_empty', 'No digest available yet')}
		</p>
	</div>
{/if}
