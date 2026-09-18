<script lang="ts">
  // Ref: japaconnect-engineering/sections/16-page-builder.md
  // Single opportunity card inside `guest_opportunities`, rendered as a child
  // block so each card is selectable/editable in the page builder tree.
  import type { GuestOpportunityItemProps } from '../../types';
  import { MapPin } from 'lucide-svelte';

  interface Props {
    props: GuestOpportunityItemProps;
    id?: string;
  }

  let { props, id }: Props = $props();

  const type = $derived(props.type ?? 'job');
  const organization = $derived(props.organization ?? '');

  function opportunityHref(): string {
    if (props.href) return props.href;
    if (type === 'job') return '/jobs';
    if (type === 'scholarship') return '/scholarships';
    return '/explore';
  }

  function opportunityTagClasses(t: string): string {
    switch (t) {
      case 'job': return 'bg-green-100 text-green-600';
      case 'scholarship': return 'bg-blue-100 text-blue-600';
      case 'visa': return 'bg-orange-100 text-orange-600';
      case 'business': return 'bg-teal-100 text-teal-600';
      case 'internship': return 'bg-purple-100 text-purple-600';
      default: return 'bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]';
    }
  }

  function logoBg(t: string): string {
    switch (t) {
      case 'job': return 'bg-sky-50 text-sky-500';
      case 'scholarship': return 'bg-blue-50 text-blue-600';
      case 'visa': return 'bg-orange-50 text-orange-600';
      case 'business': return 'bg-green-50 text-green-600';
      case 'internship': return 'bg-purple-50 text-purple-600';
      default: return 'bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]';
    }
  }

  function logoInitials(name: string): string {
    return name
      .split(' ')
      .map((w) => w[0])
      .filter(Boolean)
      .slice(0, 2)
      .join('')
      .toUpperCase();
  }

  function timeAgo(dateStr: string | undefined): string {
    if (!dateStr) return 'Recently';
    const date = new Date(dateStr);
    const now = new Date();
    const seconds = Math.floor((now.getTime() - date.getTime()) / 1000);
    const intervals = [
      { label: 'year', seconds: 31536000 },
      { label: 'month', seconds: 2592000 },
      { label: 'week', seconds: 604800 },
      { label: 'day', seconds: 86400 },
      { label: 'hour', seconds: 3600 },
      { label: 'minute', seconds: 60 },
    ];
    for (const i of intervals) {
      const count = Math.floor(seconds / i.seconds);
      if (count >= 1) return `${count} ${i.label}${count > 1 ? 's' : ''} ago`;
    }
    return 'Just now';
  }
</script>

<a href={opportunityHref()} class="opportunity-card" {id}>
  <div class="opportunity-card-top">
    <span class="opportunity-type {opportunityTagClasses(type)}">{props.tag ?? type}</span>
    {#if props.badge}
      <span class="opportunity-badge">{props.badge}</span>
    {/if}
  </div>
  <div class="opportunity-logo">
    {#if props.logo_url}
      <img src={props.logo_url} alt="" class="opportunity-logo-img" />
    {:else}
      <div class="opportunity-logo-fallback {logoBg(type)}">{logoInitials(organization)}</div>
    {/if}
  </div>
  <h3 class="opportunity-title">{props.title ?? ''}</h3>
  <p class="opportunity-org">{organization}</p>
  <div class="opportunity-meta">
    <span class="opportunity-location">
      <MapPin size={14} />
      {props.location ?? ''}
    </span>
    {#if props.value}
      <span class="opportunity-value">{props.value}</span>
    {/if}
  </div>
  <div class="opportunity-footer">
    <span class="opportunity-type-pill {opportunityTagClasses(type)}">{type}</span>
    <span class="opportunity-time">{timeAgo(props.published_at)}</span>
  </div>
</a>
