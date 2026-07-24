<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { page } from "$app/state";
	import Tooltip from "$lib/components/ui/tooltip/tooltip.svelte";
	import { hasPermission, getPermissionState, getPermissions } from "$lib/stores/permissions.svelte";
	import { user } from "$lib/stores/auth.svelte";
	import {
		LayoutDashboard, Users, Shield, BarChart3, Settings,
		FileText, MessageSquare, Ticket, Wallet, Building,
		Tag, Globe, BookOpen, GraduationCap, Users2,
		Bell, Search, Database, Zap, Bot,
		Mail, FileCode, Webhook, Share2, DollarSign,
		Link, Languages, Accessibility,
		Download, Trash2, FileInput, AlertTriangle,
		Star, MessageCircle, Layout, Image,
		Type, ListOrdered, GitCompare, Palette,
		Clock, UserPlus, ChevronDown, ChevronRight,
		Briefcase, Eye, X, Menu, ArrowRightLeft
	} from "lucide-svelte";
	import { clsx } from "clsx";

	let {
		collapsed = $bindable(false),
		mobileOpen = $bindable(false),
		onclose,
		permissions = []
	}: {
		collapsed?: boolean;
		mobileOpen?: boolean;
		onclose?: () => void;
		permissions?: string[];
	} = $props();

	let expandedSections = $state(new Set(["dashboard", "users", "content", "moderation"]));
	let searchQuery = $state("");
	
	// Use permissions prop for reactivity
	function canSeeItem(permission?: string): boolean {
		if (!permission) return true;
		return permissions.includes(permission) || permissions.includes('*');
	}

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
				{ path: "/organisations", label: "Organisations", icon: Building, permission: "organizations.manage" },
				{ path: "/universities", label: "Universities", icon: GraduationCap, permission: "admin.settings" },
				{ path: "/professional-types", label: "Professional Types", icon: Briefcase, permission: "professional.manage" },
				{ path: "/users/groups", label: "Groups", icon: Users2, permission: "admin.users.view" }
			]
		},
		{
			id: "content",
			label: "Content",
			items: [
				{ path: "/content", label: "All Content", icon: FileText, permission: "admin.content.view" },
				{ path: "/blog/review-queue", label: "Blog Posts", icon: BookOpen, permission: "blog.review.manage" },
			{ path: "/docs", label: "Documentation", icon: FileCode, permission: "docs.manage" },
			{ path: "/media", label: "Media", icon: Image, permission: "media.view_all" },
			{ path: "/spaces", label: "Spaces", icon: Globe, permission: "spaces.admin" },
			{ path: "/space-types", label: "Space Types", icon: Layout, permission: "space_types.create" },
			{ path: "/tags", label: "Tags", icon: Tag, permission: "tags.manage" },
			{ path: "/reactions", label: "Reactions", icon: Star, permission: "content.reaction_types.manage" }
			]
		},
		{
			id: "moderation",
			label: "Moderation",
			items: [
				{ path: "/moderation", label: "Queue", icon: Shield, permission: "admin.moderation" },
				{ path: "/moderation-queue", label: "Review Queue", icon: AlertTriangle, permission: "admin.moderation" },
				{ path: "/moderation/appeals", label: "Appeals", icon: MessageCircle, permission: "admin.moderation" },
				{ path: "/moderation/reports", label: "Reports", icon: FileText, permission: "admin.moderation" },
				{ path: "/moderation/content", label: "Content", icon: FileText, permission: "admin.moderation" },
				{ path: "/moderation/alerts", label: "Alerts", icon: AlertTriangle, permission: "admin.moderation" },
				{ path: "/spam", label: "Spam Management", icon: AlertTriangle, permission: "spam.ip_block.manage" },
			{ path: "/tickets", label: "Support Tickets", icon: Ticket, permission: "tickets.admin" },
			{ path: "/reviews", label: "Reviews", icon: MessageCircle, permission: "reviews.moderate" }
			]
		},
		{
			id: "analytics",
			label: "Analytics & Reports",
			items: [
				{ path: "/analytics", label: "Analytics", icon: BarChart3, permission: "admin.analytics.view" },
			{ path: "/reports", label: "Reports", icon: FileText, permission: "reports.review" },
			{ path: "/search", label: "Search Analytics", icon: Search, permission: "search.analytics.view" }
			]
		},
		{
			id: "engagement",
			label: "Engagement",
			items: [
			{ path: "/newsletter", label: "Newsletter", icon: Mail, permission: "newsletter.admin" },
			{ path: "/broadcast", label: "Broadcasts", icon: Bell, permission: "admin.broadcast" },
			{ path: "/forms", label: "Forms", icon: FileInput, permission: "forms.create" },
			{ path: "/learning", label: "Learning", icon: GraduationCap, permission: "learning.admin.manage" },
		{ path: "/mentorship", label: "Mentorship", icon: Users2, permission: "mentorship.admin" },
		{ path: "/onboarding", label: "Onboarding", icon: UserPlus, permission: "admin.settings.view" }
		]
	},
	{
		id: "monetization",
			label: "Monetization",
			items: [
				{ path: "/wallet", label: "Wallet", icon: Wallet, permission: "wallet.admin" },
			{ path: "/coupons", label: "Coupons", icon: Tag, permission: "coupons.manage" },
			{ path: "/discount-codes", label: "Discount Codes", icon: DollarSign, permission: "payments.discount_codes.manage" },
			{ path: "/affiliate", label: "Affiliate Links", icon: Link, permission: "affiliate.admin.view" },
			{ path: "/referral", label: "Referrals", icon: Share2, permission: "referral.admin" },
			{ path: "/ads", label: "Advertisements", icon: Zap, permission: "ad.campaign.manage" }
			]
		},
		{
			id: "automation",
			label: "Automation",
			items: [
				{ path: "/ai", label: "AI Tools", icon: Bot, permission: "admin.ai.view" },
				{ path: "/bots", label: "Bots", icon: Bot, permission: "bots.admin" },
				{ path: "/webhooks", label: "Webhooks", icon: Webhook, permission: "webhooks.manage" },
				{ path: "/feature-flags", label: "Feature Flags", icon: Zap, permission: "admin.feature_flags.view" }
			]
		},
		{
			id: "customization",
			label: "Customization",
			items: [
			{ path: "/page-builder", label: "Page Builder", icon: Layout, permission: "pagebuilder.blocks.manage" },
			{ path: "/page-builder/pages", label: "Pages", icon: FileText, permission: "pages.manage" },
			{ path: "/page-builder/themes", label: "Page Themes", icon: Palette, permission: "pagebuilder.themes.manage" },
			{ path: "/page-templates", label: "Page Templates", icon: FileText, permission: "pagebuilder.templates.manage" },
			{ path: "/custom-fields", label: "Custom Fields", icon: Type, permission: "custom_fields.group.view" },
			{ path: "/comparisons", label: "Comparisons", icon: GitCompare, permission: "comparison.type.create" },
			{ path: "/comparison-types", label: "Comparison Types", icon: ListOrdered, permission: "comparison.type.create" }
			]
		},
		{
			id: "system",
			label: "System",
			items: [
			{ path: "/settings", label: "Settings", icon: Settings, permission: "admin.settings.view" },
			{ path: "/i18n", label: "Translations", icon: Languages, permission: "i18n.manage" },
			{ path: "/notification-types", label: "Notifications", icon: Bell, permission: "notifications.manage" },
			{ path: "/jsonld-templates", label: "JSON-LD Templates", icon: FileCode, permission: "content.jsonld.manage" },
			{ path: "/menus", label: "Menus", icon: Menu, permission: "menus.manage" },
			{ path: "/email-templates", label: "Email Templates", icon: Mail, permission: "email.templates.manage" },
			{ path: "/redirects", label: "Redirects", icon: ArrowRightLeft, permission: "redirects.manage" },
			{ path: "/data-exports", label: "Data Exports", icon: Download, permission: "gdpr.data_exports.view" },
				{ path: "/deletions", label: "Account Deletions", icon: Trash2, permission: "admin.moderation.view" },
				{ path: "/gdpr", label: "GDPR Compliance", icon: Accessibility, permission: "admin.moderation.view" }
			]
		}
	];

	function toggleSection(id: string) {
		const next = new Set(expandedSections);
		if (next.has(id)) {
			next.delete(id);
		} else {
			next.add(id);
		}
		expandedSections = next;
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

<!-- Mobile overlay -->
{#if mobileOpen}
	<button
		type="button"
		class="fixed inset-0 z-40 bg-black/50 md:hidden"
		aria-label="Close sidebar"
		onclick={() => { mobileOpen = false; onclose?.(); }}
	></button>
{/if}

<aside
	class={clsx(
		"flex h-full flex-col border-r transition-transform duration-200",
		"fixed inset-y-0 left-0 z-50 w-64 md:static md:translate-x-0",
		!mobileOpen && "-translate-x-full md:translate-x-0",
		collapsed && "md:w-16"
	)}
	style="border-color: var(--color-border); background-color: var(--color-surface)"
>
	<!-- Logo -->
	<div class="flex h-14 items-center border-b px-4" style="border-color: var(--color-border)">
		<a href="/" class="flex items-center gap-2">
			{#if !collapsed}
				<span class="text-lg font-semibold" style="color: var(--color-foreground)">{t('lib.components.Sidebar.svelte.admin')}</span>
			{/if}
		</a>
	</div>

	<!-- Search input (expanded mode only) -->
	{#if !collapsed}
		<div class="px-3 pt-3">
			<div class="relative">
				<Search class="absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2" style="color: var(--color-muted)" />
				<input
					type="text"
					placeholder={t('common.search_nav', 'Search navigation...')}
					class="w-full rounded-md border py-1.5 pl-7 pr-3 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
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
						class="flex w-full items-center justify-between rounded-lg px-3 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/5"
						style="color: var(--color-foreground)"
						onclick={() => toggleSection(section.id)}
					>
						<span>{section.label}</span>
						{#if expandedSections.has(section.id)}
							<ChevronDown class="h-4 w-4" style="color: var(--color-muted)" />
						{:else}
							<ChevronRight class="h-4 w-4" style="color: var(--color-muted)" />
						{/if}
					</button>

					{#if expandedSections.has(section.id)}
						<div class="mt-1 space-y-1">
							{#each section.items as item}
								{#if canSeeItem(item.permission)}
									<a
										href={item.path}
										class="flex items-center gap-3 rounded-lg px-3 py-2 text-sm transition-colors"
										style={isActive(item.path)
											? "background-color: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-primary); font-weight: 500"
											: "color: var(--color-muted)"}
										onclick={() => { mobileOpen = false; }}
									>
										<item.icon
											class="h-5 w-5 flex-shrink-0"
											style={isActive(item.path) ? "color: var(--color-primary)" : "color: var(--color-muted)"}
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
					{#if canSeeItem(item.permission)}
						<Tooltip content={item.label} side="right">
							<a
								href={item.path}
								class="mb-1 flex items-center justify-center rounded-lg p-2 transition-colors"
								style={isActive(item.path)
									? "background-color: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-primary)"
									: "color: var(--color-muted)"}
							>
								<item.icon
									class="h-5 w-5"
									style={isActive(item.path) ? "color: var(--color-primary)" : "color: var(--color-muted)"}
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
		<div class="border-t p-4" style="border-color: var(--color-border)">
			<div class="flex items-center gap-3">
				<div class="flex h-8 w-8 items-center justify-center rounded-full" style="background-color: color-mix(in srgb, var(--color-primary) 15%, transparent)">
					<span class="text-sm font-medium" style="color: var(--color-primary)">
						{(user.current?.username ?? user.current?.email ?? 'A').charAt(0).toUpperCase()}
					</span>
				</div>
				<div class="min-w-0 flex-1">
					<p class="truncate text-sm font-medium" style="color: var(--color-foreground)">
						{user.current?.username ?? user.current?.email ?? 'Admin'}
					</p>
					<p class="truncate text-xs" style="color: var(--color-muted)">
						{user.current?.email ?? ''}
					</p>
				</div>
				<a
					href="/logout"
					class="rounded p-1.5 transition-colors hover:bg-black/5 dark:hover:bg-white/10"
					style="color: var(--color-muted)"
					title="Logout"
				>
					<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"></path><polyline points="16 17 21 12 16 7"></polyline><line x1="21" y1="12" x2="9" y2="12"></line></svg>
				</a>
			</div>
		</div>
	{/if}
</aside>
