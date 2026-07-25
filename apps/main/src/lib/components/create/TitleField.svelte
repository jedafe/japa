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

<div class="flex flex-col gap-2">
	<input
		type="text"
		class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-lg font-semibold text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
		placeholder={t('create.placeholder.title')}
		value={title}
		oninput={(e) => handleTitle(e.currentTarget.value)}
	/>
	<div class="flex items-center gap-2 text-sm text-[var(--color-muted)]">
		<span>/</span>
		<input
			type="text"
			class="flex-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm outline-none focus:border-[var(--color-primary)]"
			placeholder={t('create.placeholder.slug')}
			value={slug}
			oninput={(e) => {
				slugTouched = true;
				onSlug(suggestSlug(e.currentTarget.value));
			}}
		/>
	</div>
</div>
