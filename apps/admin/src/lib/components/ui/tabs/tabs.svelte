<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { page } from '$app/state';
	import { untrack } from 'svelte';
	import { syncTabToUrl } from '$lib/utils/url-sync.svelte';
  import { Tabs } from "bits-ui";
  import { clsx, type ClassValue } from "clsx";
  
  let {
    tabs,
    value = $bindable(),
    onValueChange,
    class: className,
    tabParam,
    tabPreserve,
    children
  }: {
    tabs: Array<{
      value: string;
      label: string;
      icon?: any;
      count?: number;
    }>;
    value?: string;
    onValueChange?: (value: string) => void;
    class?: ClassValue;
    /**
     * When set (e.g. 'tab'), the active tab is two-way synced to the URL as
     * ?{tabParam}={value} — deep-linkable, shareable, survives reload and
     * back/forward navigation.
     */
    tabParam?: string;
    /**
     * With tabParam: when provided (including an empty array), every query
     * param except the tab param and the listed keys is dropped on tab
     * switch — keeps stale page/q/sort/filter params from leaking across
     * per-tab lists. List page-level params to keep (e.g. ['from','to']).
     */
    tabPreserve?: string[];
    children?: any;
  } = $props();
  
  // Set initial value if not provided — when tabParam is set, prefer the
  // URL param if it matches a known tab (deep-link / reload restore). Runs
  // again when `tabs` populates (async-derived tab lists).
  $effect(() => {
    if (tabs.length === 0) return;
    if (tabParam) {
      const raw = page.url.searchParams.get(tabParam) ?? '';
      if (raw && tabs.some((t) => t.value === raw)) {
        if (untrack(() => value) !== raw) value = raw;
        return;
      }
    }
    if (!value || !tabs.some((t) => t.value === value)) {
      value = tabs[0].value;
    }
  });

  // tabParam: state → URL. Only writes when `value` actually changes (never
  // on mount) so a deep-linked param survives async tab resolution — e.g.
  // permission-gated tabs that only enter `tabs` once permissions load.
  let prevSynced: string | undefined;
  $effect(() => {
    if (!tabParam || value === undefined) return;
    if (prevSynced === undefined) {
      // First run: prime the tracker without writing so a deep-linked
      // param is never clobbered before async tabs resolve.
      prevSynced = value;
      return;
    }
    if (value !== prevSynced) {
      prevSynced = value;
      if ((page.url.searchParams.get(tabParam) ?? '') !== value) {
        syncTabToUrl(value, { param: tabParam, preserve: tabPreserve });
      }
    }
  });
</script>

<Tabs.Root bind:value {onValueChange} class={clsx("w-full", className)}>
  <Tabs.List class="inline-flex h-11 items-center justify-center rounded-lg p-1" style="background-color: var(--color-surface); color: var(--color-muted)">
    {#each tabs as tab (tab)}
      <Tabs.Trigger
        value={tab.value}
        class="inline-flex items-center justify-center whitespace-nowrap rounded-md px-3 py-1.5 text-sm font-medium transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50 data-[state=active]:shadow-sm"
        style="--tw-ring-offset-color: var(--color-background); color: {value === tab.value ? 'var(--color-foreground)' : 'var(--color-muted)'}; background-color: {value === tab.value ? 'var(--color-surface)' : 'transparent'}"
      >
        {#if tab.icon}
          {@const Icon = tab.icon}
          <Icon class="me-2 h-4 w-4" />
        {/if}
        {tab.label}
        {#if tab.count !== undefined}
          <span class="ms-2 rounded-full px-2 py-0.5 text-xs font-medium" style="background-color: var(--color-surface); color: var(--color-foreground-alt)">
            {tab.count}
          </span>
        {/if}
      </Tabs.Trigger>
    {/each}
  </Tabs.List>
  
  {@render children?.()}
</Tabs.Root>
