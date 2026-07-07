<script lang="ts">
	import {
		LayoutDashboard,
		Users,
		ShieldCheck,
		Settings,
		Ticket,
		Radio,
		Bot,
		Megaphone,
		Menu,
		X,
		Languages,
		Shield,
		Tag,
		Wallet,
		Layers,
		FileText,
		Grid3x3,
		Zap,
		GitCompare,
		Briefcase,
		Code,
		Layout
	} from 'lucide-svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions';

	let {
		currentPath = '/'
	}: {
		currentPath?: string;
	} = $props();

	let open = $state(false);

	/** Permission key required for each nav section. Null = visible to all admins. */
	const nav = [
		{ labelKey: 'nav.dashboard', href: '/', icon: LayoutDashboard, perm: null },
		{ labelKey: 'nav.users', href: '/users', icon: Users, perm: 'admin.users.view' },
		{ labelKey: 'nav.moderation', href: '/moderation', icon: ShieldCheck, perm: 'admin.moderation' },
		{ labelKey: 'nav.spam', href: '/spam', icon: Shield, perm: 'spam.ip_block.manage' },
		{ labelKey: 'nav.ticket_types', href: '/ticket-types', icon: Tag, perm: 'admin.tickets.manage' },
		{ labelKey: 'nav.wallet', href: '/wallet', icon: Wallet, perm: 'wallet.admin' },
		{ labelKey: 'nav.space_types', href: '/space-types', icon: Layers, perm: 'admin.settings' },
		{ labelKey: 'nav.comparison_types', href: '/comparison-types', icon: GitCompare, perm: 'admin.settings' },
		{ labelKey: 'nav.professional_types', href: '/professional-types', icon: Briefcase, perm: 'admin.settings' },
		{ labelKey: 'nav.docs', href: '/docs', icon: FileText, perm: 'admin.settings' },
		{ labelKey: 'nav.jsonld_templates', href: '/jsonld-templates', icon: Code, perm: 'admin.settings' },
		{ labelKey: 'nav.page_builder', href: '/page-builder', icon: Layout, perm: 'admin.settings' },
		{ labelKey: 'nav.bots', href: '/bots', icon: Bot, perm: 'bots.admin' },
		{ labelKey: 'nav.advertising', href: '/ads', icon: Megaphone, perm: 'admin.settings' },
		{ labelKey: 'nav.ad_slot_types', href: '/ads/slot-types', icon: Grid3x3, perm: 'admin.settings' },
		{ labelKey: 'nav.ad_boosts', href: '/ads/boosts', icon: Zap, perm: 'admin.settings' },
		{ labelKey: 'nav.tickets', href: '/tickets', icon: Ticket, perm: 'admin.tickets.manage' },
		{ labelKey: 'nav.broadcast', href: '/broadcast', icon: Radio, perm: 'admin.broadcast' },
		{ labelKey: 'nav.translations', href: '/translations', icon: Languages, perm: 'admin.settings' },
		{ labelKey: 'nav.settings', href: '/settings', icon: Settings, perm: 'admin.settings' }
	];

	/** Filter nav items by the user's resolved permissions. */
	function visibleNav() {
		return nav.filter((item) => !item.perm || hasPermission(item.perm));
	}
</script>

<!-- Mobile toggle -->
<button
	class="fixed left-4 top-4 z-50 rounded-md p-2 md:hidden"
	style="background-color: var(--color-surface); border: 1px solid var(--color-border)"
	onclick={() => (open = !open)}
	aria-label={open ? t('common.close_menu') : t('common.open_menu')}
>
	{#if open}
		<X class="h-5 w-5" />
	{:else}
		<Menu class="h-5 w-5" />
	{/if}
</button>

<!-- Sidebar -->
<aside
	class="fixed inset-y-0 left-0 z-40 w-60 flex-shrink-0 flex-col border-r transition-transform md:static md:translate-x-0"
	style="background-color: var(--color-surface); border-color: var(--color-border)"
	class:translate-x-0={open}
	class:-translate-x-full={!open}
>
	<div class="flex h-14 items-center border-b px-4" style="border-color: var(--color-border)">
		<span class="text-sm font-bold tracking-wide uppercase" style="color: var(--color-primary)">{t('admin.brand')}</span>
	</div>
	<nav class="flex-1 space-y-1 p-3">
		{#each visibleNav() as item}
			<a
				href={item.href}
				class="flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
				onclick={() => (open = false)}
			>
				<item.icon class="h-4 w-4" style="color: var(--color-muted)" />
				{t(item.labelKey)}
			</a>
		{/each}
	</nav>
	<div class="border-t p-3" style="border-color: var(--color-border)">
		<p class="text-xs" style="color: var(--color-muted)">{t('admin.footer')}</p>
	</div>
</aside>

{#if open}
	<button
		class="fixed inset-0 z-30 bg-black/50 md:hidden"
		aria-label={t('common.close_menu')}
		onclick={() => (open = false)}
	></button>
{/if}
