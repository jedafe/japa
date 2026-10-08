-- 000814_ads_completion.up.sql — ads system completion (§25 amendment).
-- House-ad flag, widened provider CHECK, conversion event type, new slot types.

-- 1. House-ad flag + serving index.
ALTER TABLE public.ad_campaigns
    ADD COLUMN is_house boolean NOT NULL DEFAULT false;

CREATE INDEX idx_ad_campaigns_house
    ON public.ad_campaigns USING btree (slot_type_id, status)
    WHERE is_house;

-- 2. Widen ad-network provider enum to the full supported list.
ALTER TABLE public.ad_networks DROP CONSTRAINT ad_networks_provider_check;
ALTER TABLE public.ad_networks
    ADD CONSTRAINT ad_networks_provider_check CHECK ((provider = ANY (ARRAY[
        'adsense'::text,'google_admanager'::text,'ezoic'::text,'media_net'::text,
        'mediavine'::text,'raptive'::text,'freestar'::text,'setupad'::text,
        'amazon_aps'::text,'prebid'::text,'magnite'::text,'index_exchange'::text,
        'pubmatic'::text,'openx'::text,'sovrn'::text,'criteo'::text,
        'monumetric'::text,'she_media'::text,'carbon'::text,'buysellads'::text,
        'snigel'::text,'publift'::text,'adpushup'::text,'valueimpression'::text,
        'custom'::text
    ])));

-- 3. Conversion events.
ALTER TABLE public.ad_events DROP CONSTRAINT ad_events_event_type_check;
ALTER TABLE public.ad_events
    ADD CONSTRAINT ad_events_event_type_check CHECK ((event_type = ANY (ARRAY['impression'::text,'click'::text,'conversion'::text])));

-- 4. New slot types (§25 amendment table).
INSERT INTO public.ad_slot_types (slot_name, description, max_display_per_page) VALUES
  ('left_rail',              'Skyscraper ad in the collapsed-nav left rail', 1),
  ('article_inline',         'In-article ad injected inside the body after N paragraphs', 2),
  ('article_below_comments', 'Ad below the comments section on detail pages', 1),
  ('comments_below',         'Ad directly under comment threads', 1),
  ('native_feed',            'Native card-styled ad in the main feed', 2),
  ('native_discover',        'Native card-styled ad in the Discover grid', 2),
  ('native_space_feed',      'Native card-styled ad in space feeds', 2),
  ('native_forum',           'Native ad between forum thread rows', 2),
  ('sticky_anchor',          'Dismissible sticky banner anchored to the viewport bottom', 1),
  ('sidebar_sticky',         'Sidebar ad that remains visible while scrolling', 1)
ON CONFLICT DO NOTHING;
