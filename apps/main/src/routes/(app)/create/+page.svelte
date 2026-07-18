<script lang="ts">
	import { PenSquare } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';

	// Placeholder for the dev.to-style composer (Exception #2). The real /create
	// surface — cover/title/tags, required Space selector, TipTap body, and the
	// Guide | Settings tabbed right panel — is the next feature task.
</script>

<div class="mx-auto w-full max-w-2xl py-10">
	<EmptyState
		icon={PenSquare}
		title="The composer is coming soon"
		body="Creating posts, blogs and comparisons will live here."
		action={{ label: 'Back home', href: '/' }}
	/>
</div>
