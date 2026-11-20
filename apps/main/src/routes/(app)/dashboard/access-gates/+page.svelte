<script lang="ts">
	// Ref: japaconnect-engineering/sections/123-ops-access-gates.md
	// Super-admin reachability toggles: stopping a gate removes its public
	// route entirely (mail flow is unaffected — Stalwart is never a target).
	import SeoHead from '#lib/seo/SeoHead.svelte';
	import { enhance } from '$app/forms';
	import { invalidateAll } from '$app/navigation';
	import { t } from '#lib/i18n/index.js';
	import EmptyState from '#lib/components/ui/EmptyState.svelte';
	import { Shield, ShieldOff, RefreshCw, ExternalLink, TriangleAlert } from 'lucide-svelte';
	import { onDestroy } from 'svelte';
	import type { AccessGate } from './+page.server';

	let { data, form } = $props();

	const gates: AccessGate[] = $derived(data.gates);
	let pending = $state<string | null>(null);

	const STATE_STYLE: Record<string, string> = {
		on: 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400',
		off: 'bg-[var(--color-surface-subtle)] text-[var(--color-muted)]',
		transitioning: 'bg-amber-500/10 text-amber-600 dark:text-amber-400',
		unknown: 'bg-red-500/10 text-red-600 dark:text-red-400'
	};

	const STATE_LABEL: Record<string, string> = {
		on: 'access_gates.state.on',
		off: 'access_gates.state.off',
		transitioning: 'access_gates.state.transitioning',
		unknown: 'access_gates.state.unknown'
	};

	// While any gate is converging, refresh the list periodically — Coolify
	// applies start/stop asynchronously so "transitioning" resolves on poll.
	let poll: ReturnType<typeof setInterval> | null = null;
	$effect(() => {
		const active = gates.some((g) => g.state === 'transitioning') || pending !== null;
		if (active && !poll) {
			poll = setInterval(() => invalidateAll(), 4000);
		} else if (!active && poll) {
			clearInterval(poll);
			poll = null;
		}
	});
	onDestroy(() => {
		if (poll) clearInterval(poll);
	});
</script>

<SeoHead title={t('access_gates.page_title')} robots="noindex" />

<section class="space-y-4">
	<header>
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('access_gates.page_title')}</h1>
		<p class="mt-0.5 text-sm text-[var(--color-muted)]">{t('access_gates.subtitle')}</p>
	</header>

	{#if !data.featureEnabled}
		<div
			class="flex items-start gap-2.5 rounded-[var(--radius-card)] border border-amber-500/40 bg-amber-500/10 p-4 text-sm text-amber-700 dark:text-amber-300"
		>
			<TriangleAlert class="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
			{t('access_gates.not_configured')}
		</div>
	{:else if gates.length === 0}
		<EmptyState title={t('access_gates.empty')} />
	{:else}
		{#if form?.message}
			<div
				class="rounded-[var(--radius-card)] border border-red-500/40 bg-red-500/10 p-4 text-sm text-red-600 dark:text-red-400"
			>
				{form.message}
			</div>
		{/if}

		<ul class="space-y-2">
			{#each gates as gate (gate.key)}
				{@const on = gate.state === 'on'}
				<li
					class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
				>
					<div class="flex flex-wrap items-center justify-between gap-3">
						<div class="min-w-0 flex-1">
							<div class="flex items-center gap-2">
								<p class="truncate text-sm font-medium text-[var(--color-text)]">{gate.label}</p>
								<span
									class="shrink-0 rounded-full px-2 py-0.5 text-xs font-medium {STATE_STYLE[gate.state] ??
										STATE_STYLE.unknown}"
								>
									{t(STATE_LABEL[gate.state] ?? STATE_LABEL.unknown)}
								</span>
								{#if gate.route_reachable !== undefined}
									<span
										class="inline-flex shrink-0 items-center gap-1 text-xs {gate.route_reachable
											? 'text-emerald-600 dark:text-emerald-400'
											: 'text-red-600 dark:text-red-400'}"
									>
										<span
											class="h-1.5 w-1.5 rounded-full {gate.route_reachable
												? 'bg-emerald-500'
												: 'bg-red-500'}"
											aria-hidden="true"
										></span>
										{t(gate.route_reachable ? 'access_gates.route_up' : 'access_gates.route_down')}
										{#if gate.route_status}<span class="text-[var(--color-muted)]/70">({gate.route_status})</span>{/if}
									</span>
								{/if}
							</div>
							<p class="mt-1 flex flex-wrap items-center gap-2 text-xs text-[var(--color-muted)]">
								{#if gate.domain}
									<span class="inline-flex items-center gap-1">
										{gate.domain}
										{#if on}
											<a
												href="https://{gate.domain}"
												target="_blank"
												rel="noopener noreferrer"
												class="text-[var(--color-primary)] hover:underline"
												aria-label={t('access_gates.open_link')}
											>
												<ExternalLink class="h-3 w-3" />
											</a>
										{/if}
									</span>
								{/if}
								{#if gate.status}
									<span class="text-[var(--color-muted)]/70">· {gate.status}</span>
								{/if}
							</p>
						</div>
						<form
							method="POST"
							action="?/toggle"
							use:enhance={() => {
								pending = gate.key;
								return async ({ update }) => {
									pending = null;
									await update();
								};
							}}
						>
							<input type="hidden" name="key" value={gate.key} />
							<button
								type="submit"
								disabled={pending !== null}
								class="inline-flex items-center gap-1.5 rounded-[var(--radius)] px-3 py-1.5 text-sm font-medium transition {on
									? 'border border-[var(--color-border)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'
									: 'bg-[var(--color-primary)] text-white hover:opacity-90'} disabled:cursor-not-allowed disabled:opacity-50"
							>
								{#if pending === gate.key}
									<RefreshCw class="h-4 w-4 animate-spin" aria-hidden="true" />
								{:else if on}
									<ShieldOff class="h-4 w-4" aria-hidden="true" />
								{:else}
									<Shield class="h-4 w-4" aria-hidden="true" />
								{/if}
								{pending === gate.key
									? t('access_gates.working')
									: on
										? t('access_gates.take_offline')
										: t('access_gates.bring_online')}
							</button>
						</form>
					</div>
				</li>
			{/each}
		</ul>

		<p class="text-xs text-[var(--color-muted)]">{t('access_gates.note')}</p>
	{/if}
</section>
