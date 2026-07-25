<script lang="ts">
	import { t } from '$lib/i18n';

	type FocusedField = 'space' | 'tags' | 'series' | 'body' | null;

	interface Props {
		focusedField: FocusedField;
	}

	let { focusedField }: Props = $props();

	const helpKey = $derived(
		focusedField ? `create.guide.${focusedField}` : 'create.guide.default'
	);
</script>

<div class="flex flex-col gap-2 p-3 text-sm text-[var(--color-text)]">
	<h3 class="font-semibold">{t('create.guide.title')}</h3>
	<p class="text-[var(--color-muted)]">{t(helpKey)}</p>
</div>
