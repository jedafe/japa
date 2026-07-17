<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
</script>

<svelte:head>
	<title>{t('terms.title')} | JapaConnect</title>
	<meta name="description" content="JapaConnect Terms of Service." />
</svelte:head>

<main class="mx-auto max-w-3xl px-6 py-16">
	<h1 class="text-3xl font-bold" style="color: var(--color-foreground)">{t('terms.title')}</h1>
	<p class="mt-2 text-sm" style="color: var(--color-muted)">{t('terms.last_updated_june_2026')}</p>
	<div class="mt-6 space-y-4 text-base" style="color: var(--color-muted)">
		<h2 class="pt-4 text-xl font-semibold" style="color: var(--color-foreground)">{t('terms.1_acceptance_of_terms')}</h2>
		<p>
			By accessing or using JapaConnect, you agree to be bound by these Terms of Service. If you do not agree to these terms, please do not use the platform.
		</p>
		<h2 class="pt-4 text-xl font-semibold" style="color: var(--color-foreground)">{t('terms.2_user_responsibilities')}</h2>
		<p>
			You are responsible for maintaining the confidentiality of your account credentials and for all activities that occur under your account. You agree to provide accurate and complete information when creating your account.
		</p>
		<h2 class="pt-4 text-xl font-semibold" style="color: var(--color-foreground)">{t('terms.3_content')}</h2>
		<p>
			Users retain ownership of content they create on JapaConnect. By posting content, you grant JapaConnect a non-exclusive license to display and distribute your content within the platform.
		</p>
		<h2 class="pt-4 text-xl font-semibold" style="color: var(--color-foreground)">{t('terms.4_prohibited_conduct')}</h2>
		<p>
			You may not use JapaConnect to harass others, distribute spam, impersonate individuals, or engage in any activity that violates applicable laws or these terms.
		</p>
		<h2 class="pt-4 text-xl font-semibold" style="color: var(--color-foreground)">{t('terms.5_termination')}</h2>
		<p>
			JapaConnect reserves the right to suspend or terminate your account at any time for conduct that violates these terms or is otherwise harmful to the platform or its users.
		</p>
	</div>
</main>
