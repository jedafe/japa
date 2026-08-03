<script lang="ts">
	import CreateShell from '$lib/components/create/CreateShell.svelte';
	import type { EditorMode } from '$lib/components/create/editors/PostEditor.svelte';
	import { PenSquare } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { t } from '$lib/i18n';

	let { data } = $props();

	// Resolve the admin-configured default editor mode from the public site
	// settings (appearance.default_editor). Falls back to 'block'.
	// Ref: conventions/system-settings-catalog.md (Appearance)
	const VALID_MODES = new Set<EditorMode>(['block', 'wysiwyg', 'markdown']);
	const defaultEditor: EditorMode = $derived(
		VALID_MODES.has(data.siteSettings?.['appearance.default_editor'] as EditorMode)
			? (data.siteSettings['appearance.default_editor'] as EditorMode)
			: 'block'
	);

	// Max tags per content item from the public site settings (content.max_tags).
	// Falls back to 5 when unset or invalid. The backend enforces the same limit.
	// Ref: conventions/system-settings-catalog.md (Content)
	const maxTags: number = $derived(
		Number.isFinite(Number(data.siteSettings?.['content.max_tags']))
			? Number(data.siteSettings['content.max_tags'])
			: 5
	);
</script>

{#if data.loadError}
	<div class="mx-auto w-full max-w-full py-10">
		<EmptyState
			icon={PenSquare}
			title={t('error.500_title')}
			body={t('error.500_body')}
			action={{ label: t('empty.action_home'), href: '/' }}
		/>
	</div>
{:else}
	<CreateShell
		types={data.types}
		spaces={data.spaces}
		fieldGroups={data.fieldGroups}
		series={data.series}
		organizations={data.organizations}
		draft={data.draft}
		user={data.user}
		{defaultEditor}
		{maxTags}
	/>
{/if}
