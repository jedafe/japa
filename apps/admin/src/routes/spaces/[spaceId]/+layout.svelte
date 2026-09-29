<script lang="ts">
	// Space-scoped tab nav — shared across every /spaces/[spaceId] page
	// (moderators overview, rules, join requests, forms incl. nested
	// forms/new and forms/[formId]). Without it the tab routes were
	// only reachable by direct URL.
	import { page } from '$app/state';
	import { t } from '$lib/stores/i18n.svelte';
	import { Shield, ScrollText, UserPlus, ClipboardList } from 'lucide-svelte';

	let { children } = $props();

	const spaceId = $derived(page.params.spaceId ?? '');
	const pathname = $derived(page.url.pathname);

	const tabs = $derived([
		{ label: t('spaces.moderators', 'Moderators'), href: `/spaces/${spaceId}`, icon: Shield, exact: true },
		{ label: t('space_rules.title', 'Space Rules'), href: `/spaces/${spaceId}/rules`, icon: ScrollText },
		{ label: t('joinRequests.title', 'Join Requests'), href: `/spaces/${spaceId}/join-requests`, icon: UserPlus },
		{ label: t('space_forms.title', 'Forms'), href: `/spaces/${spaceId}/forms`, icon: ClipboardList }
	]);

	const isActive = (href: string, exact = false) =>
		exact ? pathname === href : pathname === href || pathname.startsWith(`${href}/`);
</script>

<nav class="mb-4 flex flex-wrap gap-1 border-b" style="border-color: var(--color-border)" aria-label={t('spaces.tabs', 'Space sections')}>
	{#each tabs as tab (tab.href)}
		{@const active = isActive(tab.href, tab.exact)}
		{@const Icon = tab.icon}
		<a
			href={tab.href}
			aria-current={active ? 'page' : undefined}
			class="flex items-center gap-1.5 border-b-2 px-3 py-2 text-sm font-medium transition-colors"
			style={active
				? 'border-color: var(--color-primary); color: var(--color-primary)'
				: 'border-color: transparent; color: var(--color-muted)'}
		>
			<Icon class="h-4 w-4" />
			{tab.label}
		</a>
	{/each}
</nav>

{@render children()}
