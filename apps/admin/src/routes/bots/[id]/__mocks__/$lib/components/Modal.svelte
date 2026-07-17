<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	let { open = $bindable(false), title = '', children, onConfirm, confirmLabel = 'Confirm' } = $props();
</script>

{#if open}
	<div class="modal-backdrop">
		<div class="modal">
			<h2>{title}</h2>
			{#if children}
				{@render children()}
			{/if}
			<div class="modal-actions">
				<button onclick={() => (open = false)}>{t('bots.id.__mocks__.$lib.components.Modal.svelte.cancel')}</button>
				{#if onConfirm}
					<button onclick={() => { onConfirm(); open = false; }}>{confirmLabel}</button>
				{/if}
			</div>
		</div>
	</div>
{/if}
