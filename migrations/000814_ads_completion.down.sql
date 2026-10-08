-- 000814_ads_completion.down.sql

DELETE FROM public.ad_slot_types WHERE slot_name IN (
  'left_rail','article_inline','article_below_comments','comments_below',
  'native_feed','native_discover','native_space_feed','native_forum',
  'sticky_anchor','sidebar_sticky'
) AND id NOT IN (SELECT DISTINCT slot_type_id FROM public.ad_campaigns WHERE slot_type_id IS NOT NULL);

ALTER TABLE public.ad_events DROP CONSTRAINT ad_events_event_type_check;
ALTER TABLE public.ad_events
    ADD CONSTRAINT ad_events_event_type_check CHECK ((event_type = ANY (ARRAY['impression'::text,'click'::text])));

ALTER TABLE public.ad_networks DROP CONSTRAINT ad_networks_provider_check;
ALTER TABLE public.ad_networks
    ADD CONSTRAINT ad_networks_provider_check CHECK ((provider = ANY (ARRAY['adsense'::text,'ezoic'::text,'media_net'::text,'custom'::text])));

DROP INDEX IF EXISTS public.idx_ad_campaigns_house;
ALTER TABLE public.ad_campaigns DROP COLUMN is_house;
