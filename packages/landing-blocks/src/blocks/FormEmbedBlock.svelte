<script lang="ts">
  import type { FormEmbedProps } from '../types';

  interface Props {
    props: FormEmbedProps;
  }
  let { props }: Props = $props();

  let formData = $state<Record<string, string>>({});
  let submitting = $state(false);
  let success = $state(false);
  let error = $state(false);

  async function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    submitting = true;
    error = false;
    try {
      const res = await fetch(props.submit_endpoint, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify(formData),
      });
      if (!res.ok) throw new Error('Failed');
      success = true;
    } catch {
      error = true;
    } finally {
      submitting = false;
    }
  }
</script>

<section class="px-6 py-16">
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
        {#each props.fields as field}
          <div>
            <label for={field.name} class="block text-xs font-medium text-[var(--color-muted)] mb-1">
              {field.label}{#if field.required}<span class="text-[var(--color-error)]">*</span>{/if}
            </label>
            {#if field.type === 'textarea'}
              <textarea
                id={field.name}
                name={field.name}
                placeholder={field.placeholder ?? ''}
                required={field.required}
                bind:value={formData[field.name]}
                class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
                rows="4"
              ></textarea>
            {:else if field.type === 'select'}
              <select
                id={field.name}
                name={field.name}
                required={field.required}
                bind:value={formData[field.name]}
                class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
              >
                <option value="">Select...</option>
                {#each field.options ?? [] as opt}
                  <option value={opt}>{opt}</option>
                {/each}
              </select>
            {:else}
              <input
                type={field.type}
                id={field.name}
                name={field.name}
                placeholder={field.placeholder ?? ''}
                required={field.required}
                bind:value={formData[field.name]}
                class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
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
