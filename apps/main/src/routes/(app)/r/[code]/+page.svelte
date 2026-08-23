<!--
	Affiliate short-link redirect at /r/[code].
	The +page.server.ts load performs the redirect, so this component is only
	rendered if something goes wrong. Show a minimal loading state.

	Ref: japaconnect-engineering/sections/56-user-affiliate-program.md
-->
<script lang="ts">
	import { Loader2 } from 'lucide-svelte';
</script>

<div class="grid h-96 place-items-center">
	<Loader2 size={32} class="animate-spin text-[var(--color-muted)]" />
</div>
