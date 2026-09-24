/**
 * Static route paths for JapaConnect apps, used by the menu editor URL combobox.
 * These are structural paths (not configurable data) — they change only when
 * routes are added/removed in the SvelteKit apps.
 *
 * Ref: japaconnect-engineering/sections/23-main-site-frontend.md
 */

// Main site (apps/main) — port 3000
const mainRoutes: string[] = [
	'/',
	'/blog',
	'/blog/series',
	'/bookmarks',
	'/certificates',
	'/claim-ownership',
	'/compare',
	'/compare/new',
	'/content',
	'/coupons',
	'/create',
	'/dashboard',
	'/dashboard/affiliate',
	'/dashboard/bookmarks',
	'/dashboard/media',
	'/dashboard/organizations',
	'/dashboard/spaces',
	'/dashboard/tickets',
	'/dashboard/wallet',
	'/dashboard/wallet/purchase',
	'/discover',
	'/explore',
	'/feedback',
	'/forum',
	'/login',
	'/logout',
	'/messages',
	'/notifications',
	'/onboarding',
	'/organizations',
	'/organizations/new',
	'/pages',
	'/pricing',
	'/profiles',
	'/search',
	'/settings',
	'/signup',
	'/spaces',
	'/tags',
	'/trending',
	'/universities',
	'/verify-email',
];

// Docs site (apps/docs) — port 3004
const docsRoutes: string[] = [
	'/',
	'/docs',
	'/docs/search',
];

// Learn platform (apps/learn) — port 3003
const learnRoutes: string[] = [
	'/',
	'/bookmarks',
	'/certificates',
	'/dashboard/instructor',
	'/dashboard/instructor/apply',
	'/my-learning',
	'/paths',
	'/tracks',
];

// Admin panel (apps/admin) — port 3001
const adminRoutes: string[] = [
	'/',
	'/ads',
	'/ads/new',
	'/affiliate',
	'/ai',
	'/ai/tools',
	'/analytics',
	'/blog',
	'/blog/new',
	'/blog/review-queue',
	'/bots',
	'/broadcasts',
	'/campaigns',
	'/comparison-types',
	'/comparison-data-sources',
	'/comparison-ingestion-jobs',
	'/comparison-profiles',
	'/comparisons',
	'/content',
	'/content/videos',
	'/content/video-imports',
	'/content/video-claims',
	'/content-types',
	'/coupons',
	'/custom-fields',
	'/custom-fields/new',
	'/data-exports',
	'/deletions',
	'/discount-codes',
	'/docs',
	'/email-templates',
	'/forms',
	'/gamification',
	'/gdpr',
	'/gdpr/audit-logs',
	'/gdpr/consents',
	'/geo',
	'/i18n',
	'/join-requests',
	'/jobs',
	'/jsonld-templates',
	'/learning',
	'/media',
	'/menus',
	'/mentorship',
	'/moderation',
	'/moderation/alerts',
	'/moderation/appeals',
	'/moderation/content',
	'/moderation/reports',
	'/newsletter',
	'/notification-types',
	'/onboarding/templates',
	'/organisations',
	'/page-builder',
	'/page-builder/pages',
	'/page-builder/themes',
	'/page-templates',
	'/personas',
	'/persona-voices',
	'/plans',
	'/rag',
	'/reactions',
	'/redirects',
	'/referral',
	'/remittance',
	'/reports',
	'/reviews',
	'/search',
	'/settings',
	'/sidebars',
	'/space-types',
	'/spaces',
	'/spaces/new',
	'/spaces/presets',
	'/spam',
	'/studio',
	'/tags',
	'/ticket-types',
	'/tickets',
	'/travel',
	'/universities',
	'/universities/course-catalog',
	'/universities/course-offerings',
	'/university',
	'/university/categories',
	'/university/instructors',
	'/university/payouts',
	'/university/revenue-splits',
	'/university/tracks',
	'/users',
	'/users/groups',
	'/users/roles',
	'/wallet',
	'/wallet/credit-packs',
	'/wallet/purchases',
	'/webhooks',
	'/widgets',
];

// Control panel (apps/control) — port 3002
const controlRoutes: string[] = [
	'/',
	'/ai-budget',
	'/api-keys',
	'/audit',
	'/backup-configs',
	'/cache',
	'/cron',
	'/currencies',
	'/db',
	'/deployments',
	'/dlq',
	'/extensions',
	'/oauth-clients',
	'/permission-nodes',
	'/queue',
	'/security',
	'/settings',
	'/webhooks',
];

/** All known JapaConnect route paths, sorted alphabetically. */
export const allRoutes: string[] = [
	...new Set([
		...mainRoutes,
		...docsRoutes,
		...learnRoutes,
		...adminRoutes,
		...controlRoutes,
	]),
].sort();

/**
 * Returns route suggestions matching the given query (case-insensitive).
 * Excludes any URLs already in the `exclude` set.
 */
export function suggestRoutes(query: string, exclude: Set<string> = new Set()): string[] {
	const q = query.toLowerCase().trim();
	const base = allRoutes.filter((r) => !exclude.has(r));
	if (!q) return base.slice(0, 20);
	return base
		.filter((r) => r.toLowerCase().includes(q))
		.slice(0, 20);
}
