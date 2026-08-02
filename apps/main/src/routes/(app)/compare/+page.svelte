<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { GraduationCap, GitCompare, Loader2 } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { EmptyState } from '$lib/components/ui';
	import type { University, UniversityCompare } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let selected = $state<string[]>([]);
	let comparing = $state(false);
	let compareData = $state<Record<string, UniversityCompare>>({});

	const title = $derived(t('compare.title', 'Compare universities — JapaConnect'));
	const description = $derived(
		t('compare.description', 'Compare universities side by side to find the right fit.')
	);

	function toggle(id: string) {
		if (selected.includes(id)) {
			selected = selected.filter((s) => s !== id);
		} else if (selected.length < 4) {
			selected = [...selected, id];
		}
	}

	async function runCompare() {
		if (selected.length < 2 || comparing) return;
		comparing = true;
		compareData = {};
		const results = await Promise.all(
			selected.map(async (id) => {
				const res = await api.get<UniversityCompare>(`/universities/${id}/compare`);
				return [id, res.data] as const;
			})
		);
		compareData = Object.fromEntries(
			results.filter(([, v]) => v !== null) as [string, UniversityCompare][]
		);
		comparing = false;
	}

	// Collect all unique field keys across selected universities for the table.
	const fieldKeys = $derived(
		Array.from(
			new Set(
				Object.values(compareData).flatMap((c) => (c ? Object.keys(c.fields) : []))
			)
		).sort()
	);

	const selectedUniversities = $derived(
		selected
			.map((id) => data.universities.find((u) => u.id === id))
			.filter((u): u is University => !!u)
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<GitCompare size={20} /> {t('compare.heading', 'Compare universities')}
		</h1>
	</div>

	<div class="p-3">
		{#if data.universities.length === 0}
			<EmptyState icon={GraduationCap} title={t('compare.no_universities', 'No universities to compare')} />
		{:else}
			<!-- Picker -->
			<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<p class="mb-3 text-sm text-[var(--color-text-subtle)]">
					{t('compare.select_hint', 'Select 2-4 universities to compare.')}
				</p>
				<div class="grid gap-2 sm:grid-cols-2">
					{#each data.universities as uni (uni.id)}
						{@const checked = selected.includes(uni.id)}
						<label
							class="flex cursor-pointer items-center gap-3 rounded-[var(--radius)] border p-3 transition-colors {checked ? 'border-[var(--color-primary)] bg-[color-mix(in_srgb,var(--color-primary)_5%,transparent)]' : 'border-[var(--color-border)] hover:bg-[var(--color-surface-subtle)]'}"
						>
							<input type="checkbox" checked={checked} onchange={() => toggle(uni.id)} class="h-4 w-4 accent-[var(--color-primary)]" />
							<span class="grid h-9 w-9 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]"><GraduationCap size={18} /></span>
							<span class="min-w-0 flex-1">
								<span class="block truncate text-sm font-medium text-[var(--color-text)]">{uni.name}</span>
								<span class="block text-xs text-[var(--color-muted)]">{uni.member_count} {t('meta.members', 'members')}</span>
							</span>
						</label>
					{/each}
				</div>
				<button
					type="button"
					onclick={runCompare}
					disabled={selected.length < 2 || comparing}
					class="mt-4 inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
				>
					{#if comparing}<Loader2 size={16} class="animate-spin" />{:else}<GitCompare size={16} />{/if}
					{t('compare.action', 'Compare')}
				</button>
			</section>

			<!-- Comparison table -->
			{#if Object.keys(compareData).length >= 2}
				<section class="mt-4 overflow-x-auto rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
					<table class="w-full text-sm">
						<thead>
							<tr class="border-b border-[var(--color-border)]">
								<th class="p-3 text-left text-xs font-medium uppercase text-[var(--color-muted)]">{t('compare.attribute', 'Attribute')}</th>
								{#each selectedUniversities as uni (uni.id)}
									<th class="p-3 text-left text-sm font-medium text-[var(--color-text)]">
										<a href="/spaces/{uni.slug}" class="hover:text-[var(--color-primary)]">{uni.name}</a>
									</th>
								{/each}
							</tr>
						</thead>
						<tbody>
							<tr class="border-b border-[var(--color-border)]">
								<td class="p-3 text-[var(--color-muted)]">{t('compare.members', 'Members')}</td>
								{#each selectedUniversities as uni (uni.id)}
									<td class="p-3 text-[var(--color-text)]">{uni.member_count}</td>
								{/each}
							</tr>
							<tr class="border-b border-[var(--color-border)]">
								<td class="p-3 text-[var(--color-muted)]">{t('compare.posts', 'Posts')}</td>
								{#each selectedUniversities as uni (uni.id)}
									<td class="p-3 text-[var(--color-text)]">{uni.post_count}</td>
								{/each}
							</tr>
							<tr class="border-b border-[var(--color-border)]">
								<td class="p-3 text-[var(--color-muted)]">{t('compare.followers', 'Followers')}</td>
								{#each selectedUniversities as uni (uni.id)}
									<td class="p-3 text-[var(--color-text)]">{uni.follower_count}</td>
								{/each}
							</tr>
							{#each fieldKeys as key (key)}
								<tr class="border-b border-[var(--color-border)]">
									<td class="p-3 capitalize text-[var(--color-muted)]">{key.replace(/_/g, ' ')}</td>
									{#each selectedUniversities as uni (uni.id)}
										<td class="p-3 text-[var(--color-text)]">{String(compareData[uni.id]?.fields[key] ?? '—')}</td>
									{/each}
								</tr>
							{/each}
						</tbody>
					</table>
				</section>
			{/if}
		{/if}
	</div>
</div>
