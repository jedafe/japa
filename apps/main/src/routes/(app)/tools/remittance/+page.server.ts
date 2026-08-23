import { error, fail, redirect } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiList, apiGet, apiPost, apiPut, ApiError } from '$lib/server/api';
import type {
	RemittanceCorridor,
	RemittanceProvider,
	RemittanceRate,
	RemittanceCalculation,
	RemittanceProviderCalc,
	ComparisonCreated,
	JCEComparison,
	ComparisonItem,
	ComparisonDimension
} from '$lib/types';

/**
 * Remittance Rate Tracker at /tools/remittance (Section 74, ADR-075).
 * SSR-fetches the supported corridors so the corridor selector renders
 * without a client round-trip. Rates, history, and alerts are fetched
 * client-side after the user selects a corridor.
 *
 * Ref: japaconnect-engineering/sections/74-remittance-tracker.md
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['remittance.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Remittance Rate Tracker is not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	let corridors: RemittanceCorridor[] = [];
	let disabled = false;
	try {
		corridors = (await apiList<RemittanceCorridor>(ctx, '/remittance/corridors')).items;
	} catch (e) {
		if (e instanceof ApiError && (e.code === 'remittance_disabled' || e.status === 404)) {
			disabled = true;
		}
	}

	return { corridors, disabled, authed };
};

/**
 * Build a private quick JCE comparison from 2-4 selected remittance providers.
 * The matrix is pre-seeded from the calculator result when send_amount is
 * provided: Receive amount, Total cost, Rate, and Fee. If the calculator is
 * unavailable, it falls back to corridor rates. Provider name, Website, and
 * Data source are also seeded from the provider listing.
 *
 * Ref: japaconnect-engineering/sections/74-remittance-tracker.md (Integration)
 */
