<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
</script>

<svelte:head>
	<title>{t('contact.title')} | JapaConnect</title>
	<meta name="description" content="{t('search.find')}" />
</svelte:head>

<main class="mx-auto max-w-3xl px-6 py-16">
	<h1 class="text-3xl font-bold" style="color: var(--color-foreground)">{t('contact.title')}</h1>
	<p class="mt-3 text-lg" style="color: var(--color-muted)">
		{t('search.find')}
	</p>

	<div class="mt-8 grid gap-6 sm:grid-cols-2">
		<div class="rounded-xl border p-6" style="border-color: var(--color-border); background: var(--color-surface);">
			<h2 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('tickets.category')}</h2>
			<p class="mt-2 text-sm" style="color: var(--color-muted)">
				{t('tickets.account')}
			</p>
			<a
				href="mailto:hello@japaconnect.com"
				class="mt-3 inline-block text-sm font-medium hover:underline"
				style="color: var(--color-primary)"
			>
				hello@japaconnect.com
			</a>
		</div>

		<div class="rounded-xl border p-6" style="border-color: var(--color-border); background: var(--color-surface);">
			<h2 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('tickets.technical')}</h2>
			<p class="mt-2 text-sm" style="color: var(--color-muted)">
				{t('tickets.billing')}
			</p>
			<a
				href="mailto:support@japaconnect.com"
				class="mt-3 inline-block text-sm font-medium hover:underline"
				style="color: var(--color-primary)"
			>
				support@japaconnect.com
			</a>
		</div>

		<div class="rounded-xl border p-6" style="border-color: var(--color-border); background: var(--color-surface);">
			<h2 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('tickets.feature_request')}</h2>
			<p class="mt-2 text-sm" style="color: var(--color-muted)">
				{t('tickets.content')}
			</p>
			<a
				href="mailto:partners@japaconnect.com"
				class="mt-3 inline-block text-sm font-medium hover:underline"
				style="color: var(--color-primary)"
			>
				partners@japaconnect.com
			</a>
		</div>

		<div class="rounded-xl border p-6" style="border-color: var(--color-border); background: var(--color-surface);">
			<h2 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('tickets.other')}</h2>
			<p class="mt-2 text-sm" style="color: var(--color-muted)">
				{t('tickets.other')}
			</p>
			<a
				href="mailto:security@japaconnect.com"
				class="mt-3 inline-block text-sm font-medium hover:underline"
				style="color: var(--color-primary)"
			>
				security@japaconnect.com
			</a>
		</div>
	</div>
</main>
