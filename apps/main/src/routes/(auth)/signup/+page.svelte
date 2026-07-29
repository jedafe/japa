<script lang="ts">
	import SignupForm from '$lib/components/auth/SignupForm.svelte';
	import { page } from '$app/state';

	const redirectTarget = $derived(page.url.searchParams.get('redirect') ?? '/onboarding');
</script>

<SignupForm {redirectTarget} />
