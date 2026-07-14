<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent, CardHeader } from '$lib/components/ui/card';
	import { Tabs } from '$lib/components/ui/tabs';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import {
		Users,
		MessageSquare,
		TrendingUp,
		Settings,
		Shield,
		AlertTriangle,
		Clock,
		ArrowUpRight,
		ArrowDownRight,
		RefreshCw,
		Download,
		Activity,
		Zap,
		BookOpen,
		Ticket
	} from 'lucide-svelte';

	type OverviewData = {
		total_users: number;
		new_users_today: number;
		total_content: number;
		pending_tickets: number;
		active_users_7d?: number;
		pending_reports?: number;
		spam_queue_count?: number;
		total_generations_today?: number;
		hitl_pending_count?: number;
		recent_signups?: Array<{
			id: string;
			username: string;
			avatar_url?: string;
			created_at: string;
		}>;
		[key: string]: unknown;
	};

	let data = $state<OverviewData>({
		total_users: 0,
		new_users_today: 0,
		total_content: 0,
		pending_tickets: 0
	});
	let loading = $state(true);
	let activeTab = $state('overview');
	let period = $state('7d');

	const periods = [
		{ value: '24h', label: '24 Hours' },
		{ value: '7d', label: '7 Days' },
		{ value: '30d', label: '30 Days' },
		{ value: '90d', label: '90 Days' }
	];

	const tabs = [
		{ value: 'overview', label: t('dashboard.tab_overview', 'Overview'), icon: TrendingUp },
		{ value: 'community', label: t('dashboard.tab_community', 'Community'), icon: Users },
		{ value: 'moderation', label: t('dashboard.tab_moderation', 'Moderation'), icon: Shield },
		{ value: 'ai', label: t('dashboard.tab_ai', 'AI Usage'), icon: Zap }
	];

	const statsCards = $derived([
		{
			label: t('dashboard.total_users', 'Total Users'),
			value: data.total_users,
			change: 0,
			icon: Users,
			href: '/users'
		},
		{
			label: t('dashboard.active_users', 'Active Users (7d)'),
			value: data.active_users_7d ?? 0,
			change: 0,
			icon: Users,
			href: '/users?tab=active'
		},
		{
			label: t('dashboard.total_content', 'Total Content'),
			value: data.total_content,
			change: 0,
			icon: MessageSquare,
			href: '/content'
		},
		{
			label: t('dashboard.open_tickets', 'Open Tickets'),
			value: data.pending_tickets,
			change: 0,
			icon: Ticket,
			href: '/tickets'
		}
	]);

	const moderationStats = $derived([
		{
			label: t('dashboard.pending_reports', 'Pending Reports'),
			value: data.pending_reports ?? 0,
			icon: Clock,
			href: '/moderation?tab=pending'
		},
		{
			label: t('dashboard.pending_tickets', 'Pending Tickets'),
			value: data.pending_tickets,
			icon: AlertTriangle,
			href: '/tickets'
		},
		{
			label: t('dashboard.spam_queue', 'Spam Queue'),
			value: data.spam_queue_count ?? 0,
			icon: Shield,
			href: '/moderation?tab=spam'
		}
	]);

	const aiStats = $derived([
		{
			label: t('dashboard.generations_today', 'Generations Today'),
			value: data.total_generations_today ?? 0,
			icon: Zap
		},
		{
			label: t('dashboard.hitl_pending', 'HITL Pending Review'),
			value: data.hitl_pending_count ?? 0,
			icon: Clock
		},
		{
			label: t('dashboard.new_today', 'New Users Today'),
			value: data.new_users_today,
			icon: Activity
		}
	]);

	const recentSignups = $derived(data.recent_signups ?? []);

	const quickLinks = $derived([
		{ label: 'Users', icon: Users, href: '/users', count: data.total_users },
		{ label: 'Content', icon: BookOpen, href: '/content', count: data.total_content },
		{ label: 'Moderation', icon: Shield, href: '/moderation', count: data.pending_reports },
		{ label: 'Tickets', icon: Ticket, href: '/tickets', count: data.pending_tickets },
		{ label: 'Analytics', icon: TrendingUp, href: '/analytics' },
		{ label: 'Settings', icon: Settings, href: '/settings' }
	]);

	async function load() {
		loading = true;
		try {
			const params = new URLSearchParams({ period });
			const result = await api<OverviewData>(`/admin/analytics/overview?${params}`);
			if (result.ok) data = result.data;
		} catch {
			// API not available
		} finally {
			loading = false;
		}
	}

	function setPeriod(p: string) {
		period = p;
		load();
	}

	function refreshDashboard() {
		goto('', { invalidateAll: true });
		load();
	}

	onMount(load);
