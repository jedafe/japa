<script lang="ts">
	import { page } from '$app/state';
	import { t } from '$lib/stores/i18n.svelte';
	import { ChevronRight, Home } from 'lucide-svelte';

	type BreadcrumbItem = {
		label: string;
		href: string;
	};

	let items = $derived.by(() => {
		const path = page.url.pathname;
		const segments = path.split('/').filter(Boolean);
		const crumbs: BreadcrumbItem[] = [];
		
		// Always start with home
		crumbs.push({ label: t('dashboard.title'), href: '/' });
		
		// Build breadcrumbs from path segments
		let currentPath = '';
		for (const segment of segments) {
			currentPath += `/${segment}`;
			
			// Map segment to human-readable label
			let label = segment;
			switch (segment) {
				case 'forms':
					label = t('forms.title');
					break;
				case 'moderation':
					label = t('moderation.title');
					break;
				case 'content':
					label = t('moderation.content');
					break;
				case 'reports':
					label = t('moderation.reports');
					break;
				case 'appeals':
					label = t('moderation.appeals');
					break;
				case 'alerts':
					label = t('moderation.alerts');
					break;
				case 'users':
					label = t('users.title');
					break;
				case 'roles':
					label = t('users.roles.title');
					break;
				case 'groups':
					label = t('users.groups.title');
					break;
				case 'ai':
					label = t('ai.title');
					break;
				case 'tools':
					label = t('ai.tools.title');
					break;
				case 'generate':
					label = t('ai.generate.title');
					break;
				case 'learning':
					label = t('learning.title');
					break;
				case 'mentorship':
					label = t('mentorship.title');
					break;
				case 'webhooks':
					label = t('webhooks.title');
					break;
				case 'i18n':
					label = t('i18n.title');
					break;
				case 'spaces':
					label = t('spaces.title');
					break;
				case 'tags':
					label = t('tags.title');
					break;
				case 'settings':
					label = t('settings.title');
					break;
				case 'ads':
					label = t('ads.title');
					break;
				case 'tickets':
					label = t('tickets.title');
					break;
				case 'broadcast':
					label = t('broadcast.title');
					break;
				case 'translations':
					label = t('translations.title');
					break;
				case 'bots':
					label = t('bots.title');
					break;
				case 'docs':
					label = t('docs.title');
					break;
				case 'newsletter':
					label = t('newsletter.title');
					break;
				case 'analytics':
					label = t('analytics.title');
					break;
				case 'affiliate':
					label = t('affiliate.title');
					break;
				case 'referral':
					label = t('referral.title');
					break;
				case 'custom-fields':
					label = t('custom_fields.title');
					break;
				case 'notification-types':
					label = t('notification_types.title');
					break;
				case 'page-builder':
					label = t('page_builder.title');
					break;
				case 'wallet':
					label = t('wallet.title');
					break;
				case 'spam':
					label = t('spam.title');
					break;
				case 'gdpr':
					label = t('gdpr.title');
					break;
				case 'organisations':
					label = t('organisations.title');
					break;
				case 'reactions':
					label = t('reactions.title');
					break;
				case 'universities':
					label = t('universities.title');
					break;
				case 'instructors':
					label = t('university.instructors_title', 'University Instructors');
					break;
				case 'tracks':
					label = t('university.tracks_title', 'University Tracks');
					break;
				case 'payouts':
					label = t('university.payouts_title', 'University Payouts');
					break;
				case 'ticket-types':
					label = t('ticket_types.title');
					break;
				case 'plans':
					label = t('nav.plans', 'Subscription Plans');
					break;
				case 'space-types':
					label = t('space_types.title');
					break;
				case 'comparison-types':
					label = t('comparison_types.title');
					break;
				case 'comparison-profiles':
					label = t('comparison_profiles.title');
					break;
				case 'jsonld-templates':
					label = t('jsonld_templates.title');
					break;
				case 'new':
					label = t('common.new');
					break;
				default:
					// Skip UUID segments (e.g., form IDs, user IDs) — they're not human-readable
					if (/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(segment)) {
						continue;
					}
					// Try to capitalize first letter
					label = segment.charAt(0).toUpperCase() + segment.slice(1).replace(/-/g, ' ');
			}
			
			crumbs.push({ label, href: currentPath });
		}
		
		return crumbs;
	});
</script>

{#if items.length > 1}
	<nav class="mb-4 flex items-center gap-1 text-sm" aria-label="Breadcrumb">
		{#each items as item, i (i)}
			{#if i > 0}
				<ChevronRight class="h-3 w-3" style="color: var(--color-muted)" />
			{/if}
			{#if i === items.length - 1}
				<span class="font-medium" style="color: var(--color-text)">{item.label}</span>
			{:else}
				<a
					href={item.href}
					class="transition-colors hover:underline"
					style="color: var(--color-primary)"
				>
					{#if i === 0}
						<Home class="inline h-3 w-3" />
					{:else}
						{item.label}
					{/if}
				</a>
			{/if}
		{/each}
	</nav>
{/if}