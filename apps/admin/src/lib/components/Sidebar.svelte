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
		Layout,
		Building2,
		Heart,
		GraduationCap
	} from 'lucide-svelte';
	import { hasPermission } from '$lib/stores/permissions';

	let { currentPath = '/' }: { currentPath?: string } = $props();

	let open = $state(false);

	const nav = [
		{ label: 'Dashboard', href: '/', icon: LayoutDashboard, perm: null },
		{ label: 'Users', href: '/users', icon: Users, perm: 'admin.users.view' },
		{ label: 'Moderation', href: '/moderation', icon: ShieldCheck, perm: 'admin.moderation' },
		{ label: 'Spam', href: '/spam', icon: Shield, perm: 'spam.ip_block.manage' },
		{ label: 'Spaces', href: '/spaces', icon: Layers, perm: 'admin.settings' },
		{ label: 'Tags', href: '/tags', icon: Tag, perm: 'admin.settings' },
		{ label: 'Organisations', href: '/organisations', icon: Building2, perm: 'admin.settings' },
		{ label: 'Reactions', href: '/reactions', icon: Heart, perm: 'admin.settings' },
		{ label: 'Universities', href: '/universities', icon: GraduationCap, perm: 'admin.settings' },
		{ label: 'Ticket Types', href: '/ticket-types', icon: Tag, perm: 'admin.tickets.manage' },
		{ label: 'Wallet', href: '/wallet', icon: Wallet, perm: 'wallet.admin' },
		{ label: 'Space Types', href: '/space-types', icon: Layers, perm: 'admin.settings' },
		{ label: 'Comparison Types', href: '/comparison-types', icon: GitCompare, perm: 'admin.settings' },
		{ label: 'Professional Types', href: '/professional-types', icon: Briefcase, perm: 'admin.settings' },
		{ label: 'Docs', href: '/docs', icon: FileText, perm: 'admin.settings' },
		{ label: 'JSON-LD Templates', href: '/jsonld-templates', icon: Code, perm: 'admin.settings' },
		{ label: 'Page Builder', href: '/page-builder', icon: Layout, perm: 'admin.settings' },
		{ label: 'Bots', href: '/bots', icon: Bot, perm: 'bots.admin' },
		{ label: 'Advertising', href: '/ads', icon: Megaphone, perm: 'admin.settings' },
		{ label: 'Ad Slot Types', href: '/ads/slot-types', icon: Grid3x3, perm: 'admin.settings' },
		{ label: 'Ad Boosts', href: '/ads/boosts', icon: Zap, perm: 'admin.settings' },
		{ label: 'Tickets', href: '/tickets', icon: Ticket, perm: 'admin.tickets.manage' },
		{ label: 'Broadcast', href: '/broadcast', icon: Radio, perm: 'admin.broadcast' },
		{ label: 'Translations', href: '/translations', icon: Languages, perm: 'admin.settings' },
		{ label: 'Settings', href: '/settings', icon: Settings, perm: 'admin.settings' }
	];

	function visibleNav() {
		return nav.filter((item) => !item.perm || hasPermission(item.perm));
	}
</script>

<button
	class="fixed left-4 top-4 z-50 rounded-md bg-white p-2 shadow-md md:hidden dark:bg-gray-800"
	onclick={() => (open = !open)}
>
	{#if open}
		<X class="h-5 w-5" />
	{:else}
		<Menu class="h-5 w-5" />
	{/if}
</button>

<aside
	class="fixed inset-y-0 left-0 z-40 w-60 flex-shrink-0 flex-col border-r border-gray-200 bg-white transition-transform md:static md:translate-x-0 dark:border-gray-700 dark:bg-gray-800"
	class:translate-x-0={open}
	class:-translate-x-full={!open}
>
	<div class="flex h-14 items-center border-b border-gray-200 px-4 dark:border-gray-700">
		<span class="text-sm font-bold tracking-wide uppercase text-blue-600">Admin</span>
	</div>
	<nav class="flex-1 space-y-1 p-3">
		{#each visibleNav() as item}
			<a
				href={item.href}
				class="flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
				onclick={() => (open = false)}
			>
				<item.icon class="h-4 w-4" />
				{item.label}
			</a>
		{/each}
	</nav>
	<div class="border-t border-gray-200 p-3 dark:border-gray-700">
		<p class="text-xs text-gray-500 dark:text-gray-400">JapaConnect Admin</p>
	</div>
</aside>

{#if open}
	<button
		class="fixed inset-0 z-30 bg-black/50 md:hidden"
		aria-label="Close sidebar"
		onclick={() => (open = false)}
	></button>
{/if}
