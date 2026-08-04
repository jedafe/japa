<script lang="ts">
  import type { CountdownProps } from '../types';
  import { onMount } from 'svelte';

  interface Props {
    props: CountdownProps;
  }
  let { props }: Props = $props();

  let timeLeft = $state({ days: 0, hours: 0, minutes: 0, seconds: 0 });
  let expired = $state(false);

  onMount(() => {
    const target = new Date(props.target_date).getTime();
    if (isNaN(target)) { expired = true; return; }

    const update = () => {
      const now = Date.now();
      const diff = target - now;
      if (diff <= 0) {
        expired = true;
        timeLeft = { days: 0, hours: 0, minutes: 0, seconds: 0 };
        return false;
      }
      timeLeft = {
        days: Math.floor(diff / 86400000),
        hours: Math.floor((diff % 86400000) / 3600000),
        minutes: Math.floor((diff % 3600000) / 60000),
        seconds: Math.floor((diff % 60000) / 1000),
      };
      return true;
    };

    if (!update()) return;
    const interval = setInterval(() => { if (!update()) clearInterval(interval); }, 1000);
    return () => clearInterval(interval);
  });

  const units = $derived([
    { key: 'days', label: 'Days', value: timeLeft.days, show: props.show_days },
    { key: 'hours', label: 'Hours', value: timeLeft.hours, show: props.show_hours },
    { key: 'minutes', label: 'Min', value: timeLeft.minutes, show: props.show_minutes },
    { key: 'seconds', label: 'Sec', value: timeLeft.seconds, show: props.show_seconds },
  ].filter(u => u.show));

  const layoutClass = $derived(
    props.layout === 'inline' ? 'flex flex-row gap-4' :
    props.layout === 'stacked' ? 'flex flex-col gap-2' :
    'grid grid-cols-4 gap-4'
  );
</script>

{#if expired && props.on_expire === 'hide'}
  <!-- Hidden on expire -->
{:else if expired && props.on_expire === 'show_message'}
  <section class="px-6 py-16">
    <div class="mx-auto max-w-2xl text-center">
      <p class="text-lg text-[var(--color-text-subtle)]">{props.expired_message ?? 'This event has passed.'}</p>
    </div>
  </section>
{:else}
  <section class="px-6 py-16">
    <div class="mx-auto max-w-2xl text-center">
      {#if props.label}
        <p class="mb-4 text-sm text-[var(--color-muted)]">{props.label}</p>
      {/if}
      <div class={layoutClass}>
        {#each units as unit}
          <div class="flex flex-col items-center rounded-[var(--radius)] bg-[var(--color-surface)] border border-[var(--color-border)] p-4">
            <span class="text-3xl font-bold text-[var(--color-text)] tabular-nums">{String(unit.value).padStart(2, '0')}</span>
            <span class="text-xs text-[var(--color-muted)]">{unit.label}</span>
          </div>
        {/each}
      </div>
    </div>
  </section>
{/if}
