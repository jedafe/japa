<script lang="ts">
	import { t } from '$lib/i18n';
	import { X } from 'lucide-svelte';

	interface Props {
		tags: string[];
		onTags: (next: string[]) => void;
	}

	let { tags, onTags }: Props = $props();
	let draft = $state('');

	function addTag() {
		const v = draft.trim().toLowerCase().replace(/\s+/g, '-');
		if (v && !tags.includes(v)) onTags([...tags, v]);
		draft = '';
	}

	function removeTag(tag: string) {
		onTags(tags.filter((t) => t !== tag));
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' || e.key === ',') {
			e.preventDefault();
			addTag();
		} else if (e.key === 'Backspace' && draft === '' && tags.length > 0) {
			onTags(tags.slice(0, -1));
		}
	}
</script>

<div class="flex flex-col gap-1">
	<label class="text-sm font-medium text-[var(--color-text)]">{t('create.label.tags')}</label>
	<div
		class="flex flex-wrap items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1.5"
	>
		{#each tags as tag (tag)}
			<span
				class="flex items-center gap-1 rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs text-[var(--color-text)]"
			>
				#{tag}
				<button type="button" onclick={() => removeTag(tag)} aria-label={t('create.action.remove_tag')}>
					<X size={12} />
				</button>
			</span>
		{/each}
		<input
			type="text"
			class="flex-1 min-w-[120px] bg-transparent text-sm outline-none"
			placeholder={t('create.placeholder.tag')}
			bind:value={draft}
			onkeydown={onKeydown}
			onblur={addTag}
		/>
	</div>
</div>
