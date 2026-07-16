<script lang="ts">
  import { clsx, type ClassValue } from "clsx";
  import Skeleton from "./skeleton.svelte";
  
  let {
    class: className
  }: {
    class?: ClassValue;
  } = $props();
</script>

<div class={clsx("rounded-xl border p-6 space-y-4", className)} style="border-color: var(--color-border); background-color: var(--color-surface)">
  <div class="flex items-center justify-between">
    <Skeleton class="h-5 w-1/3" />
    <Skeleton variant="circular" class="h-10 w-10" />
  </div>
  <Skeleton class="h-8 w-1/2" />
  <Skeleton class="h-4 w-2/3" />
</div>
