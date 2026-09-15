<script lang="ts">
  import type { GuestOpportunitiesProps, GuestHomeData, GuestHomeOpportunity } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { getGuestHomeData } from '../../data/guestHomeContext';
  import GuestCardList from './GuestCardList.svelte';
  import { getLucideIcon } from './guestIconMap';
  import { MapPin } from 'lucide-svelte';

  interface Props {
    props: GuestOpportunitiesProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, id = 'guest-opportunities' }: Props = $props();

  const guestHomeStore = getGuestHomeData();
  let guestHome: GuestHomeData | undefined = $state(undefined);

  $effect(() => {
    if (!guestHomeStore) return;
    const unsub = guestHomeStore.subscribe((v) => (guestHome = v));
    return unsub;
  });

  const items = $derived<GuestHomeOpportunity[]>(
    props.data_source === 'manual' && props.items
      ? props.items
      : (guestHome?.opportunities ?? [])
  );

  const visibleItems = $derived(items.slice(0, props.max_items ?? 6));
  const title = $derived(props.title ?? 'Featured Opportunities');
  const subtitle = $derived(props.subtitle ?? 'Hand-picked opportunities from around the world.');

  function opportunityHref(opp: GuestHomeOpportunity): string {
    if (opp.href) return opp.href;
    if (opp.type === 'job') return '/jobs';
    if (opp.type === 'scholarship') return '/scholarships';
    return '/explore';
  }

  function opportunityTagClasses(type: string): string {
    switch (type) {
      case 'job':
        return 'bg-green-100 text-green-600';
      case 'scholarship':
        return 'bg-blue-100 text-blue-600';
      case 'visa':
        return 'bg-orange-100 text-orange-600';
      case 'business':
        return 'bg-teal-100 text-teal-600';
      case 'internship':
        return 'bg-purple-100 text-purple-600';
      default:
        return 'bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]';
    }
  }

  function logoBg(type: string): string {
    switch (type) {
      case 'job':
        return 'bg-sky-50 text-sky-500';
      case 'scholarship':
        return 'bg-blue-50 text-blue-600';
      case 'visa':
        return 'bg-orange-50 text-orange-600';
      case 'business':
        return 'bg-green-50 text-green-600';
      case 'internship':
        return 'bg-purple-50 text-purple-600';
      default:
        return 'bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]';
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

<!-- Ref: japaconnect-engineering/sections/16-page-builder.md -->
<GuestCardList
  {title}
  {subtitle}
  view_all_href={props.view_all_href ?? '/explore'}
  view_all_label="View all opportunities"
  layout={props.layout ?? 'slider'}
  columns={props.columns ?? 3}
  responsive_columns={props.responsive_columns}
  gap={props.gap ?? 'md'}
  card_style={props.card_style ?? 'default'}
  items={visibleItems}
  {id}
>
  {#snippet card(opp: GuestHomeOpportunity)}
    <a href={opportunityHref(opp)} class="opportunity-card">
      <div class="opportunity-card-top">
        <span class="opportunity-type {opportunityTagClasses(opp.type)}">{opp.tag}</span>
        {#if opp.badge}
          <span class="opportunity-badge">{opp.badge}</span>
        {/if}
      </div>
      <div class="opportunity-logo">
        {#if opp.logo_url}
          <img src={opp.logo_url} alt="" class="opportunity-logo-img" />
        {:else}
          <div class="opportunity-logo-fallback {logoBg(opp.type)}">{logoInitials(opp.organization)}</div>
        {/if}
      </div>
      <h3 class="opportunity-title">{opp.title}</h3>
      <p class="opportunity-org">{opp.organization}</p>
      <div class="opportunity-meta">
        <span class="opportunity-location">
          <MapPin size={14} />
          {opp.location}
        </span>
        {#if opp.value}
          <span class="opportunity-value">{opp.value}</span>
        {/if}
      </div>
      <div class="opportunity-footer">
        <span class="opportunity-type-pill {opportunityTagClasses(opp.type)}">{opp.tag}</span>
        <span class="opportunity-time">{timeAgo(opp.published_at)}</span>
      </div>
    </a>
  {/snippet}
</GuestCardList>
