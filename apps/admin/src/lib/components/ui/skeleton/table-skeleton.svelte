<script lang="ts">
  import { clsx, type ClassValue } from "clsx";
  import Skeleton from "./skeleton.svelte";
  
  let {
    rows = 5,
    columns = 4,
    class: className
  }: {
    rows?: number;
    columns?: number;
    class?: ClassValue;
  } = $props();
</script>

<div class={clsx("space-y-3", className)}>
  {#each Array(rows) as _ (_)}
    <div class="flex items-center gap-4">
      {#each Array(columns) as _, i (i)}
        <Skeleton 
          class={clsx(
            "h-8",
            i === 0 ? "w-1/4" : i === columns - 1 ? "w-1/6" : "w-1/5"
          )} 
        />
      {/each}
    </div>
  {/each}
</div>
