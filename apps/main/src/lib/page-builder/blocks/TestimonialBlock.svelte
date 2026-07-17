<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	interface Props {
		quote?: string;
		author?: string;
		role?: string;
		avatar_url?: string;
		readOnly?: boolean;
	}

	let {
		quote = '',
		author = '',
		role = '',
		avatar_url = '',
		readOnly = false,
	}: Props = $props();
	let wrapperClass = $derived(
		`block-testimonial${!readOnly ? ' ring-2 ring-blue-400/40 hover:ring-blue-400/60' : ''}`
	);
</script>

<div class={wrapperClass}>
	{#if !quote && !author}
		<div class="flex h-32 items-center justify-center rounded-lg border-2 border-dashed border-gray-300 bg-gray-50 text-sm text-gray-400 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-500">
			Testimonial — add a quote and author
		</div>
	{:else}
		<blockquote class="relative rounded-xl bg-gray-50 px-8 py-10 dark:bg-gray-800/50">
			<svg
				class="absolute left-6 top-6 h-8 w-8 text-blue-200 dark:text-blue-900"
				fill="currentColor"
				viewBox="0 0 32 32"
				aria-hidden="true"
			>
				<path d="M9.352 4C4.456 7.456 1 13.12 1 19.36c0 5.088 3.072 8.064 6.624 8.064 3.36 0 5.856-2.688 5.856-5.856 0-3.168-2.208-5.472-5.088-5.472-.576 0-1.344.096-1.536.192.48-3.264 3.552-7.104 6.624-9.024L9.352 4zm16.512 0c-4.8 3.456-8.256 9.12-8.256 15.36 0 5.088 3.072 8.064 6.624 8.064 3.264 0 5.856-2.688 5.856-5.856 0-3.168-2.304-5.472-5.184-5.472-.576 0-1.248.096-1.44.192.48-3.264 3.456-7.104 6.528-9.024L25.864 4z" />
			</svg>

			<div class="relative z-10">
				{#if quote}
					<p class="mb-6 text-lg leading-relaxed text-gray-700 italic dark:text-gray-200 md:text-xl">
						"{quote}"
					</p>
				{/if}

				<div class="flex items-center gap-4">
					{#if avatar_url}
						<img
							src={avatar_url}
							alt={author || 'Author'}
							class="h-12 w-12 rounded-full object-cover"
						/>
					{:else}
						<div class="flex h-12 w-12 items-center justify-center rounded-full bg-blue-100 text-lg font-semibold text-blue-600 dark:bg-blue-900 dark:text-blue-300">
							{author ? author.charAt(0).toUpperCase() : '?'}
						</div>
					{/if}

					<div>
						{#if author}
							<cite class="block text-base font-semibold not-italic text-gray-900 dark:text-white">
								{author}
							</cite>
						{/if}
						{#if role}
							<span class="text-sm text-gray-500 dark:text-gray-400">
								{role}
							</span>
						{/if}
					</div>
				</div>
			</div>
		</blockquote>
	{/if}
</div>
