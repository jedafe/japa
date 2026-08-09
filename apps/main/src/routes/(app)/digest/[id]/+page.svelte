<script lang="ts">
	// Ref: japaconnect-engineering/sections/84-community-digest-widget.md
	import DigestView from '../DigestView.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
</script>

<DigestView digest={data.digest} loadError={data.loadError} showBack showStatus />
