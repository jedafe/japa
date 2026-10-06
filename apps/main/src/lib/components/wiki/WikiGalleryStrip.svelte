<script lang="ts">
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md
	// (Wiki-Only Space Types & Attractions) — the full-width gallery strip
	// under the destination hero: every resolved image the entity's media
	// fields carry (the attraction overview group's `gallery` image_multi)
	// as linked thumbnails; v1 links the full media URL, no lightbox.
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { getEntityFields } from '$lib/api/wiki';
	import {
		isMediaFieldType,
		mediaFieldItems,
		type MediaFieldItem
	} from '$lib/utils/wikiFieldValue';

	let { entityId }: { entityId: string } = $props();

	// The entity-fields endpoint sits behind wiki.structured_edits — the
	// strip silently skips when the flag is off.
	const flagOn = $derived(page.data?.features?.['wiki.structured_edits'] === true);

	let media = $state<MediaFieldItem[]>([]);

	$effect(() => {
		if (!flagOn || !entityId) {
			media = [];
			return;
		}
		let cancelled = false;
		getEntityFields('space', entityId)
			.then((data) => {
				if (cancelled || !data) return;
				const out: MediaFieldItem[] = [];
				for (const g of data.groups) {
					for (const f of g.fields) {
						if (!f.visible || !isMediaFieldType(f.field_type)) continue;
						out.push(...mediaFieldItems(f.value));
					}
				}
				media = out;
			})
			.catch(() => {
				if (!cancelled) media = [];
			});
		return () => {
			cancelled = true;
		};
	});
</script>

{#if media.length > 0}
	<div
		class="mt-4 flex gap-2 overflow-x-auto pb-1"
		role="group"
		aria-label={t('wiki.gallery', 'Gallery')}
	>
		{#each media as m (m.id || m.url)}
			<a
				href={m.url}
				target="_blank"
				rel="noopener noreferrer"
				class="shrink-0 transition-opacity hover:opacity-90"
			>
				<img
					src={m.url}
					alt={m.alt ?? ''}
					loading="lazy"
					class="h-28 w-44 rounded-[var(--radius)] border border-[var(--color-border)] object-cover"
				/>
			</a>
		{/each}
	</div>
{/if}
