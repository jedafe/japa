<script lang="ts">
	// "How it works" button + guide slide-over for a tool page. The drawer
	// content lives in the public `tools.guides` system setting — a JSON map
	// keyed by tool slug ({slug: {title, body (md), ctas}}), editable inline
	// by admins via PUT /admin/settings/general → {tools_guides}. Same
	// pattern as the per-directory `*.guide` panels (sources.guide, …).
	import { page } from '$app/state';
	import { CirclePlay } from 'lucide-svelte';
	import { t } from '#lib/i18n/index.js';
	import { api } from '#lib/api/client.js';
	import { GuideSlideOver, type GuideContent } from '#lib/components/ui/index.js';

	interface Props {
		/** Tool slug (e.g. 'visa-checklist') — key inside tools.guides. */
		slug: string;
		/** Extra classes on the trigger button (default: japa-stage ghost look). */
		class?: string;
		/** CirclePlay icon size in px. */
		iconSize?: number;
		/** Extra classes on the label span (e.g. 'max-sm:hidden'). */
		labelClass?: string;
	}

	let { slug, class: className = '', iconSize = 17, labelClass = '' }: Props = $props();

	let open = $state(false);
	let guideOverride = $state<GuideContent | null>(null);

	const siteSettings = $derived((page.data.siteSettings ?? {}) as Record<string, unknown>);
	const guidesMap = $derived.by((): Record<string, Partial<GuideContent>> => {
		const raw = siteSettings['tools.guides'];
		if (raw && typeof raw === 'object' && !Array.isArray(raw)) {
			return raw as Record<string, Partial<GuideContent>>;
		}
		return {};
	});
	const storedGuide = $derived(guidesMap[slug] ?? {});
	// Per-tool default body — its own i18n key (tools.guide_<slug>), with the
	// generic tools.guide_body as the last resort.
	const defaultBody = $derived(
		t(`tools.guide_${slug.replaceAll('-', '_')}`, t('tools.guide_body', ''))
	);
	const effectiveGuide = $derived<GuideContent>({
		title: guideOverride?.title ?? storedGuide.title ?? t('common.how_it_works', 'How it works'),
		body: guideOverride?.body ?? storedGuide.body ?? defaultBody,
		ctas: guideOverride?.ctas ??
			storedGuide.ctas ?? [{ title: t('guide_panel.read_full_guide', 'Read the full guide'), url: '/guides' }]
	});

	// Staff gate for the inline pencil — same check the directory pages run
	// server-side: super_admin group bypasses (it isn't in /me/permissions
	// output), everyone else needs an admin.settings.* node or the '*'
	// wildcard. The session user (root layout) already carries both lists.
	const canManage = $derived(
		(page.data.user?.groups ?? []).some((g: string) => g === 'Super Admin' || g === 'super_admin') ||
		(page.data.user?.permissions ?? []).some(
			(p: string) => p === '*' || p.startsWith('admin.settings')
		)
	);

	async function saveGuide(v: GuideContent): Promise<string | null> {
		const res = await api.put('/admin/settings/general', {
			tools_guides: { ...guidesMap, [slug]: v }
		});
		if (res.error) return res.error.message || t('common.error', 'Something went wrong');
		guideOverride = v;
		return null;
	}
</script>

<button
	type="button"
	onclick={() => (open = true)}
	class={className ||
		'inline-flex items-center gap-1.5 text-sm font-medium text-[var(--color-text)] transition-opacity hover:opacity-75'}
>
	<CirclePlay size={iconSize} class="shrink-0 text-[var(--color-primary-text)]" />
	<span class={labelClass}>{t('common.how_it_works', 'How it works')}</span>
</button>

<GuideSlideOver
	bind:open
	title={effectiveGuide.title}
	body={effectiveGuide.body}
	ctas={effectiveGuide.ctas}
	{canManage}
	onSave={saveGuide}
/>
