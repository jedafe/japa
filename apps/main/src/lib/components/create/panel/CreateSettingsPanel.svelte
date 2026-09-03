<script lang="ts">
	import { t } from '$lib/i18n';
	import type { Visibility } from '$lib/types';
	import SelectField from '$lib/components/ui/SelectField.svelte';

	interface Props {
		slug: string;
		visibility: Visibility;
		canonicalUrl: string;
		publishAt: string | null;
		visibilityOptions?: { value: Visibility; label: string }[];
		/** Slug change callback. */
		onslug: (v: string) => void;
		onvisibility: (v: Visibility) => void;
		oncanonical: (v: string) => void;
		onpublishat: (v: string | null) => void;
		onedit: () => void;
	}

	let {
		slug,
		visibility,
		canonicalUrl,
		publishAt,
		visibilityOptions,
		onslug,
		onvisibility,
		oncanonical,
		onpublishat,
		onedit
	}: Props = $props();

	let resolvedVisibilityOptions = $derived(visibilityOptions ?? []);
</script>

<div class="settings-panel">
	<!-- Slug -->
	<div class="field-row">
		<label class="field-label" for="settings-slug">{t('create.label.slug')}</label>
		<input
			id="settings-slug"
			type="text"
			class="field-input"
			placeholder={t('create.placeholder.slug')}
			value={slug}
			oninput={(e) => {
					onslug(e.currentTarget.value);
				onedit();
			}}
		/>
	</div>

	<!-- Visibility -->
	<div class="field-row">
		<label class="field-label" for="settings-visibility">{t('create.label.visibility')}</label>
		<SelectField
			id="settings-visibility"
			value={visibility}
			options={resolvedVisibilityOptions}
			onValueChange={(v) => {
				onvisibility(v as Visibility);
				onedit();
			}}
		/>
	</div>

	<!-- Canonical URL -->
	<div class="field-row">
		<label class="field-label" for="settings-canonical">{t('create.label.canonical_url')}</label>
		<input
			id="settings-canonical"
			type="url"
			class="field-input"
			placeholder={t('create.label.canonical_url')}
			value={canonicalUrl}
			oninput={(e) => {
				oncanonical(e.currentTarget.value);
				onedit();
			}}
		/>
	</div>

	<!-- Publish at -->
	<div class="field-row">
		<label class="field-label" for="settings-publishat">{t('create.label.publish_at')}</label>
		<input
			id="settings-publishat"
			type="datetime-local"
			class="field-input"
			placeholder={t('create.label.publish_at')}
			value={publishAt ?? ''}
			onchange={(e) => {
				onpublishat(e.currentTarget.value || null);
				onedit();
			}}
		/>
	</div>
</div>

<style>
	.settings-panel {
		display: flex;
		flex-direction: column;
		gap: 12px;
		padding: 0.5rem;
	}

	.field-row {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.field-label {
		font-size: 0.75rem;
		font-weight: 500;
		color: var(--color-muted);
	}

	.field-input {
		width: 100%;
		padding: 10px 14px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 1rem;
		outline: none;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.field-input::placeholder {
		color: var(--color-muted);
	}
	.field-input:focus {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 15%, transparent);
	}
</style>
