<script lang="ts">
	import { t } from '$lib/i18n';
	import type { BlogSeries, CustomFieldGroup, Visibility } from '$lib/types';
	import SeriesSelector from './SeriesSelector.svelte';
	import CustomFieldGroupCmp from './CustomFieldGroup.svelte';

	interface Props {
		visibility: Visibility;
		canonicalUrl: string;
		publishAt: string | null;
		fieldValues: Record<string, unknown>;
		fieldGroups: CustomFieldGroup[];
		series: BlogSeries[];
		seriesId: string | null;
		onvisibility: (v: Visibility) => void;
		oncanonical: (v: string) => void;
		onpublishat: (v: string | null) => void;
		onfieldvalues: (v: Record<string, unknown>) => void;
		onseries: (id: string | null) => void;
		onedit: () => void;
	}

	let {
		visibility,
		canonicalUrl,
		publishAt,
		fieldValues,
		fieldGroups,
		series,
		seriesId,
		onvisibility,
		oncanonical,
		onpublishat,
		onfieldvalues,
		onseries,
		onedit
	}: Props = $props();
</script>

<div class="flex flex-col gap-4 p-2">
	<div class="flex flex-col gap-1">
		<label class="text-sm font-medium text-[var(--color-text)]">{t('create.label.visibility')}</label>
		<select
			class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm outline-none"
			value={visibility}
			onchange={(e) => onvisibility(e.currentTarget.value as Visibility)}
		>
			<option value="public">{t('create.settings.visibility.public')}</option>
			<option value="unlisted">{t('create.settings.visibility.unlisted')}</option>
			<option value="private">{t('create.settings.visibility.private')}</option>
		</select>
	</div>

	<div class="flex flex-col gap-1">
		<label class="text-sm font-medium text-[var(--color-text)]">{t('create.label.canonical_url')}</label>
		<input
			type="url"
			class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm outline-none"
			placeholder="https://…"
			value={canonicalUrl}
			oninput={(e) => {
				oncanonical(e.currentTarget.value);
				onedit();
			}}
		/>
	</div>

	<div class="flex flex-col gap-1">
		<label class="text-sm font-medium text-[var(--color-text)]">{t('create.label.publish_at')}</label>
		<input
			type="datetime-local"
			class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm outline-none"
			value={publishAt ?? ''}
			onchange={(e) => {
				onpublishat(e.currentTarget.value || null);
				onedit();
			}}
		/>
	</div>

	<SeriesSelector {series} {seriesId} onselect={(id) => { onseries(id); onedit(); }} />

	{#each fieldGroups as group (group.id)}
		<div class="flex flex-col gap-2">
			<h4 class="text-sm font-semibold text-[var(--color-text)]">{group.name}</h4>
			<CustomFieldGroupCmp
				fields={group.fields}
				values={fieldValues}
				onvalues={onfieldvalues}
				{onedit}
			/>
		</div>
	{/each}
</div>
