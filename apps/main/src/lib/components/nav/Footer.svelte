<script lang="ts">
	const currentYear = new Date().getFullYear();
</script>

<footer
	class="border-t py-8"
	style="background-color: var(--color-background); border-color: var(--color-border)"
>
	<div class="mx-auto max-w-6xl px-4">
		<div class="flex flex-col items-center justify-between gap-4 sm:flex-row">
			<div class="flex items-center gap-6 text-sm" style="color: var(--color-muted)">
				<a href="/about" class="hover:underline">About</a>
				<a href="/terms" class="hover:underline">Terms</a>
				<a href="/privacy" class="hover:underline">Privacy</a>
				<a href="/contact" class="hover:underline">Contact</a>
			</div>
			<p class="text-sm" style="color: var(--color-muted)">
				&copy; {currentYear} JapaConnect. All rights reserved.
			</p>
		</div>
	</div>
</footer>
