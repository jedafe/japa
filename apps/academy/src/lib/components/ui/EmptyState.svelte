<script lang="ts">
	// Thin wrapper over @japaconnect/ui — preserves the localized default title.
	import { EmptyState } from '@japaconnect/ui';
	import { t } from '#lib/i18n/index.js';
	import type { ComponentProps } from 'svelte';

	let { title, ...rest }: ComponentProps<typeof EmptyState> = $props();
</script>

<EmptyState title={title ?? t('common.empty')} {...rest} />
