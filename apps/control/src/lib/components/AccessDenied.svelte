<script lang="ts">
	import { ShieldAlert } from 'lucide-svelte';
	import { t } from '$lib/stores/i18n.svelte';

	let { message = '' }: { message?: string } = $props();
</script>

<div class="flex flex-col items-center justify-center py-16 text-center">
	<div class="rounded-full bg-destructive/10 p-4">
		<ShieldAlert class="h-10 w-10 text-destructive" />
	</div>
	<h2 class="mt-4 text-lg font-semibold text-foreground">{t('access.title')}</h2>
	<p class="text-muted-foreground mt-2 max-w-sm text-sm">{message || t('access.denied_message')}</p>
	<a href="/" class="bg-foreground text-background mt-6 rounded-md px-4 py-2 text-sm font-medium">
		{t('access.back_to_dashboard')}
	</a>
</div>
