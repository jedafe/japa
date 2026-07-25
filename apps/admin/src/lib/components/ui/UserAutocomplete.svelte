<script lang="ts">
	import { apiFetch } from "$lib/api";
	import { onMount } from "svelte";

	let { value = $bindable(""), placeholder = "Username", onSelect = (v: string) => {} } = $props();

	let suggestions = $state<{ id: string; username: string }[]>([]);
	let showDropdown = $state(false);
	let loading = $state(false);
	let debounceTimer: ReturnType<typeof setTimeout> | null = null;

	async function searchUsers(query: string) {
		if (!query.trim() || query.length < 2) {
			suggestions = [];
			showDropdown = false;
			return;
		}
		loading = true;
		try {
			const result = await apiFetch<{ items: { id: string; username: string }[] }>(
				`/admin/users?page_size=10&search=${encodeURIComponent(query)}`
			);
			if (!result.error && result.data?.items) {
				suggestions = result.data.items;
				showDropdown = suggestions.length > 0;
			} else {
				suggestions = [];
				showDropdown = false;
			}
		} catch {
			suggestions = [];
			showDropdown = false;
		} finally {
			loading = false;
		}
	}

	function handleInput(e: Event) {
		const target = e.target as HTMLInputElement;
		value = target.value;
		if (debounceTimer) clearTimeout(debounceTimer);
		debounceTimer = setTimeout(() => searchUsers(value), 200);
	}

	function selectUser(username: string) {
		value = username;
		showDropdown = false;
		onSelect(username);
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === "Escape") {
			showDropdown = false;
		} else if (e.key === "Enter" && suggestions.length > 0) {
			e.preventDefault();
			selectUser(suggestions[0].username);
		}
	}

	onMount(() => {
		return () => {
			if (debounceTimer) clearTimeout(debounceTimer);
		};
	});
</script>

<div class="relative">
	<input
		type="text"
		{value}
		{placeholder}
		oninput={handleInput}
		onkeydown={handleKeydown}
		onfocus={() => { if (suggestions.length > 0) showDropdown = true; }}
		onblur={() => { setTimeout(() => showDropdown = false, 200); }}
		class="w-full rounded-lg border p-2 text-sm"
		style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
	/>
	{#if showDropdown && suggestions.length > 0}
		<ul class="absolute z-[60] mt-1 max-h-48 w-full overflow-auto rounded-lg border shadow-lg" style="border-color: var(--color-border); background: var(--color-surface)">
			{#each suggestions as user}
				<li>
					<button
						type="button"
						class="w-full px-3 py-2 text-left text-sm hover:bg-black/5 dark:hover:bg-white/10"
						style="color: var(--color-foreground)"
						onmousedown={() => selectUser(user.username)}
					>
						{user.username}
					</button>
				</li>
			{/each}
		</ul>
	{/if}
</div>
