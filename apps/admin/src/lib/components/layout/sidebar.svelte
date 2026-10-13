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
		Clock, ChevronDown, ChevronRight,
		Briefcase, Eye, X, Menu, ArrowRightLeft, CreditCard,
		Package, Receipt, PanelLeft,
		Brain, Wand, Trophy, Plane, Banknote, Megaphone,
		FileUp, LayoutGrid, Boxes,
		BookOpenText, ShieldAlert, LifeBuoy, Target,
		CalendarDays, Newspaper, Award, Building2,
		Route, ListChecks, BadgeCheck, Home,
		FileCheck, Globe2, Calculator, MapPin,
		CalendarClock, ClipboardCheck, Gauge, PiggyBank, Timer,
		ScanSearch, Video, Inbox, Rss, Tags,
		Map as MapIcon
	} from "lucide-svelte";
	import { clsx } from "clsx";
	import { sidebarCollapsed } from "$lib/stores/sidebar.svelte";

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

	// Prefer bound collapsed prop, fall back to shared sidebar store.
	let effectiveCollapsed = $derived(collapsed ?? sidebarCollapsed());

	let expandedSections = $state(new Set(["dashboard", "users", "content", "moderation", "community", "v2-tools"]));
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
			label: t("nav.section.dashboard", "Dashboard"),
			items: [
				{ path: "/", label: t("nav.overview", "Overview"), icon: LayoutDashboard }
			]
		},
		{
			id: "users",
			label: t("nav.section.users", "User Management"),
			items: [
				{ path: "/users", label: t("nav.users", "Users"), icon: Users, permission: "admin.users.view" },
				{ path: "/organisations", label: t("nav.organisations", "Organisations"), icon: Building, permission: "admin.moderation" },
			{ path: "/organisations/claims", label: t("nav.organisation_claims", "Ownership Claims"), icon: FileText, permission: "admin.moderation" },
				{ path: "/universities", label: t("nav.universities", "Universities"), icon: GraduationCap, permission: "admin.settings" },
				{ path: "/universities/course-catalog", label: t("nav.course_catalog", "Course Catalog"), icon: BookOpen, permission: "universities.course_catalog.list" },
				{ path: "/universities/course-offerings", label: t("nav.course_offerings", "Course Offerings"), icon: GraduationCap, permission: "universities.course_offering.list" },
				{ path: "/users/groups", label: t("nav.users_groups", "Groups"), icon: Users2, permission: "admin.users.view" }
			]
		},
		{
			id: "content",
			label: t("nav.section.content", "Content"),
			items: [
				{ path: "/content", label: t("nav.content", "All Content"), icon: FileText, permission: "admin.content.view" },
				{ path: "/content/videos", label: t("nav.video_sources", "Video Sources"), icon: Video, permission: "admin.video_sources.read" },
				{ path: "/content/video-imports", label: t("nav.video_import_jobs", "Video Imports"), icon: FileUp, permission: "admin.video_import_jobs.read" },
				{ path: "/content/video-claims", label: t("nav.video_claims", "Video Claims"), icon: Shield, permission: "admin.video_claims.read" },
				{ path: "/content-types", label: t("nav.content_types", "Content Types"), icon: LayoutGrid, permission: "admin.settings" },
				{ path: "/blog/review-queue", label: t("nav.blog_review_queue", "Blog Posts"), icon: BookOpen, permission: "blog.review.manage" },
			{ path: "/docs", label: t("nav.docs", "Documentation"), icon: FileCode, permission: "docs.admin" },
			{ path: "/docs/categories", label: t("nav.docs_categories", "Doc Categories"), icon: Tags, permission: "admin.settings" },
			{ path: "/media", label: t("nav.media", "Media"), icon: Image, permission: "media.view_all" },
			{ path: "/spaces", label: t("nav.spaces", "Spaces"), icon: Globe, permission: "spaces.admin" },
			{ path: "/geo", label: t("nav.geo", "Geo Tools"), icon: Globe, permission: "geo.lookup" },
			{ path: "/space-types", label: t("nav.space_types", "Space Types"), icon: Layout, permission: "spaces.types.create" },
			{ path: "/tags", label: t("nav.tags", "Tags"), icon: Tag, permission: "tags.manage" },
			{ path: "/reactions", label: t("nav.reactions", "Reactions"), icon: Star, permission: "content.reaction_types.manage" },
		{ path: "/share-platforms", label: t("nav.share_platforms", "Share Platforms"), icon: Share2, permission: "content.reaction_types.manage" },
			{ path: "/roadmap", label: t("nav.roadmap", "Roadmap"), icon: MapIcon, permission: "roadmap.view" },
			{ path: "/roadmap/submissions", label: t("nav.roadmap_submissions", "Roadmap Submissions"), icon: Inbox, permission: "roadmap.moderate" },
			{ path: "/roadmap/categories", label: t("nav.roadmap_categories", "Roadmap Categories"), icon: Tags, permission: "roadmap.edit" },
	{ path: "/imports", label: t("nav.imports", "Content Imports"), icon: FileUp, permission: "import.manage" }
			]
		},
		{
			id: "moderation",
			label: t("nav.section.moderation", "Moderation"),
			items: [
				{ path: "/moderation", label: t("nav.moderation", "Queue"), icon: Shield, permission: "admin.moderation" },
				{ path: "/moderation-queue", label: t("nav.moderation_queue", "Review Queue"), icon: AlertTriangle, permission: "admin.moderation" },
				{ path: "/moderation/appeals", label: t("nav.moderation_appeals", "Appeals"), icon: MessageCircle, permission: "reports.review" },
				{ path: "/moderation/reports", label: t("nav.moderation_reports", "Reports"), icon: FileText, permission: "reports.review" },
				{ path: "/moderation/content", label: t("nav.moderation_content", "Content"), icon: FileText, permission: "moderation.content.review" },
				{ path: "/moderation/alerts", label: t("nav.moderation_alerts", "Alerts"), icon: AlertTriangle, permission: "moderation.alerts.manage" },
				{ path: "/spam", label: t("nav.spam", "Spam Management"), icon: AlertTriangle, permission: "spam.ip_block.manage" },
			{ path: "/tickets", label: t("nav.tickets", "Support Tickets"), icon: Ticket, permission: "tickets.admin" },
		{ path: "/ticket-types", label: t("nav.ticket_types", "Ticket Types"), icon: Tag, permission: "tickets.type.manage" },
			{ path: "/reviews", label: t("nav.reviews", "Reviews"), icon: MessageCircle, permission: "reviews.moderate" }
			]
		},
		{
			id: "analytics",
			label: t("nav.section.analytics", "Analytics & Reports"),
			items: [
				{ path: "/analytics", label: t("nav.analytics", "Analytics"), icon: BarChart3, permission: "admin.analytics.view" },
			{ path: "/reports", label: t("nav.reports", "Reports"), icon: FileText, permission: "reports.review" },
			{ path: "/search", label: t("nav.search", "Search Analytics"), icon: Search, permission: "search.analytics.view" }
			]
		},
		{
			id: "engagement",
			label: t("nav.section.engagement", "Engagement"),
			items: [
			{ path: "/newsletter", label: t("nav.newsletter", "Newsletter"), icon: Mail, permission: "newsletter.manage" },
			{ path: "/broadcasts", label: t("nav.broadcasts", "Broadcasts"), icon: Bell, permission: "admin.broadcast" },
			{ path: "/forms", label: t("nav.forms", "Forms"), icon: FileInput, permission: "forms.create" },
			{ path: "/academy", label: t("nav.academy", "Academy"), icon: GraduationCap, permission: "academy.admin.manage" },
			{ path: "/academy/instructors", label: t("nav.academy_instructors", "Academy Instructors"), icon: GraduationCap, permission: "academy.instructor.approve" },
			{ path: "/academy/tracks", label: t("nav.academy_tracks", "Academy Tracks"), icon: BookOpen, permission: "academy.course.approve" },
		{ path: "/academy/curriculum", label: t("nav.academy_curriculum", "Curriculum"), icon: BookOpen, permission: "academy.admin.manage" },
		{ path: "/academy/categories", label: t("nav.academy_categories", "Track Categories"), icon: Tag, permission: "academy.category.manage" },
			{ path: "/academy/payouts", label: t("nav.academy_payouts", "Academy Payouts"), icon: DollarSign, permission: "academy.payout.process" },
		{ path: "/academy/revenue-splits", label: t("nav.academy_revenue_splits", "Revenue Splits"), icon: DollarSign, permission: "academy.revenue.list" },
		{ path: "/mentorship", label: t("nav.mentorship", "Mentorship"), icon: Users2, permission: "mentorship.admin" },
		{ path: "/onboarding/templates", label: t("nav.onboarding_templates", "Onboarding Templates"), icon: FileText, permission: "admin.settings.view" },
		{ path: "/gamification", label: t("nav.gamification", "Gamification"), icon: Trophy, permission: "gamification.view" },
		{ path: "/jobs", label: t("nav.jobs", "Jobs Board"), icon: Briefcase, permission: "jobs.admin" }
		]
	},
	{
		id: "community",
		label: t("nav.section.community", "Community (V2)"),
		items: [
			{ path: "/community/story-wall", label: t("nav.community_story_wall", "Story Wall"), icon: BookOpenText, permission: "story.verify" },
			{ path: "/community/wiki", label: t("nav.community_wiki", "Wiki Revisions"), icon: BookOpen, permission: "wiki.review" },
			{ path: "/community/scam-alerts", label: t("nav.community_scam_alerts", "Scam Alerts"), icon: ShieldAlert, permission: "scam_alert.review" },
			{ path: "/community/planning-groups", label: t("nav.community_planning_groups", "Planning Groups"), icon: Target, permission: "planning_group.manage_any" },
			{ path: "/community/mutual-aid", label: t("nav.community_mutual_aid", "Mutual Aid"), icon: LifeBuoy, permission: "mutual_aid.moderate" },
			{ path: "/community/calendar", label: t("nav.community_calendar", "Calendar"), icon: CalendarDays, permission: "calendar.review" },
			{ path: "/community/digest", label: t("nav.community_digest", "Digest"), icon: Newspaper, permission: "digest.manage" },
			{ path: "/community/achievements", label: t("nav.community_achievements", "Achievements"), icon: Award, permission: "achievement.moderate" },
			{ path: "/community/services", label: t("nav.community_services", "Service Directory"), icon: Building2, permission: "service.verify" },
			{ path: "/community/service-bookings", label: t("nav.community_service_bookings", "Service Bookings"), icon: Inbox, permission: "service.booking.admin" }
		]
	},
	{
		id: "v2-tools",
		label: t("nav.section.v2_tools", "Tools (V2)"),
		items: [
			{ path: "/tools/appointment-alert", label: t("nav.tools_appointment_alert", "Embassy Alerter"), icon: CalendarClock, permission: "embassy_monitor.manage" },
			{ path: "/tools/cv-formatter", label: t("nav.tools_cv_formatter", "CV Formatter"), icon: FileText, permission: "cv_formatter.manage" },
			{ path: "/tools/visa-checklist", label: t("nav.tools_visa_checklist", "Visa Checklist"), icon: ClipboardCheck, permission: "checklist.manage" },
			{ path: "/tools/readiness", label: t("nav.tools_readiness", "Readiness Assessment"), icon: Gauge, permission: "assessment.view_results" },
			{ path: "/tools/cost-of-living", label: t("nav.tools_cost_of_living", "Cost of Living"), icon: PiggyBank, permission: "col_calculator.manage" },
			{ path: "/tools/scholarships", label: t("nav.tools_scholarships", "Scholarship Finder"), icon: GraduationCap, permission: "scholarship.manage" },
			{ path: "/tools/processing-tracker", label: t("nav.tools_processing_tracker", "Processing Tracker"), icon: Timer, permission: "visa_timeline.manage_any" },
			{ path: "/tools/visa-routes", label: t("nav.tools_visa_routes", "Visa Routes"), icon: Route, permission: "visa_routes.manage" },
			{ path: "/tools/arrival-checklist", label: t("nav.tools_arrival_checklist", "Arrival Checklist"), icon: ListChecks, permission: "arrival_checklist.manage" },
			{ path: "/tools/credential-checker", label: t("nav.tools_credential_checker", "Credential Checker"), icon: BadgeCheck, permission: "credential_checker.manage" },
			{ path: "/tools/housing-guide", label: t("nav.tools_housing_guide", "Housing Guide"), icon: Home, permission: "housing_guide.manage" },
			{ path: "/tools/attestation-guide", label: t("nav.tools_attestation_guide", "Attestation Guide"), icon: FileCheck, permission: "attestation_guide.manage" },
			{ path: "/tools/language-test-hub", label: t("nav.tools_language_test_hub", "Language Test Hub"), icon: Globe2, permission: "language_test_hub.manage" },
			{ path: "/tools/tax-calculator", label: t("nav.tools_tax_calculator", "Tax Calculator"), icon: Calculator, permission: "tax_calculator.manage" },
			{ path: "/tools/community-locator", label: t("nav.tools_community_locator", "Community Locator"), icon: MapPin, permission: "community_locator.manage" },
			{ path: "/crs-calculator", label: t("nav.tools_crs_calculator", "CRS Calculator"), icon: Calculator, permission: "crs_calculator.manage" },
			{ path: "/pof-calculator", label: t("nav.tools_pof_calculator", "POF Calculator"), icon: Banknote, permission: "pof_calculator.manage" },
			{ path: "/eca-guide", label: t("nav.tools_eca_guide", "ECA Guide"), icon: BadgeCheck, permission: "eca_guide.manage" },
			{ path: "/sop-generator", label: t("nav.tools_sop_generator", "SOP Generator"), icon: FileText, permission: "sop_generator.manage" }
		]
	},
	{
		id: "intelligence",
		label: t("nav.section.intelligence", "Intelligence"),
		items: [
			{ path: "/intelligence/review", label: t("nav.intel_review", "Review Queue"), icon: ScanSearch, permission: "intel.review.view" }
		]
	},
	{
		id: "monetization",
			label: t("nav.section.monetization", "Monetization"),
			items: [
				{ path: "/wallet", label: t("nav.wallet", "Wallet"), icon: Wallet, permission: "wallet.admin" },
			{ path: "/wallet/credit-packs", label: t("nav.credit_packs", "Credit Packs"), icon: Package, permission: "wallet.credit_pack.manage" },
			{ path: "/wallet/purchases", label: t("nav.credit_purchases", "Credit Purchases"), icon: Receipt, permission: "wallet.purchases.view_any" },
			{ path: "/wallet/withdrawals", label: t("nav.withdrawals", "Withdrawals"), icon: ArrowRightLeft, permission: "wallet.withdraw.approve" },
			{ path: "/deals", label: t("nav.deals", "Deals"), icon: Tag, permission: "coupons.manage" },
			{ path: "/coupon-feeds", label: t("nav.coupon_feeds", "Deal Feeds"), icon: Rss, permission: "coupons.manage" },
			{ path: "/discount-codes", label: t("nav.discount_codes", "Discount Codes"), icon: DollarSign, permission: "payments.discount_codes.manage" },
			{ path: "/plans", label: t("nav.plans", "Subscription Plans"), icon: CreditCard, permission: "payments.plans.manage" },
			{ path: "/affiliate", label: t("nav.affiliate", "Affiliate Links"), icon: Link, permission: "affiliate.admin.view" },
			{ path: "/referral", label: t("nav.referral", "Referrals"), icon: Share2, permission: "referral.manage" },
			{ path: "/ads", label: t("nav.ads", "Advertisements"), icon: Zap, permission: "ad.campaign.manage" },
			{ path: "/ads/slot-types", label: t("nav.slot_types", "Slot Types"), icon: ListOrdered, permission: "ad.campaign.manage" },
			{ path: "/ads/networks", label: t("nav.ad_networks", "Ad Networks"), icon: Globe, permission: "ad.network.manage" },
		{ path: "/travel", label: t("nav.travel", "Travel Providers"), icon: Plane, permission: "travel.manage" },
		{ path: "/remittance", label: t("nav.remittance", "Remittance Providers"), icon: Banknote, permission: "remittance.manage" }
		]
		},
		{
			id: "automation",
			label: t("nav.section.automation", "Automation"),
			items: [
				{ path: "/ai", label: t("nav.ai", "AI Tools"), icon: Bot, permission: "admin.ai.view" },
				{ path: "/personas", label: t("nav.personas", "AI Personas"), icon: Bot, permission: "persona.view" },
			{ path: "/persona-voices", label: t("nav.persona_voices", "Persona Voices"), icon: Bot, permission: "persona.view" },
				{ path: "/studio", label: t("nav.studio", "Content Studio"), icon: Wand, permission: "studio.view" },
			{ path: "/campaigns", label: t("nav.campaigns", "Campaigns"), icon: Megaphone, permission: "campaign.view" },
				{ path: "/participation", label: t("nav.participation", "Participation Queue"), icon: Bot, permission: "participation.view" },
				{ path: "/rag", label: t("nav.rag", "RAG Index"), icon: Brain, permission: "ai.rag.manage" },
				{ path: "/bots", label: t("nav.bots", "Bots"), icon: Bot, permission: "bots.admin" },
				{ path: "/webhooks", label: t("nav.webhooks", "Webhooks"), icon: Webhook, permission: "integrations.webhooks.manage" }
			]
		},
		{
			id: "customization",
			label: t("nav.section.customization", "Customization"),
			items: [
			{ path: "/page-builder", label: t("nav.page_builder", "Page Builder"), icon: Layout, permission: "pagebuilder.blocks.manage" },
			{ path: "/page-builder/pages", label: t("nav.page_builder_pages", "Pages"), icon: FileText, permission: "pages.manage" },
				{ path: "/page-builder/themes", label: t("nav.page_builder_themes", "Page Themes"), icon: Palette, permission: "pagebuilder.themes.manage" },
			{ path: "/page-templates", label: t("nav.page_templates", "Page Templates"), icon: FileText, permission: "pagebuilder.templates.manage" },
			{ path: "/widgets", label: t("nav.widgets", "Widgets"), icon: Boxes, permission: "widgets.manage" },
			{ path: "/custom-fields", label: t("nav.custom_fields", "Custom Fields"), icon: Type, permission: "custom_fields.group.view" },
			{ path: "/custom-fields/entity-types", label: t("nav.custom_field_entity_types", "Entity Types"), icon: Type, permission: "custom_fields.group.view" },
			{ path: "/guide-categories", label: t("nav.guide_categories", "Guide Categories"), icon: Type, permission: "custom_fields.group.view" },
			{ path: "/comparisons", label: t("nav.comparisons", "Comparisons"), icon: GitCompare, permission: "comparison.create" },
			{ path: "/comparison-types", label: t("nav.comparison_types", "Comparison Types"), icon: ListOrdered, permission: "comparison.type.create" },
			{ path: "/comparison-data-sources", label: t("nav.comparison_data_sources", "Data Sources"), icon: Database, permission: "jce.data_source.view" },
			{ path: "/comparison-ingestion-jobs", label: t("nav.comparison_ingestion_jobs", "Ingestion Jobs"), icon: Timer, permission: "jce.ingestion_job.view" }
			]
		},
		{
			id: "system",
			label: t("nav.section.system", "System"),
			items: [
			{ path: "/settings", label: t("nav.settings", "Settings"), icon: Settings, permission: "admin.settings.view" },
			{ path: "/seo", label: t("nav.seo_validation", "SEO Validation"), icon: Search, permission: "admin.settings" },
			{ path: "/i18n", label: t("nav.i18n", "Translations"), icon: Languages, permission: "i18n.manage" },
			{ path: "/notification-types", label: t("nav.notification_types", "Notifications"), icon: Bell, permission: "notifications.manage" },
			{ path: "/jsonld-templates", label: t("nav.jsonld_templates", "JSON-LD Templates"), icon: FileCode, permission: "content.jsonld.manage" },
			{ path: "/literals", label: t("nav.literals", "Template Literals"), icon: FileCode, permission: "content.literals.manage" },
			{ path: "/menus", label: t("nav.menus", "Menus"), icon: Menu, permission: "menus.manage" },
			{ path: "/sidebars", label: t("nav.sidebars", "Sidebars"), icon: PanelLeft, permission: "sidebars.manage" },
			{ path: "/email-templates", label: t("nav.email_templates", "Email Templates"), icon: Mail, permission: "email.templates.manage" },
			{ path: "/redirects", label: t("nav.redirects", "Redirects"), icon: ArrowRightLeft, permission: "redirects.manage" },
			{ path: "/data-exports", label: t("nav.data_exports", "Data Exports"), icon: Download, permission: "gdpr.data_exports.view" },
			{ path: "/data-imports", label: t("nav.data_imports", "Data Imports"), icon: FileUp, permission: "gdpr.data_exports.view" },
				{ path: "/deletions", label: t("nav.deletions", "Account Deletions"), icon: Trash2, permission: "gdpr.deletions.view" },
				{ path: "/gdpr", label: t("nav.gdpr", "GDPR Compliance"), icon: Accessibility, permission: "gdpr.deletions.view" }
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
		effectiveCollapsed && "md:w-16"
	)}
	style="border-color: var(--color-border); background-color: var(--color-surface)"
>
	<!-- Logo -->
	<div class="flex h-14 items-center border-b px-4" style="border-color: var(--color-border)">
		<a href="/" class="flex items-center gap-2">
			{#if !effectiveCollapsed}
				<span class="text-lg font-semibold" style="color: var(--color-foreground)">{t('lib.components.Sidebar.svelte.admin')}</span>
			{/if}
		</a>
	</div>

	<!-- Search input (expanded mode only) -->
	{#if !effectiveCollapsed}
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
		{#each filterSections(searchQuery) as section (section.id)}
			{#if !effectiveCollapsed}
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
							{#each section.items as item (item.path)}
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
				{#each section.items as item (item.path)}
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
	{#if !effectiveCollapsed}
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
