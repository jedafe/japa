<script lang="ts">
  import type { FormEmbedProps } from '../types';
  import type { FetchContext } from '../data/types';
  import { buildApiUrl } from '@japaconnect/api-url';

  interface Props {
    props: FormEmbedProps;
    id?: string;
    fetchCtx?: FetchContext;
  }
  let { props, id, fetchCtx }: Props = $props();

  // Field schema as emitted by GET /forms/{slug}/embed (§06).
  interface EmbedField {
    id: string;
    key?: string;
    label: string;
    type: string;
    required?: boolean;
    placeholder?: string | null;
    options?: string[] | null;
    option_defs?: { value: string; label: string }[] | null;
    options_source?: string | null;
  }

  interface RenderField {
    name: string;
    label: string;
    type: string;
    required: boolean;
    placeholder?: string;
    options?: string[];
    options_source?: string | null;
  }

  let formData = $state<Record<string, string | string[]>>({});
  let submitting = $state(false);
  let success = $state(false);
  let error = $state(false);
  let liveFields = $state<EmbedField[] | null>(null);
  let liveFormId = $state<string | null>(null);
  let countries = $state<string[]>([]);

  // §06: when form_slug names a platform form, pull the live field schema
  // from the public embed endpoint so the block never drifts from admin
  // edits (the embed is already guest-scoped). props.fields is the fallback.
  //
  // The API base comes from the injected fetch context when present — the
  // baked submit_endpoint is a *relative* URL on split-origin deploys
  // (japa.to does not proxy /api), so deriving the base from it resolves
  // every fetch against the frontend origin and 404s.
  const SUBMISSIONS_ENDPOINT = /\/forms\/([0-9a-f-]{36})\/submissions\/?$/i;
  const apiBase = $derived(
    fetchCtx?.apiBase ?? props.submit_endpoint.replace(SUBMISSIONS_ENDPOINT, '')
  );
  const doFetch = $derived(fetchCtx?.fetch ?? fetch);
  // X-JC-Surface lets the backend resolve feature flags/geo rules for the
  // right surface (same convention as GuestHomeLegacyBlock/WidgetBlock).
  const reqHeaders = $derived.by((): Record<string, string> => {
    const h: Record<string, string> = {};
    if (fetchCtx?.surface) h['x-jc-surface'] = fetchCtx.surface;
    return h;
  });

  $effect(() => {
    if (!props.form_slug || !apiBase) return;
    let cancelled = false;
    doFetch(buildApiUrl(`/forms/${props.form_slug}/embed`, apiBase), { headers: reqHeaders })
      .then((r) => r.json())
      .then((p) => {
        const fields = p?.data?.fields;
        if (!cancelled && Array.isArray(fields) && fields.length) liveFields = fields;
        // The live form id is also the submission target — the baked
        // submit_endpoint carries a literal UUID that goes stale when the
        // seeded form lives under a different id (e.g. a contact form that
        // predated the seed kept its own id on slug conflict).
        const id = p?.data?.id;
        if (!cancelled && typeof id === 'string' && id) liveFormId = id;
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  });

  // geo_countries suggest fields resolve their options client-side the same
  // way the SSR renderers do — fetched once when a live field sources them.
  $effect(() => {
    if (!apiBase || !liveFields?.some((f) => f.options_source === 'geo_countries')) return;
    let cancelled = false;
    doFetch(buildApiUrl('/geo/countries', apiBase), { headers: reqHeaders })
      .then((r) => r.json())
      .then((p) => {
        if (!cancelled && Array.isArray(p?.data)) {
          countries = p.data
            .map((c: { name?: string }) => c?.name)
            .filter((n: unknown): n is string => typeof n === 'string' && n.length > 0);
        }
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  });

  // Live schema wins; props.fields is the fallback when form_slug is unset
  // or the embed fetch fails. Select options emit labels — the submissions
  // endpoint normalizes labels to canonical values (CF3).
  const renderFields = $derived<RenderField[]>(
    liveFields
      ? liveFields.map((f) => ({
          name: f.key || f.label,
          label: f.label,
          type: f.type,
          required: f.required ?? false,
          placeholder: f.placeholder ?? undefined,
          options_source: f.options_source,
          options:
            f.type === 'select'
              ? (f.option_defs?.length ? f.option_defs.map((o) => o.label) : (f.options ?? []))
              : f.options_source === 'geo_countries'
                ? countries
                : (f.options ?? [])
        }))
      : (props.fields ?? [])
  );

  // §06 POST /forms/{id}/submissions expects the values under field_values;
  // other preset endpoints (newsletter/waitlist) take the bare map.
  // Submissions endpoint rebuilds on apiBase with the best-known form id:
  // the live embed's id is authoritative on every environment; the UUID
  // baked into submit_endpoint is the fallback (kept when the embed fetch
  // fails or no form_slug was configured). Non-forms submit endpoints
  // (newsletter/waitlist presets) pass through untouched.
  const endpoint = $derived.by(() => {
    const m = props.submit_endpoint.match(SUBMISSIONS_ENDPOINT);
    if (!m || !apiBase) return props.submit_endpoint;
    return buildApiUrl(`/forms/${liveFormId ?? m[1]}/submissions`, apiBase);
  });
  const body = $derived(
    SUBMISSIONS_ENDPOINT.test(endpoint) ? { field_values: formData } : formData
  );

  async function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    submitting = true;
    error = false;
    try {
      const headers: Record<string, string> = {
        'content-type': 'application/json',
        ...reqHeaders,
      };
      if (fetchCtx?.csrfToken) headers['x-csrf-token'] = fetchCtx.csrfToken;
      const res = await doFetch(endpoint, {
        method: 'POST',
        headers,
        body: JSON.stringify(body),
        credentials: 'include',
      });
      if (!res.ok) throw new Error('Failed');
      success = true;
    } catch {
      error = true;
    } finally {
      submitting = false;
    }
  }

  const inputCls =
    'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none';
</script>

<section id={id} class="px-6 py-16">
  <div class="mx-auto max-w-xl rounded-[var(--radius-card)] bg-[var(--color-surface)] border border-[var(--color-border)] p-8">
    {#if success}
      <div class="text-center">
        <p class="text-lg font-medium text-[var(--color-primary)]">{props.success_message}</p>
      </div>
    {:else}
      {#if props.title}
        <h2 class="text-xl font-bold text-[var(--color-text)] mb-1">{props.title}</h2>
      {/if}
      {#if props.subtitle}
        <p class="text-sm text-[var(--color-text-subtle)] mb-6">{props.subtitle}</p>
      {/if}
      <form onsubmit={handleSubmit} class="space-y-4">
        {#each renderFields as field}
          <div>
            <label for={field.name} class="block text-xs font-medium text-[var(--color-muted)] mb-1">
              {field.label}{#if field.required}<span class="text-[var(--color-error)]">*</span>{/if}
            </label>
            {#if field.type === 'textarea' || field.type === 'long_text'}
              <textarea
                id={field.name}
                name={field.name}
                placeholder={field.placeholder ?? ''}
                required={field.required}
                bind:value={formData[field.name]}
                class={inputCls}
                rows="4"
              ></textarea>
            {:else if field.type === 'select'}
              <select
                id={field.name}
                name={field.name}
                required={field.required}
                bind:value={formData[field.name]}
                class={inputCls}
              >
                <option value="">Select...</option>
                {#each field.options ?? [] as opt}
                  <option value={opt}>{opt}</option>
                {/each}
              </select>
            {:else if field.type === 'multiselect'}
              <div class="flex flex-col gap-1.5" role="group" aria-label={field.label}>
                {#each field.options ?? [] as opt}
                  <label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
                    <input
                      type="checkbox"
                      class="h-4 w-4 rounded border-[var(--color-border)]"
                      checked={Array.isArray(formData[field.name]) && (formData[field.name] as string[]).includes(opt)}
                      onchange={(e) => {
                        const cur = Array.isArray(formData[field.name])
                          ? (formData[field.name] as string[])
                          : [];
                        const next = e.currentTarget.checked
                          ? [...cur, opt]
                          : cur.filter((v) => v !== opt);
                        formData = { ...formData, [field.name]: next };
                      }}
                    />
                    {opt}
                  </label>
                {/each}
              </div>
            {:else if field.type === 'suggest'}
              {#if field.options_source === 'geo_countries' && field.options?.length}
                <select
                  id={field.name}
                  name={field.name}
                  required={field.required}
                  bind:value={formData[field.name]}
                  class={inputCls}
                >
                  <option value="">Select...</option>
                  {#each field.options as opt}
                    <option value={opt}>{opt}</option>
                  {/each}
                </select>
              {:else if field.options?.length}
                <input
                  id={field.name}
                  name={field.name}
                  list="dl-{field.name}"
                  placeholder={field.placeholder ?? ''}
                  required={field.required}
                  bind:value={formData[field.name]}
                  class={inputCls}
                />
                <datalist id="dl-{field.name}">
                  {#each field.options as opt}
                    <option value={opt}></option>
                  {/each}
                </datalist>
              {:else}
                <input
                  type="text"
                  id={field.name}
                  name={field.name}
                  placeholder={field.placeholder ?? ''}
                  required={field.required}
                  bind:value={formData[field.name]}
                  class={inputCls}
                />
              {/if}
            {:else if field.type === 'boolean' || field.type === 'checkbox'}
              <label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
                <input
                  type="checkbox"
                  id={field.name}
                  required={field.required}
                  checked={formData[field.name] === 'true'}
                  onchange={(e) => (formData = { ...formData, [field.name]: e.currentTarget.checked ? 'true' : '' })}
                  class="h-4 w-4 rounded border-[var(--color-border)]"
                />
                {field.placeholder ?? field.label}
              </label>
            {:else}
              <input
                type={field.type === 'email' ? 'email' : field.type === 'url' ? 'url' : field.type === 'number' ? 'number' : 'text'}
                id={field.name}
                name={field.name}
                placeholder={field.placeholder ?? ''}
                required={field.required}
                bind:value={formData[field.name]}
                class={inputCls}
              />
            {/if}
          </div>
        {/each}
        {#if error}
          <p class="text-sm text-[var(--color-error)]">{props.error_message}</p>
        {/if}
        <button
          type="submit"
          disabled={submitting}
          class="w-full rounded-[var(--radius)] bg-[var(--color-primary)] px-6 py-2.5 text-sm font-semibold text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
        >
          {submitting ? 'Submitting...' : props.submit_label}
        </button>
      </form>
    {/if}
  </div>
</section>
