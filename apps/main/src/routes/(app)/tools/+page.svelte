<script lang="ts">
	import { t } from '$lib/i18n';
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
		MapPin
	} from 'lucide-svelte';
	import type { IconComponent, UrlTab } from '$lib/components/ui';
	import { ListPageBar, UrlTabs } from '$lib/components/ui';
	import { getEnabledToolsBySection, TOOL_SECTIONS, type MainToolMeta, type ToolSection } from '$lib/tools/registry';
	import { FileCheck2, Wallet2, PenLine } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('tools.title', 'Tools — JapaConnect'));
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
	<div class="mx-3 border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Compass size={20} /> {t('tools.heading', 'Tools')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)]">{description}</p>
	</div>

	{#if totalEnabled === 0}
		<p class="px-4 py-8 text-center text-sm text-[var(--color-muted)]">
			{t('tools.disabled_body', 'Check back soon or contact support.')}
		</p>
	{:else}
		<!-- ListPageBar pattern — same as /guides -->
		<ListPageBar>
			<UrlTabs {tabs} bind:value={activeTab} class="!border-b-0 !bg-transparent" />
		</ListPageBar>

		<div class="p-3">
			<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
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
			class="grid h-11 w-11 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-primary)]"
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
