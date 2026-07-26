<script lang="ts">
	import { t } from '$lib/i18n';
	import { suggestSlug } from './slug';

	interface Props {
		title: string;
		slug: string;
		onTitle: (v: string) => void;
		onSlug: (v: string) => void;
	}

	let { title, slug, onTitle, onSlug }: Props = $props();

	let slugTouched = $state(false);

	function handleTitle(v: string) {
		onTitle(v);
		if (!slugTouched) onSlug(suggestSlug(v));
	}
</script>

<div class="title-field">
	<input
		type="text"
		class="title-input"
		placeholder={t('create.placeholder.title')}
		value={title}
		oninput={(e) => handleTitle(e.currentTarget.value)}
	/>
	<input
		type="text"
		class="slug-input"
		placeholder={t('create.placeholder.slug')}
		value={slug}
		oninput={(e) => {
			slugTouched = true;
			onSlug(suggestSlug(e.currentTarget.value));
		}}
	/>
</div>

<style>
	.title-field {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.title-input {
		width: 100%;
		padding: 10px 14px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 1.125rem;
		font-weight: 600;
		outline: none;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.title-input::placeholder {
		color: var(--color-muted);
		font-weight: 500;
	}
	.title-input:focus {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 15%, transparent);
	}

	.slug-input {
		width: 100%;
		padding: 10px 14px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text-subtle);
		font-size: 0.875rem;
		outline: none;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.slug-input::placeholder {
		color: var(--color-muted);
	}
	.slug-input:focus {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 15%, transparent);
	}
</style>
