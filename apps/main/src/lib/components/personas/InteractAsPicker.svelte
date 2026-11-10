<script lang="ts">
	/**
	 * InteractAsPicker — compact "Interact as" chip for delegated authorship
	 * (spec §1.8). Selecting a persona makes subsequent comments, replies,
	 * votes, reactions, and follows record the persona as the actor; the
	 * operator's session is unchanged. Hidden when the operator has no
	 * drivable personas.
	 */
	import { Bot } from 'lucide-svelte';
	import { t } from '#lib/i18n/index.js';
	import { actAsPersona } from '#lib/stores/actAsPersona.svelte.js';

	const personas = $derived(actAsPersona.items);
	const selected = $derived(actAsPersona.selected);

	function onChange(e: Event) {
		const id = (e.currentTarget as HTMLSelectElement).value;
		actAsPersona.select(personas.find((p) => p.user_id === id) ?? null);
	}
</script>

{#if personas.length > 0}
	<label
		class="inline-flex items-center gap-1.5 rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-1 text-xs text-[var(--color-muted)]"
	>
		<Bot size={12} aria-hidden="true" />
		<span>{t('personas.interact_as', 'Interact as')}</span>
		<select
			value={selected?.user_id ?? ''}
			onchange={onChange}
			aria-label={t('personas.interact_as', 'Interact as')}
			class="max-w-44 cursor-pointer bg-transparent text-xs font-medium text-[var(--color-text)] outline-none"
		>
			<option value="">{t('personas.you', 'You')}</option>
			{#each personas as persona (persona.user_id)}
				<option value={persona.user_id}>
					{persona.name}{persona.geo_space_name ? ` · ${persona.geo_space_name}` : ''}
				</option>
			{/each}
		</select>
	</label>
{/if}
