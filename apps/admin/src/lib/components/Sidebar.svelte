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
		GraduationCap,
		Bell,
		Lock,
		Mail,
		BarChart2,
		Link,
		UserPlus,
		BellRing,
		Sparkles,
		ListTree,
		Flag,
		Scale,
		Wrench
	} from 'lucide-svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Search } from 'lucide-svelte';

	let { currentPath = '/' }: { currentPath?: string } = $props();

	let open = $state(false);
	let searchQuery = $state('');

	const nav = [
		{ label: 'Dashboard', href: '/', icon: LayoutDashboard, perm: null },
		{ label: 'Users', href: '/users', icon: Users, perm: 'admin.users.view' },
		{ label: 'Moderation', href: '/moderation', icon: ShieldCheck, perm: 'admin.moderation' },
		{ label: 'Moderation Alerts', href: '/moderation/alerts', icon: Bell, perm: 'admin.moderation.view' },
		{ label: 'GDPR', href: '/gdpr', icon: Lock, perm: 'admin.moderation.view' },
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
		{ label: 'Settings', href: '/settings', icon: Settings, perm: 'admin.settings' },
		{ label: 'Custom Fields', href: '/custom-fields', icon: ListTree, perm: 'admin.settings' },
		{ label: 'Newsletter', href: '/newsletter', icon: Mail, perm: 'admin.settings' },
		{ label: 'Analytics', href: '/analytics', icon: BarChart2, perm: 'admin.analytics.view' },
		{ label: 'Affiliate', href: '/affiliate', icon: Link, perm: 'admin.settings' },
		{ label: 'Referral', href: '/referral', icon: UserPlus, perm: 'admin.settings' },
		{ label: 'Notification Types', href: '/notification-types', icon: BellRing, perm: 'admin.settings' },
		{ label: 'AI', href: '/ai', icon: Sparkles, perm: 'admin.ai.view' },
		{ label: 'AI Tools', href: '/ai/tools', icon: Wrench, perm: 'admin.ai.view' },
		{ label: 'AI Generate', href: '/ai/generate', icon: Sparkles, perm: 'admin.ai.view' },
		{ label: 'Learning', href: '/learning', icon: GraduationCap, perm: 'admin.settings' },
		{ label: 'Mentorship', href: '/mentorship', icon: Users, perm: 'admin.settings' },
		{ label: 'Moderation Content', href: '/moderation/content', icon: FileText, perm: 'admin.moderation' },
		{ label: 'Moderation Reports', href: '/moderation/reports', icon: Flag, perm: 'admin.moderation' },
		{ label: 'Moderation Appeals', href: '/moderation/appeals', icon: Scale, perm: 'admin.moderation' },
		{ label: 'Users Roles', href: '/users/roles', icon: Shield, perm: 'admin.users.view' },
		{ label: 'Users Groups', href: '/users/groups', icon: Users, perm: 'admin.users.view' },
		{ label: 'Page Builder Pages', href: '/page-builder/pages', icon: Layout, perm: 'admin.settings' },
		{ label: 'Webhooks', href: '/webhooks', icon: Code, perm: 'admin.settings' },

		{ label: 'Blog Review Queue', href: '/blog/review-queue', icon: FileText, perm: 'blog.review.manage' },
		{ label: 'Feature Flags', href: '/feature-flags', icon: Flag, perm: 'admin.feature_flags.view' }
	];

	function isActive(href: string): boolean {
		if (href === '/') return currentPath === '/';
		return currentPath === href || currentPath.startsWith(href + '/');
	}

	function visibleNav() {
		return nav.filter((item) => {
			// Check permission
			if (item.perm && !hasPermission(item.perm)) return false;

			// Check search query
			if (searchQuery) {
				const query = searchQuery.toLowerCase();
				return item.label.toLowerCase().includes(query) ||
					   item.href.toLowerCase().includes(query);
			}

			return true;
		});
	}
</script>

<button
	class="fixed left-4 top-4 z-50 rounded-md p-2 shadow-md md:hidden"
	style="background-color: var(--color-surface); color: var(--color-foreground)"
	onclick={() => (open = !open)}
>
	{#if open}
		<X class="h-5 w-5" />
	{:else}
		<Menu class="h-5 w-5" />
	{/if}
</button>

<aside
	class="fixed inset-y-0 left-0 z-40 flex h-full w-60 flex-shrink-0 flex-col overflow-hidden transition-transform md:static md:translate-x-0"
	style="border-color: var(--color-border); background-color: var(--color-surface)"
	class:translate-x-0={open}
	class:-translate-x-full={!open}
>
	<div class="flex h-14 items-center border-b px-4" style="border-color: var(--color-border)">
		<span class="text-sm font-bold tracking-wide uppercase" style="color: var(--color-primary)">Admin</span>
	</div>
	
	<!-- Search input -->
	<div class="px-3 pt-3">
		<div class="relative">
			<Search class="absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2" style="color: var(--color-muted)" />
			<input
				type="text"
				placeholder="Search navigation..."
				class="w-full rounded-md border py-1.5 pl-7 pr-3 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				bind:value={searchQuery}
			/>
		</div>
	</div>
	
	<nav class="flex-1 space-y-1 overflow-y-auto p-3">
		{#each visibleNav() as item}
			{@const active = isActive(item.href)}
			<a
				href={item.href}
				class="flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors {active ? 'text-primary-700 dark:text-primary-300' : 'hover:bg-black/5 dark:hover:bg-white/5'}"
				style={active ? 'background-color: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-primary)' : `color: var(--color-foreground)`}
				onclick={() => (open = false)}
			>
				<item.icon class="h-4 w-4" />
				{item.label}
			</a>
		{/each}
	</nav>
	<div class="border-t p-3" style="border-color: var(--color-border)">
		<p class="text-xs" style="color: var(--color-muted)">JapaConnect Admin</p>
	</div>
</aside>

{#if open}
	<button
		class="fixed inset-0 z-30 bg-black/50 md:hidden"
		aria-label="Close sidebar"
		onclick={() => (open = false)}
	></button>
{/if}
