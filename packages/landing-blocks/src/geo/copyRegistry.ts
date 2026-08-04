// packages/landing-blocks/src/geo/copyRegistry.ts
import type { GeoCopyTokens } from '../types';

/** Fallback tokens for unknown/global visitors — never a raw {{token}}. */
export const GLOBAL_TOKENS: GeoCopyTokens = {
  country: 'your country',
  country_short: 'global',
  country_adjective: 'local',
  continent: 'abroad',
  region: '',
  diaspora_phrase: 'the diaspora',
  destination_phrase: 'a new country',
  locale_hint: 'en',
  names: { en: 'your country', fr: 'votre pays' },
};

/** ISO-2 → GeoCopyTokens. Seeded with top diaspora destinations. */
export const geoCopy: Record<string, GeoCopyTokens> = {
  NG: {
    country: 'Nigeria', country_short: 'NG', country_adjective: 'Nigerian',
    continent: 'Africa', region: 'West Africa',
    diaspora_phrase: 'Nigerians at home',
    destination_phrase: 'your destination',
    locale_hint: 'en-NG',
    names: { en: 'Nigeria', fr: 'Nigeria' },
  },
  GB: {
    country: 'the UK', country_short: 'UK', country_adjective: 'British',
    continent: 'Europe', region: 'Western Europe',
    diaspora_phrase: 'Nigerians in the UK',
    destination_phrase: 'Nigeria',
    locale_hint: 'en-GB',
    names: { en: 'the UK', fr: 'le Royaume-Uni' },
  },
  US: {
    country: 'the US', country_short: 'US', country_adjective: 'American',
    continent: 'North America', region: 'North America',
    diaspora_phrase: 'Nigerians in the US',
    destination_phrase: 'Nigeria',
    locale_hint: 'en-US',
    names: { en: 'the US', fr: 'les États-Unis' },
  },
  CA: {
    country: 'Canada', country_short: 'CA', country_adjective: 'Canadian',
    continent: 'North America', region: 'North America',
    diaspora_phrase: 'Nigerians in Canada',
    destination_phrase: 'Nigeria',
    locale_hint: 'en-CA',
    names: { en: 'Canada', fr: 'Canada' },
  },
  DE: {
    country: 'Germany', country_short: 'DE', country_adjective: 'German',
    continent: 'Europe', region: 'Western Europe',
    diaspora_phrase: 'Nigerians in Germany',
    destination_phrase: 'Nigeria',
    locale_hint: 'de-DE',
    names: { en: 'Germany', fr: 'Allemagne' },
  },
  AU: {
    country: 'Australia', country_short: 'AU', country_adjective: 'Australian',
    continent: 'Oceania', region: 'Oceania',
    diaspora_phrase: 'Nigerians in Australia',
    destination_phrase: 'Nigeria',
    locale_hint: 'en-AU',
    names: { en: 'Australia', fr: 'Australie' },
  },
  IE: {
    country: 'Ireland', country_short: 'IE', country_adjective: 'Irish',
    continent: 'Europe', region: 'Western Europe',
    diaspora_phrase: 'Nigerians in Ireland',
    destination_phrase: 'Nigeria',
    locale_hint: 'en-IE',
    names: { en: 'Ireland', fr: 'Irlande' },
  },
  FR: {
    country: 'France', country_short: 'FR', country_adjective: 'French',
    continent: 'Europe', region: 'Western Europe',
    diaspora_phrase: 'Nigerians in France',
    destination_phrase: 'Nigeria',
    locale_hint: 'fr-FR',
    names: { en: 'France', fr: 'la France' },
  },
  NL: {
    country: 'the Netherlands', country_short: 'NL', country_adjective: 'Dutch',
    continent: 'Europe', region: 'Western Europe',
    diaspora_phrase: 'Nigerians in the Netherlands',
    destination_phrase: 'Nigeria',
    locale_hint: 'nl-NL',
    names: { en: 'the Netherlands', fr: 'les Pays-Bas' },
  },
  IT: {
    country: 'Italy', country_short: 'IT', country_adjective: 'Italian',
    continent: 'Europe', region: 'Southern Europe',
    diaspora_phrase: 'Nigerians in Italy',
    destination_phrase: 'Nigeria',
    locale_hint: 'it-IT',
    names: { en: 'Italy', fr: 'Italie' },
  },
  ZA: {
    country: 'South Africa', country_short: 'ZA', country_adjective: 'South African',
    continent: 'Africa', region: 'Southern Africa',
    diaspora_phrase: 'Nigerians in South Africa',
    destination_phrase: 'Nigeria',
    locale_hint: 'en-ZA',
    names: { en: 'South Africa', fr: 'Afrique du Sud' },
  },
  AE: {
    country: 'the UAE', country_short: 'AE', country_adjective: 'Emirati',
    continent: 'Asia', region: 'Middle East',
    diaspora_phrase: 'Nigerians in the UAE',
    destination_phrase: 'Nigeria',
    locale_hint: 'en-AE',
    names: { en: 'the UAE', fr: 'les Émirats arabes unis' },
  },
};

/** Resolve tokens for a country code, returning localized names for the locale. */
export function getGeoTokens(countryCode: string | undefined, locale: 'en' | 'fr' = 'en'): GeoCopyTokens {
  if (!countryCode || countryCode === 'global') return GLOBAL_TOKENS;
  const tokens = geoCopy[countryCode.toUpperCase()];
  if (!tokens) return GLOBAL_TOKENS;
  // Return tokens with the country name localized for the active locale.
  return { ...tokens, country: tokens.names[locale] ?? tokens.names.en };
}
