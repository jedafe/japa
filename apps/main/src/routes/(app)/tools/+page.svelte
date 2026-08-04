<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import {
		Plane,
		ClipboardCheck,
		Gauge,
		Calculator,
		GraduationCap,
		Banknote,
		Compass,
		Bot
	} from 'lucide-svelte';
	import type { IconComponent } from '$lib/components/ui';
	import type { PageData } from './$types';

	let { data: _data }: { data: PageData } = $props();

	const title = $derived(t('tools.title', 'Tools — JapaConnect'));
	const description = $derived(
		t('tools.description', 'Plan your move with calculators, checklists, and trackers built for the diaspora.')
	);

	interface ToolLink {
		href: string;
		icon: IconComponent;
		labelKey: string;
		labelFallback: string;
		descKey: string;
		descFallback: string;
	}

	// Spec-defined tool set (§67, §69–74). Each tool's availability is gated
	// by its own backend handler; the hub lists all spec tools and each
	// destination surfaces a disabled state when its setting is off.
	const tools: ToolLink[] = [
		{
			href: '/tools/visa-checklist',
			icon: ClipboardCheck,
			labelKey: 'tools.visa_checklist',
			labelFallback: 'Visa Checklist',
			descKey: 'tools.visa_checklist_desc',
			descFallback: 'Build a document checklist for your visa route.'
		},
		{
			href: '/tools/readiness',
			icon: Gauge,
			labelKey: 'tools.readiness',
			labelFallback: 'Readiness Assessment',
			descKey: 'tools.readiness_desc',
			descFallback: 'Score your japa readiness across key dimensions.'
		},
		{
			href: '/tools/cost-of-living',
			icon: Calculator,
			labelKey: 'tools.cost_of_living',
			labelFallback: 'Cost of Living Calculator',
			descKey: 'tools.cost_of_living_desc',
			descFallback: 'Compare living costs between cities.'
		},
		{
			href: '/scholarships',
			icon: GraduationCap,
			labelKey: 'tools.scholarships',
			labelFallback: 'Scholarship Finder',
			descKey: 'tools.scholarships_desc',
			descFallback: 'Find and track scholarships by country and level.'
		},
		{
			href: '/tools/remittance',
			icon: Banknote,
			labelKey: 'tools.remittance',
			labelFallback: 'Remittance Rate Tracker',
			descKey: 'tools.remittance_desc',
			descFallback: 'Compare provider rates and set alerts.'
		},
		{
			href: '/tools/travel',
			icon: Plane,
			labelKey: 'tools.travel',
			labelFallback: 'Travel Search',
			descKey: 'tools.travel_desc',
			descFallback: 'Search flights and hotels for your move.'
		},
		{
			href: '/ai/chat',
			icon: Bot,
			labelKey: 'tools.japabot',
			labelFallback: 'JapaBot',
			descKey: 'tools.japabot_desc',
			descFallback: 'Ask JapaBot anything about relocating.'
		}
	];
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
			<Compass size={20} /> {t('tools.heading', 'Tools')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)]">{description}</p>
	</div>

	<div class="p-3">
		<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
			{#each tools as tool (tool.href)}
				<a
					href={tool.href}
					class="flex items-start gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:bg-[var(--color-surface-subtle)]"
				>
					<span
						class="grid h-11 w-11 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-primary)]"
					>
						<tool.icon size={22} />
					</span>
					<span class="min-w-0 flex-1">
						<span class="block text-sm font-medium text-[var(--color-text)]">
							{t(tool.labelKey, tool.labelFallback)}
						</span>
						<span class="mt-1 block text-xs text-[var(--color-muted)]">
							{t(tool.descKey, tool.descFallback)}
						</span>
					</span>
				</a>
			{/each}
		</div>
	</div>
</div>
