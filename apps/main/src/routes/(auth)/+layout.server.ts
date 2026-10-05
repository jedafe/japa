// Ref: japaconnect-engineering/sections/02-authentication-sessions.md
// ADR-058 4th exception: auth gate. Authed users are redirected away from
// (auth)/* routes — except verify-email, which authed users legitimately
// visit when clicking the email verification link.
import { redirect } from '@sveltejs/kit';
import type { LayoutServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import { resolvePostLoginRedirect } from '$lib/server/auth';
import type { OnboardingStatus } from '$lib/types';

export const load: LayoutServerLoad = async (event) => {
	const user = event.locals.user;
	const routeId = event.route.id ?? '';

	// verify-email is exempt — authed users click the email link to verify.
	if (user && !routeId.startsWith('/(auth)/verify-email')) {
		const ctx = apiContext(event);
		let onboarding: OnboardingStatus | null = null;
		try {
			onboarding = await apiGet<OnboardingStatus>(ctx, '/onboarding/status');
		} catch {
			onboarding = null;
		}

		// resolvePostLoginRedirect sanitizes the ?redirect= target (§120:
		// relative paths or the allowlisted API origin only) and threads it
		// through /onboarding when the wizard hasn't run yet.
		throw redirect(
			303,
			resolvePostLoginRedirect(onboarding, event.url.searchParams.get('redirect'))
		);
	}

	return {};
};
