<script lang="ts">
	import SeoHead from '$lib/seo/SeoHead.svelte';
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md (Entity Directory amendment)
	import { t, siteName } from '$lib/i18n';
	import { EntityHero } from '$lib/components/ui';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { api } from '$lib/api/client';
	import { invalidateAll } from '$app/navigation';
	import {
		ArrowRight,
		Award,
		BadgePercent,
		BookOpen,
		Building2,
		GraduationCap,
		Landmark,
		Package,
		Plane,
		Plus,
		School,
		Store,
		X
	} from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
	let kinds = $derived(data.kinds ?? []);
	let totalEntities = $derived(kinds.reduce((n, k) => n + k.entity_count, 0));
	let showCreate = $state(false);
	let creating = $state(false);
	let createErr = $state('');
	let form = $state<Record<string, string>>({});

	// New kind = a product type (§80 Product Domain) — POST provisions the
	// full chain: field groups → JCE comparison type → pool → wiki surfacing.
	async function submitCreate() {
		createErr = '';
		if (!form.slug?.trim() || !form.label?.trim()) {
			createErr = t('wiki.entities.err_kind_fields', 'Slug and label are required');
			return;
		}
		creating = true;
		const categories = (form.categories ?? '')
			.split('\n')
			.map((l) => l.trim())
			.filter(Boolean)
			.map((l) => {
				const [slug, ...rest] = l.split(':');
				return { slug: slug.trim(), label: (rest.join(':').trim() || slug.trim()) };
			});
		const res = await api.post('/admin/wiki/product-types', {
			slug: form.slug.trim(),
			label: form.label.trim(),
			icon: form.icon?.trim() || undefined,
			categories: categories.length ? categories : undefined
		});
		creating = false;
		if (res.error) { createErr = res.error.message || t('wiki.entities.err_create', 'Create failed'); return; }
		showCreate = false;
		form = {};
		await invalidateAll();
	}

	const KIND_ICONS: Record<string, typeof BookOpen> = {
		course: GraduationCap,
		program: School,
		bank: Landmark,
		embassy: Building2,
		visa_route: Plane,
		scholarship: Award,
		organization: Building2,
		product: Package,
		provider: Store,
		deal: BadgePercent
	};

	function kindIcon(kind: string) {
		return KIND_ICONS[kind] ?? BookOpen;
	}
</script>

<SeoHead title={`${t('wiki.entities.title', 'Entity Wiki')} — ${siteName()}`} description={t('wiki.entities.description', 'Community-maintained wiki entries for destinations, services, schools, and organisations — written and kept up to date by members.')} />

<div class="mx-auto w-full max-w-full" style="--page-gutter: max(1rem, calc((100% - 48rem)/2))">
	<EntityHero
		class="mt-4"
		icon={BookOpen}
		accent="var(--color-primary)"
		heading={t('wiki.entities.title', 'Entity Wiki')}
		description={t('wiki.entities.subtitle', 'Browse entities by kind — programs, course offerings, banks, embassies, and more.')}
		crumbs={[
			{ label: t('common.home', 'Home'), href: '/' },
			{ label: t('wiki.entities.title', 'Entity Wiki') }
		]}
		stats={[
			{ icon: BookOpen, value: String(totalEntities), label: t('wiki.entities.stat_entries', 'Entries') },
			{ icon: Package, value: String(kinds.length), label: t('wiki.entities.stat_kinds', 'Kinds') }
		]}
	/>

	{#if data.canManage}
		<div class="mt-4" style="margin-inline: var(--page-gutter)">
			{#if !showCreate}
				<button
					type="button"
					onclick={() => (showCreate = true)}
					class="inline-flex items-center gap-1.5 rounded-full border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-muted)] transition-colors hover:border-[var(--color-primary)]/60 hover:text-[var(--color-primary)]"
				>
					<Plus size={13} /> {t('wiki.entities.new_kind', 'New kind')}
				</button>
			{:else}
				<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<div class="mb-3 flex items-center justify-between">
						<p class="text-sm font-semibold text-[var(--color-text)]">{t('wiki.entities.new_kind', 'New kind')}</p>
						<button type="button" onclick={() => (showCreate = false)} class="text-[var(--color-muted)] hover:text-[var(--color-text)]"><X size={15} /></button>
					</div>
					<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
						<label class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)]">
							{t('wiki.entities.fld_slug', 'Slug')}
							<input bind:value={form.slug} class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]/60" placeholder="ai-agents" />
						</label>
						<label class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)]">
							{t('wiki.entities.fld_name', 'Label')}
							<input bind:value={form.label} class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]/60" placeholder="AI Agents" />
						</label>
						<label class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)] sm:col-span-2">
							{t('wiki.entities.fld_categories', 'Categories — one per line, slug: Label')}
							<textarea
								bind:value={form.categories}
								rows={3}
								class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]/60"
								placeholder={'overview: Overview\nspecs: Specs\npricing: Pricing'}
							></textarea>
						</label>
					</div>
					{#if createErr}
						<p class="mt-2 text-xs text-[var(--color-danger,#dc2626)]">{createErr}</p>
					{/if}
					<div class="mt-3 flex justify-end">
						<button
							type="button"
							onclick={submitCreate}
							disabled={creating}
							class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-opacity disabled:opacity-50"
						>
							{creating ? t('common.creating', 'Creating…') : t('common.create', 'Create')}
						</button>
					</div>
				</div>
			{/if}
		</div>
	{/if}

	{#if kinds.length === 0}
		<div class="mt-8" style="margin-inline: var(--page-gutter)">
			<EmptyState icon={BookOpen} title={t('wiki.entities.empty', 'No entity kinds available')} />
		</div>
	{:else}
		<div class="mt-6 grid grid-cols-1 gap-3 p-4 sm:grid-cols-2" style="margin-inline: var(--page-gutter)">
			{#each kinds as k (k.kind)}
				{@const Icon = kindIcon(k.kind)}
				<div class="flex flex-col gap-2">
					<a
						href="/wiki/e/{k.kind}"
						class="group flex items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:border-[var(--color-primary)]/60 focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary)]"
					>
						<span class="flex h-9 w-9 shrink-0 items-center justify-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-muted)] transition-colors group-hover:text-[var(--color-primary)]">
							<Icon size={18} />
						</span>
						<span class="min-w-0 flex-1">
							<span class="block truncate text-sm font-semibold text-[var(--color-text)]">{k.label}</span>
							<span class="mt-0.5 block text-xs text-[var(--color-muted)] tabular-nums">
								{t('wiki.entities.count', '{count} entries').replace('{count}', String(k.entity_count))}
							</span>
						</span>
						<ArrowRight size={16} class="shrink-0 text-[var(--color-muted)] transition-all group-hover:translate-x-0.5 group-hover:text-[var(--color-primary)]" />
					</a>
					{#if k.types?.length}
						<div class="flex flex-wrap gap-1.5 ps-1">
							{#each k.types as ty (ty.slug)}
								<a
									href="/wiki/e/{ty.slug}"
									class="inline-flex items-center gap-1 rounded-full border border-[var(--color-border)] px-2.5 py-1 text-[11px] font-medium text-[var(--color-muted)] transition-colors hover:border-[var(--color-primary)]/60 hover:text-[var(--color-primary)]"
								>
									{ty.label}
									<span class="tabular-nums opacity-70">{ty.entity_count}</span>
								</a>
							{/each}
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>
