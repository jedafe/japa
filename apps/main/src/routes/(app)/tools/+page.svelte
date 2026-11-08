<script lang="ts">
	import SeoHead from '#lib/seo/SeoHead.svelte';
	import { t } from '#lib/i18n/index.js';
	import { page } from '$app/state';
	import { untrack } from 'svelte';
	import {
		Plane,
		ClipboardCheck,
		Gauge,
		Calculator,
		GraduationCap,
		Banknote,
		Compass,
		Bot,
		Route,
		BellRing,
		PlaneLanding,
		Timer,
		BadgeCheck,
		FileText,
		Home,
		Stamp,
		Languages,
		Receipt,
		MapPin,
		Search,
		ChevronRight,
		Layers
	} from 'lucide-svelte';
	import type { IconComponent, UrlTab } from '#lib/components/ui/index.js';
	import { GuideSlideOver, type GuideContent } from '#lib/components/ui/index.js';
	import { Filter } from 'lucide-svelte';
	import { api } from '#lib/api/client.js';
	import { EntityHero, ListPageBar, UrlTabs } from '#lib/components/ui/index.js';
	import HeroArtTools from '#lib/components/heroes/HeroArtTools.svelte';
	import { formatMetric } from '#lib/components/hub/charts/format.js';
	import { getEnabledToolsBySection, TOOL_SECTIONS, type MainToolMeta, type ToolSection } from '#lib/tools/registry.js';
	import { FileCheck2, Wallet2, PenLine } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('tools.title', 'Tools — {site}'));
	const description = $derived(
		t('tools.description', 'Plan your move with calculators, checklists, and trackers built for the diaspora.')
	);

	// Lucide icon component per tool href. Kept here (not in the registry) because
	// the registry stores plain serialisable metadata; icon components are runtime
	// imports. Ref: §78 (Tools Web Surface).
	const ICONS: Record<string, IconComponent> = {
		'/tools/visa-checklist': ClipboardCheck,
		'/tools/readiness': Gauge,
		'/tools/cost-of-living': Calculator,
		'/scholarships': GraduationCap,
		'/tools/remittance': Banknote,
		'/tools/travel': Plane,
		'/ai': Bot,
		'/tools/visa-routes': Route,
		'/tools/embassy-alerter': BellRing,
		'/tools/arrival-checklist': PlaneLanding,
		'/tools/processing-tracker': Timer,
		'/tools/credential-checker': BadgeCheck,
		'/tools/cv-formatter': FileText,
		'/tools/housing-guide': Home,
		'/tools/attestation-guide': Stamp,
		'/tools/language-test': Languages,
		'/tools/tax-calculator': Receipt,
		'/tools/community-locator': MapPin,
		'/tools/crs-calculator': Calculator,
		'/tools/pof-calculator': Wallet2,
		'/tools/eca-guide': FileCheck2,
		'/tools/sop-generator': PenLine
	};

	// Data-driven: the tool list is filtered by each tool's feature flag
	// (read from the root layout's /feature-flags fetch). Disabled tools
	// are hidden from the hub. Ref: sections/42-feature-flags.md.
	const features = $derived<Record<string, boolean>>(data.features ?? {});
	const sections = $derived(
		TOOL_SECTIONS.map((section) => ({
			section,
			tools: getEnabledToolsBySection(features, section)
		})).filter((s) => s.tools.length > 0)
	);
	const totalEnabled = $derived(sections.reduce((sum, s) => sum + s.tools.length, 0));

	// URL-based tab value, synced via UrlTabs (instant client-side switching).
	let activeTab = $state<ToolSection>(
		untrack(() => {
			const urlTab = page.url.searchParams.get('tab');
			if (urlTab && TOOL_SECTIONS.includes(urlTab as ToolSection)) {
				return urlTab as ToolSection;
			}
			return sections[0]?.section ?? 'planning';
		})
	);
	$effect(() => {
		if (sections.length > 0 && !sections.some((s) => s.section === activeTab)) {
			activeTab = sections[0].section;
		}
	});

	// Tabs + content for the active section.
	const tabs = $derived<UrlTab[]>(
		sections.map((s) => ({ value: s.section, label: t(`tools.section_${s.section}`, s.section) }))
	);
	const activeTools = $derived(sections.find((s) => s.section === activeTab)?.tools ?? []);

	// "How it works" slide-over — markdown panel + CTA repeater persisted to
	// public system settings (tools.guide) via PUT /admin/settings/general.
	let guideOpen = $state(false);
	let guideOverride = $state<GuideContent | null>(null);
	const siteSettings = $derived((page.data.siteSettings ?? {}) as Record<string, unknown>);
	const storedGuide = $derived.by((): Partial<GuideContent> => {
		const raw = siteSettings['tools.guide'];
		if (raw && typeof raw === 'object' && !Array.isArray(raw)) return raw as Partial<GuideContent>;
		return {};
	});
	const effectiveGuide = $derived<GuideContent>({
		title: guideOverride?.title ?? storedGuide.title ?? t('common.how_it_works', 'How it works'),
		body: guideOverride?.body ?? storedGuide.body ?? t('tools.guide_body', ''),
		ctas: guideOverride?.ctas ?? storedGuide.ctas ?? [{ title: t('guide_panel.read_full_guide', 'Read the full guide'), url: '/guides' }]
	});

	async function saveGuide(v: GuideContent): Promise<string | null> {
		const res = await api.put('/admin/settings/general', { tools_guide: v });
		if (res.error) return res.error.message || t('common.error', 'Something went wrong');
		guideOverride = v;
		return null;
	}
