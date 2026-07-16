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
    default: "text-gray-700 hover:bg-gray-100",
    destructive: "text-danger-700 hover:bg-danger-50",
    ghost: "text-gray-600 hover:bg-gray-100"
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