</script>

<svelte:head>
	<title>Dashboard — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t('dashboard.title', 'Dashboard')}
	description={t('dashboard.description', "Welcome back! Here's what's happening on your platform.")}
	actions={[
		{
			label: 'Refresh',
			icon: RefreshCw,
			variant: 'outline',
			onclick: refreshDashboard
		},
		{
			label: 'Export Report',
			icon: Download,
			variant: 'outline'
		}
	]}
/>

<!-- Period Selector -->
<div class="mb-6 flex items-center gap-2">
	<span class="text-sm text-gray-500">Period:</span>
	{#each periods as p}
		<Button
			variant={period === p.value ? 'default' : 'ghost'}
			size="sm"
			onclick={() => setPeriod(p.value)}
		>
			{p.label}
		</Button>
	{/each}
</div>

{#if loading}
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
		{#each Array(4) as _}
			<Card>
				<CardContent class="p-6">
					<div class="skeleton mb-2 h-4 w-20 rounded-lg"></div>
					<div class="skeleton h-8 w-16 rounded-lg"></div>
				</CardContent>
			</Card>
		{/each}
	</div>
{:else}
	<div class="space-y-6">
		<Tabs {tabs} bind:value={activeTab} />

		{#if activeTab === 'overview'}
			<!-- Stats Grid -->
			<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
				{#each statsCards as stat}
					<a href={stat.href}>
						<Card class="transition-shadow hover:shadow-md">
							<CardContent class="p-6">
								<div class="flex items-center justify-between">
									<div>
										<p class="text-sm font-medium text-gray-500">{stat.label}</p>
										<p class="text-2xl font-bold text-gray-900">{stat.value}</p>
										<p class="flex items-center gap-1 text-sm text-gray-500">
											{#if stat.change >= 0}
												<ArrowUpRight class="h-4 w-4 text-success-600" />
											{:else}
												<ArrowDownRight class="h-4 w-4 text-danger-600" />
											{/if}
											{Math.abs(stat.change)}% from last period
										</p>
									</div>
									<div class="rounded-lg bg-primary-100 p-3">
										<stat.icon class="h-6 w-6 text-primary-600" />
									</div>
								</div>
							</CardContent>
						</Card>
					</a>
				{/each}
			</div>

			<!-- Quick Links -->
			<Card>
				<CardHeader>
					<h3 class="text-lg font-semibold text-gray-900">
						{t('dashboard.quick_links', 'Quick Links')}
					</h3>
				</CardHeader>
				<CardContent>
					<div class="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-6">
						{#each quickLinks as link}
							<a
								href={link.href}
								class="flex flex-col items-center gap-2 rounded-lg border border-gray-200 p-4 transition-colors hover:bg-gray-50"
							>
								<link.icon class="h-6 w-6 text-gray-600" />
								<span class="text-sm font-medium text-gray-900">{link.label}</span>
								{#if link.count !== undefined}
									<Badge variant="secondary">{link.count}</Badge>
								{/if}
							</a>
						{/each}
					</div>
				</CardContent>
			</Card>

			<!-- Community Health -->
			<div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
				<!-- Recent Signups -->
				<Card>
					<CardHeader>
						<div class="flex items-center justify-between">
							<h3 class="text-lg font-semibold text-gray-900">
								{t('dashboard.recent_signups', 'Recent Signups')}
							</h3>
							<Button variant="ghost" size="sm" href="/users">View All</Button>
						</div>
					</CardHeader>
					<CardContent>
						{#if recentSignups.length > 0}
							<div class="space-y-3">
								{#each recentSignups as user}
									<a
										href="/users/{user.id}"
										class="flex items-center gap-3 rounded-lg px-3 py-2 transition-colors hover:bg-gray-50"
									>
										<div
											class="flex h-8 w-8 items-center justify-center rounded-full text-sm font-bold text-white"
											style="background: var(--gradient-primary)"
										>
											{user.username.charAt(0).toUpperCase()}
										</div>
										<div class="min-w-0 flex-1">
											<p class="truncate text-sm font-medium text-gray-900">{user.username}</p>
											<p class="text-xs text-gray-500">
												{new Date(user.created_at).toLocaleDateString()}
											</p>
										</div>
									</a>
								{/each}
							</div>
						{:else}
							<p class="py-4 text-center text-sm text-gray-500">
								{t('dashboard.no_signups', 'No recent signups')}
							</p>
						{/if}
					</CardContent>
				</Card>

				<!-- AI Usage Widget -->
				<Card>
					<CardHeader>
						<div class="flex items-center justify-between">
							<h3 class="text-lg font-semibold text-gray-900">
								{t('dashboard.ai_usage', 'AI Usage')}
							</h3>
							<Button variant="ghost" size="sm" href="/ai">View All</Button>
						</div>
					</CardHeader>
					<CardContent>
						<div class="space-y-3">
							{#each aiStats as stat}
								<div
									class="flex items-center justify-between rounded-lg border border-gray-200 px-4 py-3"
								>
									<div>
										<p class="text-2xl font-bold text-gray-900">{stat.value}</p>
										<p class="text-xs text-gray-500">{stat.label}</p>
									</div>
									<div class="rounded-lg bg-primary-100 p-2">
										<stat.icon class="h-5 w-5 text-primary-600" />
									</div>
								</div>
							{/each}
						</div>
					</CardContent>
				</Card>
			</div>
		{/if}

		{#if activeTab === 'community'}
			<div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
				<!-- Community Stats -->
				<Card>
					<CardHeader>
						<h3 class="text-lg font-semibold text-gray-900">
							{t('dashboard.community_health', 'Community Health')}
						</h3>
					</CardHeader>
					<CardContent>
						<div class="space-y-4">
							<div class="flex items-center justify-between">
								<span class="text-sm text-gray-600">{t('dashboard.total_users', 'Total Users')}</span>
								<span class="font-medium text-gray-900">{data.total_users}</span>
							</div>
							<div class="flex items-center justify-between">
								<span class="text-sm text-gray-600">{t('dashboard.new_today', 'New Today')}</span>
								<span class="font-medium text-gray-900">{data.new_users_today}</span>
							</div>
							<div class="flex items-center justify-between">
								<span class="text-sm text-gray-600">{t('dashboard.total_content', 'Content Created')}</span>
								<span class="font-medium text-gray-900">{data.total_content}</span>
							</div>
							<div class="flex items-center justify-between">
								<span class="text-sm text-gray-600">{t('dashboard.active_users_7d', 'Active (7d)')}</span>
								<span class="font-medium text-gray-900">{data.active_users_7d ?? 0}</span>
							</div>
						</div>
					</CardContent>
				</Card>

				<!-- Top Spaces (placeholder) -->
				<Card>
					<CardHeader>
						<h3 class="text-lg font-semibold text-gray-900">
							{t('dashboard.top_spaces', 'Top Spaces')}
						</h3>
					</CardHeader>
					<CardContent>
						<p class="py-4 text-center text-sm text-gray-500">
							{t('dashboard.space_data_coming', 'Space data coming soon')}
						</p>
					</CardContent>
				</Card>
			</div>
		{/if}

		{#if activeTab === 'moderation'}
			<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
				{#each moderationStats as stat}
					<a href={stat.href}>
						<Card class="transition-shadow hover:shadow-md">
							<CardContent class="p-6">
								<div class="flex items-center justify-between">
									<div>
										<p class="text-sm font-medium text-gray-500">{stat.label}</p>
										<p class="text-2xl font-bold text-gray-900">{stat.value}</p>
									</div>
									<div
										class="rounded-lg p-3"
										style="background: color-mix(in srgb, var(--color-warning) 12%, transparent)"
									>
										<stat.icon
											class="h-6 w-6"
											style="color: var(--color-warning)"
										/>
									</div>
								</div>
							</CardContent>
						</Card>
					</a>
				{/each}
			</div>

			<Card>
				<CardHeader>
					<h3 class="text-lg font-semibold text-gray-900">
						{t('dashboard.pending_moderation', 'Pending Moderation')}
					</h3>
				</CardHeader>
				<CardContent>
					<p class="py-4 text-center text-sm text-gray-500">
						{t('dashboard.pending_items', 'No pending items')}
					</p>
				</CardContent>
			</Card>
		{/if}

		{#if activeTab === 'ai'}
			<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
				{#each aiStats as stat}
					<Card>
						<CardContent class="p-6">
							<div class="flex items-center justify-between">
								<div>
									<p class="text-sm font-medium text-gray-500">{stat.label}</p>
									<p class="text-2xl font-bold text-gray-900">{stat.value}</p>
								</div>
								<div class="rounded-lg bg-primary-100 p-3">
									<stat.icon class="h-6 w-6 text-primary-600" />
								</div>
							</div>
						</CardContent>
					</Card>
				{/each}
			</div>

			<Card>
				<CardHeader>
					<h3 class="text-lg font-semibold text-gray-900">
						{t('dashboard.recent_ai_usage', 'Recent AI Usage')}
					</h3>
				</CardHeader>
				<CardContent>
					<p class="py-4 text-center text-sm text-gray-500">
						{t('dashboard.no_ai_usage', 'No recent AI usage data')}
					</p>
				</CardContent>
			</Card>
		{/if}
	</div>
{/if}
