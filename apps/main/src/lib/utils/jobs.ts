import { t } from '$lib/i18n';

// salary_period is a seeded select (hourly|daily|weekly|biweekly|monthly|
// yearly); this map also tolerates the legacy aliases some imported
// listings carry ("annual", "bi-weekly").
const SALARY_PERIODS: Record<string, string> = {
	year: 'year', yearly: 'year', annual: 'year', annually: 'year',
	month: 'month', monthly: 'month',
	week: 'week', weekly: 'week',
	'2 weeks': 'biweek', biweek: 'biweek', biweekly: 'biweek', 'bi-weekly': 'biweek',
	hour: 'hour', hourly: 'hour',
	day: 'day', daily: 'day'
};

/** Localized "per month"-style label for a salary_period value; '' when unset. */
export function salaryPeriodLabel(v: string): string {
	const norm = SALARY_PERIODS[v.trim().toLowerCase()];
	return norm ? t(`jobs.per_${norm}`, `per ${norm}`) : '';
}
