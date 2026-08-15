<script lang="ts">
	// ActivityKeyAutocomplete — a debounced autocomplete dropdown for selecting
	// an activity_key. Aggregates known activity keys from three sources:
	//
	//   1. `extraKeys` prop — activity keys already loaded by the parent page
	//      (from badge_definitions.trigger_activity_key and
	//      challenge_definitions.activity_key).
	//   2. `/admin/wallet/rules` — the §28 activity_point_rules registry
	//      (best-effort; may 403 if the admin lacks wallet.admin.rules.manage).
	//   3. Manual entry — the user can always type a custom key.
	//
	// Ref: japaconnect-engineering/sections/28-wallet-credits.md —
	// activity_rules registry (activity_key → point_value).
	// Ref: AGENTS.md — never hardcode activity keys; always source from API.
	import { apiFetch } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type ActivityRule = {
		id: string;
		activity_key: string;
		point_value: number;
		is_enabled: boolean | null;
		description: string | null;
	};

	let {
		value = $bindable(''),
		placeholder = 'post.create',
		id = 'activity-key-input',
		disabled = false,
		extraKeys = [] as string[]
	}: {
		value?: string;
		placeholder?: string;
		id?: string;
		disabled?: boolean;
		extraKeys?: string[];
	} = $props();

	let walletKeys = $state<string[]>([]);
	let query = $state('');
	let dropdownOpen = $state(false);
	let loading = $state(false);
	let activeIndex = $state(-1);
	let debounceTimer: ReturnType<typeof setTimeout> | undefined;
	let loaded = $state(false);

	// Fetch wallet activity rules once on mount (best-effort).
	$effect(() => {
		(async () => {
			loading = true;
			try {
				const result = await apiFetch<ActivityRule[] | { data: ActivityRule[] }>(
					'/admin/wallet/rules?page_size=200'
				);
				if (!result.error && result.data) {
					const data = result.data;
					const arr = Array.isArray(data) ? data : (data as { data: ActivityRule[] }).data ?? [];
					walletKeys = arr.map((r) => r.activity_key).filter(Boolean);
				}
			} catch {
				walletKeys = [];
			} finally {
				loading = false;
				loaded = true;
			}
		})();
	});

	// Aggregate all known activity keys: wallet rules + extra keys from parent
	// (badge trigger_activity_key + challenge activity_key). Deduplicated + sorted.
	let allKeys = $derived.by(() => {
		const set = new Set<string>(walletKeys);
		for (const k of extraKeys) {
			if (k) set.add(k);
		}
		return Array.from(set).sort();
	});

	// Debounced client-side filter against the cached activity-key list.
	let filtered = $derived.by(() => {
		if (!allKeys.length) return [];
		const q = query.trim().toLowerCase();
		if (!q) return allKeys.slice(0, 20);
		return allKeys
			.filter((k) => k.toLowerCase().includes(q))
			.slice(0, 20);
	});

	function handleInput(e: Event) {
		const target = e.target as HTMLInputElement;
		value = target.value;
		query = target.value;
		activeIndex = -1;
		if (debounceTimer) clearTimeout(debounceTimer);
		debounceTimer = setTimeout(() => {
			dropdownOpen = query.trim().length > 0 && allKeys.length > 0;
		}, 200);
	}

	// Clear any pending debounce timer when the component unmounts.
	$effect(() => {
		return () => {
			if (debounceTimer) clearTimeout(debounceTimer);
		};
	});

	function selectKey(key: string) {
		value = key;
		query = key;
		dropdownOpen = false;
		activeIndex = -1;
	}

	function handleKeydown(e: KeyboardEvent) {
		if (!dropdownOpen) {
			if (e.key === 'ArrowDown' && filtered.length > 0) {
				dropdownOpen = true;
				activeIndex = 0;
				e.preventDefault();
			}
			return;
		}
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			activeIndex = Math.min(activeIndex + 1, filtered.length - 1);
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			activeIndex = Math.max(activeIndex - 1, 0);
		} else if (e.key === 'Enter' && activeIndex >= 0 && filtered[activeIndex]) {
			e.preventDefault();
			selectKey(filtered[activeIndex]);
		} else if (e.key === 'Escape') {
			dropdownOpen = false;
			activeIndex = -1;
		}
	}

	function handleFocus() {
		if (query.trim() && allKeys.length > 0) {
			dropdownOpen = true;
		}
	}

	function handleBlur() {
		// Delay close so a click on a dropdown item registers first.
		setTimeout(() => {
			dropdownOpen = false;
		}, 200);
	}

	// Cleanup the debounce timer on destroy.
	$effect(() => {
		return () => {
			if (debounceTimer) clearTimeout(debounceTimer);
		};
	});
</script>

<div class="relative">
	<input
		{id}
		type="text"
		{placeholder}
		{value}
		{disabled}
		oninput={handleInput}
		onkeydown={handleKeydown}
		onfocus={handleFocus}
		onblur={handleBlur}
		role="combobox"
		aria-expanded={dropdownOpen}
		aria-controls={`${id}-list`}
		aria-autocomplete="list"
		class="w-full rounded-md border px-3 py-2 text-sm font-mono"
		style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
	/>

	{#if loading}
		<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('common.loading', 'Loading...')}</p>
	{/if}

	{#if dropdownOpen && filtered.length > 0}
		<ul
			id={`${id}-list`}
			role="listbox"
			class="absolute z-[60] mt-1 max-h-48 w-full overflow-auto rounded-md border shadow-lg"
			style="border-color: var(--color-border); background: var(--color-surface)"
		>
			{#each filtered as key, i (key)}
				<li role="option" aria-selected={i === activeIndex}>
					<button
						type="button"
						class="flex w-full items-center px-3 py-2 text-left text-sm font-mono transition-colors"
						style={i === activeIndex
							? 'background-color: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-foreground)'
							: 'color: var(--color-foreground)'}
						onmousedown={() => selectKey(key)}
						onmouseenter={() => (activeIndex = i)}
					>
						{key}
					</button>
				</li>
			{/each}
		</ul>
	{:else if dropdownOpen && query.trim() && !loading && loaded && filtered.length === 0}
		<p class="absolute z-[60] mt-1 rounded-md border px-3 py-2 text-xs shadow-lg" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-muted)">
			{t('gamification.no_activity_keys', 'No matching activity keys. Type a custom key.')}
		</p>
	{/if}
</div>