export const actions: Actions = {
	default: async (event) => {
		if (!event.locals.user) {
			return fail(401, { error: 'unauthenticated', message: 'Sign in to save a comparison' });
		}

		const form = await event.request.formData();
		const providerIds = form
			.getAll('provider_id')
			.filter((v): v is string => typeof v === 'string');
		const sendCurrency = form.get('send_currency');
		const receiveCurrency = form.get('receive_currency');
		const sendAmount = form.get('send_amount');

		if (providerIds.length < 2 || providerIds.length > 4) {
			return fail(400, { error: 'invalid_selection', message: 'Select 2 to 4 providers' });
		}

		const ctx = apiContext(event);

		let providers: RemittanceProvider[];
		try {
			providers = await apiGet<RemittanceProvider[]>(ctx, '/remittance/providers');
		} catch (e) {
			const message = e instanceof ApiError ? e.message : 'Could not load providers';
			return fail(500, { error: 'fetch_failed', message });
		}

		const providerById = new Map(providers.map((p) => [p.id, p]));
		const orderedProviders = providerIds
			.map((id) => providerById.get(id))
			.filter((p): p is RemittanceProvider => !!p);

		if (orderedProviders.length !== providerIds.length) {
			return fail(500, { error: 'provider_not_found', message: 'One or more selected providers could not be resolved' });
		}

		// Try to load a calculation for the user's amount. The calc result is
		// the canonical source for rate/fee/receive_amount/total_cost.
		let calcProviders: RemittanceProviderCalc[] = [];
		if (
			sendCurrency &&
			receiveCurrency &&
			sendAmount &&
			typeof sendCurrency === 'string' &&
			typeof receiveCurrency === 'string' &&
			typeof sendAmount === 'string'
		) {
			try {
				const calc = await apiGet<RemittanceCalculation>(
					ctx,
					`/remittance/calculate?send=${encodeURIComponent(sendCurrency)}&receive=${encodeURIComponent(receiveCurrency)}&amount=${encodeURIComponent(sendAmount)}`
				);
				calcProviders = calc.providers ?? [];
			} catch {
				// Calculation seeding is non-fatal; we fall back to corridor rates below.
				calcProviders = [];
			}
		}

		const calcByProviderId = new Map<string, RemittanceProviderCalc>(
			calcProviders.map((c) => [c.provider_id, c])
		);

		// Fallback to corridor rates when no calculation is available.
		let rates: RemittanceRate[] = [];
		if (sendCurrency && receiveCurrency && typeof sendCurrency === 'string' && typeof receiveCurrency === 'string') {
			try {
				rates = await apiGet<RemittanceRate[]>(
					ctx,
					`/remittance/rates?send=${encodeURIComponent(sendCurrency)}&receive=${encodeURIComponent(receiveCurrency)}`
				);
			} catch {
				rates = [];
			}
		}

		const rateByProviderId = new Map(rates.map((r) => [r.provider_id, r]));

		const items = orderedProviders.map((p, i) => ({
			title: p.name,
			description: p.website_url || '',
			position: i,
			item_ref: { type: 'remittance', id: p.id }
		}));

		const dimensionLabels = ['Receive amount', 'Total cost', 'Rate', 'Fee', 'Transfer time', 'Website', 'Data source', 'Provider name'];

		let created: ComparisonCreated;
		try {
			created = await apiPost<ComparisonCreated>(ctx, '/jce/comparisons', {
				title: 'Remittance rate comparison',
				depth: 'quick',
				body: '',
				items,
				dimensions: dimensionLabels
			});
		} catch (e) {
			const message = e instanceof ApiError ? e.message : 'Could not create comparison';
			return fail(500, { error: 'create_failed', message });
		}

		let jce: JCEComparison;
		try {
			jce = await apiGet<JCEComparison>(ctx, `/jce/comparisons/${created.id}`);
		} catch (e) {
			const message = e instanceof ApiError ? e.message : 'Could not load created comparison';
			return fail(500, { error: 'detail_failed', message });
		}

		const itemByPosition = new Map<number, ComparisonItem>();
		for (const [i, item] of jce.items.entries()) {
			itemByPosition.set(i, item);
		}
		const dimByLabel = new Map<string, ComparisonDimension>(
			jce.dimensions.map((d) => [d.label, d])
		);

		function getCalc(p: RemittanceProvider): RemittanceProviderCalc | null {
			return calcByProviderId.get(p.id) ?? null;
		}

		function getRate(p: RemittanceProvider): RemittanceRate | null {
			return rateByProviderId.get(p.id) ?? null;
		}

		function valueForProvider(p: RemittanceProvider, label: string): string {
			const calc = getCalc(p);
			const rate = getRate(p);

			switch (label) {
				case 'Receive amount':
					if (calc) return calc.receive_amount.toFixed(2);
					if (rate && sendAmount && typeof sendAmount === 'string') {
						const amount = parseFloat(sendAmount);
						if (Number.isFinite(amount)) {
							return (amount * rate.rate).toFixed(2);
						}
					}
					return '';
				case 'Total cost':
					return calc ? calc.total_cost.toFixed(2) : '';
				case 'Rate':
					if (calc) return calc.rate.toFixed(2);
					return rate ? rate.rate.toFixed(2) : '';
				case 'Fee':
					if (calc) return calc.fee_flat.toFixed(2);
					if (rate) {
						if (rate.fee_flat && rate.fee_flat > 0) return rate.fee_flat.toFixed(2);
						if (rate.fee_pct && rate.fee_pct > 0) return `${(rate.fee_pct * 100).toFixed(2)}%`;
					}
					return '';
				case 'Transfer time':
					return rate?.transfer_time || '';
				case 'Website':
					return p.website_url || '';
				case 'Data source':
					return p.data_source || '';
				case 'Provider name':
					return p.name;
				default:
					return '';
			}
		}

		for (const label of dimensionLabels) {
			const dim = dimByLabel.get(label);
			if (!dim) continue;

			const values = orderedProviders
				.map((p, i) => {
					const item = itemByPosition.get(i);
					if (!item) return null;
					return { item_id: item.id, value: valueForProvider(p, label) };
				})
				.filter((v): v is { item_id: string; value: string } => v !== null);

			if (values.length === 0) continue;

			try {
				await apiPut<unknown>(ctx, `/jce/comparisons/dimensions/${dim.id}/values`, { values });
			} catch (e) {
				const message = e instanceof ApiError ? e.message : String(e);
				console.error('failed to seed dimension values:', message);
			}
		}

		throw redirect(303, `/compare/${created.slug}`);
	}
};
