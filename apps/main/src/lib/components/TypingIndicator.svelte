<script lang="ts">
	let { username = 'Someone' }: { username?: string } = $props();
</script>

<div class="flex items-center gap-2 px-1 py-1" aria-label="{username} is typing">
	<span class="text-xs" style="color: var(--color-muted)">{username} is typing</span>
	<span class="typing-dots inline-flex gap-0.5">
		<span class="h-1 w-1 rounded-full" style="background-color: var(--color-muted)"></span>
		<span class="h-1 w-1 rounded-full" style="background-color: var(--color-muted)"></span>
		<span class="h-1 w-1 rounded-full" style="background-color: var(--color-muted)"></span>
	</span>
</div>

<style>
	.typing-dots span {
		animation: typing-bounce 1.4s ease-in-out infinite;
	}
	.typing-dots span:nth-child(1) {
		animation-delay: 0s;
	}
	.typing-dots span:nth-child(2) {
		animation-delay: 0.2s;
	}
	.typing-dots span:nth-child(3) {
		animation-delay: 0.4s;
	}

	@keyframes typing-bounce {
		0%, 60%, 100% {
			opacity: 0.3;
			transform: translateY(0);
		}
		30% {
			opacity: 1;
			transform: translateY(-3px);
		}
	}
</style>
