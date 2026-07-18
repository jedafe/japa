<script lang="ts">
  import { clsx, type ClassValue } from "clsx";
  import type { Component } from "svelte";

  type Action = {
    label: string;
    icon?: Component;
    onclick: () => void;
    variant?: "default" | "destructive" | "ghost";
    class?: ClassValue;
  };

  let {
    actions,
    class: className = undefined
  }: {
    actions: Action[];
    class?: ClassValue;
  } = $props();

  const variantClasses: Record<string, string> = {
    default: "text-foreground hover:bg-surface",
    destructive: "text-danger hover:bg-danger/15",
    ghost: "text-foreground-alt hover:bg-surface"
  };
</script>

<div class={clsx("flex items-center gap-1", className)}>
  {#each actions as action}
    <button
      type="button"
      class={clsx(
        "inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium transition-colors",
        variantClasses[action.variant ?? "default"],
        action.class
      )}
      onclick={(e) => {
        e.stopPropagation();
        action.onclick();
      }}
    >
      {#if action.icon}
        <action.icon class="h-4 w-4" />
      {/if}
      {action.label}
    </button>
  {/each}
</div>
