<script lang="ts">
	import { t } from '#lib/i18n/index.js';
	import type { Visibility } from '#lib/types/index.js';
	import SelectField from '#lib/components/ui/SelectField.svelte';

	interface Props {
		slug: string;
		visibility: Visibility;
		canonicalUrl: string;
		publishAt: string | null;
		visibilityOptions?: { value: Visibility; label: string }[];
		/** §126 — member's per-post "don't federate" flag; the row renders
		 *  only when a federation feature is live (federateVisible). */
		federateOptOut?: boolean;
		federateVisible?: boolean;
		/** Slug change callback. */
		onslug: (v: string) => void;
		onvisibility: (v: Visibility) => void;
		oncanonical: (v: string) => void;
		onpublishat: (v: string | null) => void;
		onfederateoptout?: (v: boolean) => void;
		onedit: () => void;
	}

	let {
		slug,
		visibility,
		canonicalUrl,
		publishAt,
		visibilityOptions,
		federateOptOut = false,
		federateVisible = false,
		onslug,
		onvisibility,
		oncanonical,
		onpublishat,
		onfederateoptout,
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

	<!-- §126 — fediverse opt-out: keeps the post public on JapaConnect
	     while withholding it from remote servers (both AP + ATProto
	     projections honor the flag). -->
	{#if federateVisible}
		<div class="field-row">
			<label
				class="checkbox-label"
				for="settings-federate-opt-out"
				title={t('create.federate_opt_out_hint')}
			>
				<input
					id="settings-federate-opt-out"
					type="checkbox"
					checked={federateOptOut}
					onchange={(e) => {
						onfederateoptout?.(e.currentTarget.checked);
						onedit();
					}}
				/>
				{t('create.federate_opt_out')}
			</label>
		</div>
	{/if}
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

	.checkbox-label {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 0.875rem;
		color: var(--color-text);
		cursor: pointer;
	}
	.checkbox-label input {
		width: 1rem;
		height: 1rem;
		accent-color: var(--color-primary);
	}

	.field-input {
		width: 100%;
		height: 2.375rem;
		padding: 0 14px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.875rem;
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
