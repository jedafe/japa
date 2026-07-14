<script lang="ts">
	import { page } from "$app/state";
	import Tooltip from "$lib/components/ui/tooltip/tooltip.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import {
		LayoutDashboard, Users, Shield, BarChart3, Settings,
		FileText, MessageSquare, Ticket, Wallet, Building,
		Tag, Globe, BookOpen, GraduationCap, Users2,
		Bell, Search, Database, Zap, Bot,
		Mail, FileCode, Webhook, Share2, DollarSign,
		Link, Languages, Accessibility,
		Download, Trash2, FileInput, AlertTriangle,
		Star, MessageCircle, Layout, Image,
		Type, ListOrdered, GitCompare,
		Clock, UserPlus, ChevronDown, ChevronRight,
		Briefcase, Eye, X, Menu
	} from "lucide-svelte";
	import { clsx } from "clsx";

	let {
		collapsed = false,
		mobile = false,
		onclose
	}: {
		collapsed?: boolean;
		mobile?: boolean;
		onclose?: () => void;
	} = $props();

	let expandedSections = $state(new Set(["dashboard", "users", "content", "moderation"]));
	let open = $state(false);
	let searchQuery = $state("");

	interface NavItem {
		path: string;
		label: string;
		icon: any;
		permission?: string;
	}

	interface NavSection {
		id: string;
		label: string;
		items: NavItem[];
	}

	const sections: NavSection[] = [
		{
			id: "dashboard",
			label: "Dashboard",
			items: [
				{ path: "/", label: "Overview", icon: LayoutDashboard }
			]
		},
		{
			id: "users",
			label: "User Management",
			items: [
				{ path: "/users", label: "Users", icon: Users, permission: "admin.users.view" },
				{ path: "/organisations", label: "Organisations", icon: Building, permission: "admin.settings" },
				{ path: "/universities", label: "Universities", icon: GraduationCap, permission: "admin.settings" },
				{ path: "/professional-types", label: "Professional Types", icon: Briefcase, permission: "admin.settings" },
				{ path: "/users/groups", label: "Groups", icon: Users2, permission: "admin.users.view" }
			]
		},
		{
			id: "content",
			label: "Content",
			items: [
				{ path: "/content", label: "All Content", icon: FileText, permission: "admin.moderation" },
				{ path: "/blog/review-queue", label: "Blog Posts", icon: BookOpen, permission: "blog.review.manage" },
				{ path: "/docs", label: "Documentation", icon: FileCode, permission: "admin.settings" },
				{ path: "/media", label: "Media", icon: Image, permission: "admin.settings" },
				{ path: "/spaces", label: "Spaces", icon: Globe, permission: "admin.settings" },
				{ path: "/space-types", label: "Space Types", icon: Layout, permission: "admin.settings" },
				{ path: "/tags", label: "Tags", icon: Tag, permission: "admin.settings" },
				{ path: "/reactions", label: "Reactions", icon: Star, permission: "admin.settings" }
			]
		},
		{
			id: "moderation",
			label: "Moderation",
			items: [
				{ path: "/moderation", label: "Queue", icon: Shield, permission: "admin.moderation" },
				{ path: "/moderation-queue", label: "Review Queue", icon: AlertTriangle, permission: "admin.moderation" },
				{ path: "/spam", label: "Spam Management", icon: AlertTriangle, permission: "spam.ip_block.manage" },
				{ path: "/tickets", label: "Support Tickets", icon: Ticket, permission: "admin.tickets.manage" },
				{ path: "/reviews", label: "Reviews", icon: MessageCircle, permission: "admin.settings" }
			]
		},
		{
			id: "analytics",
			label: "Analytics & Reports",
			items: [
				{ path: "/analytics", label: "Analytics", icon: BarChart3, permission: "admin.analytics.view" },
				{ path: "/reports", label: "Reports", icon: FileText, permission: "admin.settings" },
				{ path: "/search", label: "Search Analytics", icon: Search, permission: "admin.settings" }
			]
		},
		{
			id: "engagement",
			label: "Engagement",
			items: [
				{ path: "/newsletter", label: "Newsletter", icon: Mail, permission: "admin.settings" },
				{ path: "/broadcast", label: "Broadcasts", icon: Bell, permission: "admin.broadcast" },
				{ path: "/forms", label: "Forms", icon: FileInput, permission: "admin.settings" },
				{ path: "/learning", label: "Learning", icon: GraduationCap, permission: "admin.settings" },
				{ path: "/mentorship", label: "Mentorship", icon: Users2, permission: "admin.settings" }
			]
		},
		{
			id: "monetization",
			label: "Monetization",
			items: [
				{ path: "/wallet", label: "Wallet", icon: Wallet, permission: "wallet.admin" },
				{ path: "/coupons", label: "Coupons", icon: Tag, permission: "admin.settings" },
				{ path: "/discount-codes", label: "Discount Codes", icon: DollarSign, permission: "admin.settings" },
				{ path: "/affiliate", label: "Affiliate Links", icon: Link, permission: "admin.settings" },
				{ path: "/referral", label: "Referrals", icon: Share2, permission: "admin.settings" },
				{ path: "/ads", label: "Advertisements", icon: Zap, permission: "admin.settings" }
			]
		},
		{
			id: "automation",
			label: "Automation",
			items: [
				{ path: "/ai", label: "AI Tools", icon: Bot, permission: "admin.ai.view" },
				{ path: "/bots", label: "Bots", icon: Bot, permission: "bots.admin" },
				{ path: "/webhooks", label: "Webhooks", icon: Webhook, permission: "admin.settings" },
				{ path: "/feature-flags", label: "Feature Flags", icon: Zap, permission: "admin.feature_flags.view" }
			]
		},
		{
			id: "customization",
			label: "Customization",
			items: [
				{ path: "/page-builder", label: "Page Builder", icon: Layout, permission: "admin.settings" },
				{ path: "/page-templates", label: "Page Templates", icon: FileText, permission: "admin.settings" },
				{ path: "/custom-fields", label: "Custom Fields", icon: Type, permission: "admin.settings" },
				{ path: "/comparisons", label: "Comparisons", icon: GitCompare, permission: "admin.settings" },
				{ path: "/comparison-types", label: "Comparison Types", icon: ListOrdered, permission: "admin.settings" }
			]
		},
		{
			id: "system",
			label: "System",
			items: [
				{ path: "/settings", label: "Settings", icon: Settings, permission: "admin.settings" },
				{ path: "/i18n", label: "Translations", icon: Languages, permission: "admin.settings" },
				{ path: "/notification-types", label: "Notifications", icon: Bell, permission: "admin.settings" },
				{ path: "/jsonld-templates", label: "JSON-LD Templates", icon: FileCode, permission: "admin.settings" },
				{ path: "/data-exports", label: "Data Exports", icon: Download, permission: "admin.settings" },
				{ path: "/deletions", label: "Account Deletions", icon: Trash2, permission: "admin.moderation.view" },
				{ path: "/gdpr", label: "GDPR Compliance", icon: Accessibility, permission: "admin.moderation.view" }
			]
		}
	];

	function toggleSection(id: string) {
		if (expandedSections.has(id)) {
			expandedSections.delete(id);
		} else {
			expandedSections.add(id);
		}
		expandedSections = expandedSections;
	}

	function isActive(path: string): boolean {
		if (path === "/") {
			return page.url.pathname === "/";
		}
		return page.url.pathname === path || page.url.pathname.startsWith(path + "/");
	}

	function filterSections(query: string): NavSection[] {
		if (!query) return sections;
		const lower = query.toLowerCase();
		return sections
			.map((section) => ({
				...section,
				items: section.items.filter(
					(item) =>
						item.label.toLowerCase().includes(lower) ||
						item.path.toLowerCase().includes(lower)
				)
			}))
			.filter((section) => section.items.length > 0);
	}
