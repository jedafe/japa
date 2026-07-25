<script lang="ts">
	import { t } from '$lib/i18n';
	import type { MeSpace } from '$lib/types';

	interface Props {
		spaces: MeSpace[];
		selectedId: string | null;
		onSelect: (id: string) => void;
	}

	let { spaces, selectedId, onSelect }: Props = $props();
</script>

<div class="flex flex-col gap-1">
	<label for="space-select" class="text-sm font-medium text-[var(--color-text)]">
		{t('create.label.space')}
	</label>
	<select
		id="space-select"
		class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
		value={selectedId ?? ''}
		onchange={(e) => onSelect(e.currentTarget.value)}
	>
		<option value="" disabled>{t('create.placeholder.space')}</option>
		{#each spaces as s (s.id)}
			<option value={s.id}>
				{s.is_primary ? '★ ' : ''}{s.name}
			</option>
		{/each}
	</select>
</div>