</script>

<SeoHead title={title} description={description} ogTitle={title} />

<div class="mx-auto w-full max-w-full" style="--page-gutter: max(1rem, calc((100% - 48rem)/2))">
	<!-- Header — EntityHero directory panel -->
	<EntityHero
		class="mt-4"
		icon={Compass}
		accent="var(--color-primary)"
		heading={t('tools.heading', 'Tools')}
		description={t('tools.hero_desc', 'Plan your move with calculators, checklists, and trackers built for the diaspora.')}
		crumbs={[
			{ label: t('common.home', 'Home'), href: '/' },
			{ label: t('tools.heading', 'Tools') }
		]}
		stats={[
			{ icon: Compass, value: formatMetric(totalEnabled), label: t('tools.stats_tools', 'Tools') },
			{ icon: Layers, value: formatMetric(sections.length), label: t('tools.stats_categories', 'Categories') }
		]}
	>
		{#snippet art()}
			<HeroArtTools accent="var(--color-primary)" />
		{/snippet}
		{#snippet actions(c)}
			<div class="grid w-full grid-cols-2 gap-3 sm:flex sm:items-center">
			<a href="#tools-grid" class="{c.primary} flex-1 justify-center whitespace-nowrap">
				<Search size={15} /> {t('tools.browse', 'Browse tools')}
			</a>
			<button type="button" onclick={() => (guideOpen = true)} class="{c.ghost} flex-1 justify-center whitespace-nowrap">
				{t('common.how_it_works', 'How it works')}
				<ChevronRight size={14} class="rtl:-scale-x-100" />
				</button>
			</div>
		{/snippet}
	</EntityHero>

	{#if totalEnabled === 0}
		<p class="px-4 py-8 text-center text-sm text-[var(--color-muted)]">
			{t('tools.disabled_body', 'Check back soon or contact support.')}
		</p>
	{:else}
		<!-- ListPageBar pattern — same as /guides -->
		<ListPageBar>
			<UrlTabs {tabs} bind:value={activeTab} class="!border-b-0 !bg-transparent" />
		</ListPageBar>

		<div id="tools-grid" class="mx-auto w-full max-w-3xl p-4">
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
				{#each activeTools as tool (tool.href)}
					{@render toolCard(tool)}
				{/each}
			</div>
		</div>
	{/if}
</div>

{#snippet toolCard(tool: MainToolMeta)}
	{@const Icon = ICONS[tool.href] ?? Compass}
	<a
		href={tool.href}
		class="flex items-start gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:bg-[var(--color-surface-subtle)]"
	>
		<span
			class="grid h-11 w-11 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-primary-text)]"
		>
			<Icon size={22} />
		</span>
		<span class="min-w-0 flex-1">
			<span class="block text-sm font-medium text-[var(--color-text)]">
				{t(tool.labelKey, tool.labelKey)}
			</span>
			<span class="mt-1 block text-xs text-[var(--color-muted)]">
				{t(tool.descKey, tool.descKey)}
			</span>
		</span>
	</a>
{/snippet}

<!-- "How it works" slide-over — GuideSlideOver; admins edit inline (tools.guide). -->
<GuideSlideOver
	bind:open={guideOpen}
	title={effectiveGuide.title}
	body={effectiveGuide.body}
	ctas={effectiveGuide.ctas}
	canManage={data.canManage}
	onSave={saveGuide}
/>