</script>

{#if mobile}
	<div class="fixed inset-0 z-50 bg-black/80 dark:bg-black/60" role="presentation" onclick={onclose}></div>
{/if}

<button
	type="button"
	class="fixed left-4 top-4 z-50 rounded-md bg-white p-2 shadow-md md:hidden dark:bg-gray-800"
	onclick={() => (open = !open)}
>
	{#if open}
		<X class="h-5 w-5 text-gray-700 dark:text-gray-300" />
	{:else}
		<Menu class="h-5 w-5 text-gray-700 dark:text-gray-300" />
	{/if}
</button>

<aside
	class={clsx(
		"flex h-full flex-col border-r border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-900",
		mobile
			? "fixed inset-y-0 left-0 z-50 w-64 transition-transform duration-200"
			: "hidden w-64 md:flex",
		mobile && !open && "-translate-x-full",
		collapsed && !mobile && "w-16"
	)}
>
	<!-- Logo -->
	<div class="flex h-14 items-center border-b border-gray-200 px-4 dark:border-gray-700">
		<a href="/" class="flex items-center gap-2">
			<div class="flex h-8 w-8 items-center justify-center rounded-lg bg-primary-600">
				<span class="text-sm font-bold text-white">JC</span>
			</div>
			{#if !collapsed}
				<span class="text-lg font-semibold text-gray-900 dark:text-gray-100">Admin</span>
			{/if}
		</a>
	</div>

	<!-- Search input (expanded mode only) -->
	{#if !collapsed}
		<div class="px-3 pt-3">
			<div class="relative">
				<Search class="absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-gray-400" />
				<input
					type="text"
					placeholder="Search navigation..."
					class="w-full rounded-md border border-gray-200 bg-white py-1.5 pl-7 pr-3 text-sm text-gray-900 placeholder:text-gray-400 focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-100 dark:placeholder:text-gray-500"
					bind:value={searchQuery}
				/>
			</div>
		</div>
	{/if}

	<!-- Navigation -->
	<nav class="flex-1 overflow-y-auto p-2 scrollbar-thin">
		{#each filterSections(searchQuery) as section}
			{#if !collapsed}
				<div class="mb-1">
					<button
						type="button"
						class="flex w-full items-center justify-between rounded-lg px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-800"
						onclick={() => toggleSection(section.id)}
					>
						<span>{section.label}</span>
						{#if expandedSections.has(section.id)}
							<ChevronDown class="h-4 w-4 text-gray-400" />
						{:else}
							<ChevronRight class="h-4 w-4 text-gray-400" />
						{/if}
					</button>

					{#if expandedSections.has(section.id)}
						<div class="mt-1 space-y-1">
							{#each section.items as item}
								{#if !item.permission || hasPermission(item.permission)}
									<a
										href={item.path}
										class={clsx(
											"flex items-center gap-3 rounded-lg px-3 py-2 text-sm transition-colors",
											isActive(item.path)
												? "bg-primary-50 font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-400"
												: "text-gray-600 hover:bg-gray-100 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-gray-800 dark:hover:text-gray-100"
										)}
									>
										<svelte:component
											this={item.icon}
											class={clsx(
												"h-5 w-5 flex-shrink-0",
												isActive(item.path) ? "text-primary-600 dark:text-primary-400" : "text-gray-400 dark:text-gray-500"
											)}
										/>
										<span>{item.label}</span>
									</a>
								{/if}
							{/each}
						</div>
					{/if}
				</div>
			{:else}
				<!-- Collapsed view - just icons -->
				{#each section.items as item}
					{#if !item.permission || hasPermission(item.permission)}
						<Tooltip content={item.label} side="right">
							<a
								href={item.path}
								class={clsx(
									"mb-1 flex items-center justify-center rounded-lg p-2 transition-colors",
									isActive(item.path)
										? "bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400"
										: "text-gray-600 hover:bg-gray-100 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-gray-800 dark:hover:text-gray-100"
								)}
							>
								<svelte:component
									this={item.icon}
									class={clsx(
										"h-5 w-5",
										isActive(item.path) ? "text-primary-600 dark:text-primary-400" : "text-gray-400 dark:text-gray-500"
									)}
								/>
							</a>
						</Tooltip>
					{/if}
				{/each}
			{/if}
		{/each}
	</nav>

	<!-- User Menu -->
	{#if !collapsed}
		<div class="border-t border-gray-200 p-4 dark:border-gray-700">
			<div class="flex items-center gap-3">
				<div class="flex h-8 w-8 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/40">
					<span class="text-sm font-medium text-primary-700 dark:text-primary-300">A</span>
				</div>
				<div class="min-w-0 flex-1">
					<p class="truncate text-sm font-medium text-gray-900 dark:text-gray-100">Admin User</p>
					<p class="truncate text-xs text-gray-500 dark:text-gray-400">admin@japa.com</p>
				</div>
			</div>
		</div>
	{/if}
</aside>

{#if mobile && open}
	<button
		type="button"
		class="fixed inset-0 z-30 bg-black/50 md:hidden"
		aria-label="Close sidebar"
		onclick={() => (open = false)}
	></button>
{/if}
