-- 000685_seed_intel_channel_settings.up.sql
-- Seed the intel-channels Phase 1 system_settings rows (intel-channels
-- curation spec, §System settings) so every consumed key is listed in the
-- control settings UI with its code default instead of falling back to an
-- uncatalogued row. Phase-2/3 keys (render, search, email, social) are
-- seeded empty/disabled so admins can see and pre-configure them; no Phase 1
-- code path reads them yet.
--
-- intel.category_method_map carries the spec §2 category→method map — the
-- same document embedded at internal/intelligence/sources/category_map.json.
-- An empty/malformed value resolves against the embedded default, so this
-- seed exists to make the map admin-editable in place.
--
-- Post-de-tenant convention (000668 dropped system_settings.tenant_id):
-- rows are keyed globally on `key`; conflicts use ON CONFLICT (key).

BEGIN;

INSERT INTO system_settings (key, value) VALUES
    -- Probe (§2 preview + verify stages)
    ('intel.probe.max_requests', to_jsonb(5)),
    ('intel.probe.verify_max_requests', to_jsonb(25)),
    ('intel.probe.max_urls', to_jsonb(50)),
    ('intel.probe.rate_limit_per_hr', to_jsonb(20)),
    -- Category → method/pipeline/topics map (spec §2)
    ('intel.category_method_map', '{
  "job":        {"entity_types": ["job"],          "methods": ["jsonld","api","tabular","render","static"], "pipeline": "extract",
                 "topics": ["job","hiring","vacancy","career","remote work","recruitment"]},
  "news":       {"entity_types": ["news"],          "methods": ["rss","jsonld","render","static"],         "pipeline": "curate",
                 "topics": ["migration","immigration","japa","diaspora","visa","relocation","travel"]},
  "blog":       {"entity_types": ["article"],       "methods": ["rss","render","static"],                  "pipeline": "curate",
                 "topics": ["japa","relocation","study abroad","settlement","expat","diaspora"]},
  "event":      {"entity_types": ["event"],         "methods": ["ical","jsonld"],                          "pipeline": "curate",
                 "topics": ["webinar","fair","embassy","orientation","deadline"]},
  "scholarship":{"entity_types": ["scholarship"],   "methods": ["rss","jsonld","render","static"],         "pipeline": "extract",
                 "topics": ["scholarship","grant","fellowship","funding","tuition waiver","bursary"]},
  "visa":       {"entity_types": ["visa"],          "methods": ["static","pdf","render"],                  "pipeline": "extract",
                 "topics": ["visa","immigration","embassy","residence permit","asylum","border"]},
  "dataset":    {"entity_types": ["dataset"],       "methods": ["tabular","git","api"],                    "pipeline": "curate",
                 "topics": ["exchange rate","processing time","visa fee","tuition"]},
  "video":      {"entity_types": [],                "methods": ["channel_sync"],                           "pipeline": "curate",
                 "topics": ["japa","relocation","study abroad","visa"]}
}'::jsonb),
    -- Curate pipeline (§4)
    ('intel.curate.auto_publish_trust_max', to_jsonb(2)),
    -- Relevance gate (§4)
    ('intel.relevance.on_topic_threshold', to_jsonb(0.15)),
    ('intel.relevance.off_topic_threshold', to_jsonb(0.03)),
    ('intel.relevance.borderline_rate_cap', to_jsonb(50)),
    ('intel.relevance.classifier_enabled', to_jsonb(true)),
    ('intel.relevance.classifier_model', to_jsonb(''::text)),
    -- Channel failure auto-disable (pre-existing consumer, now catalogued)
    ('intel.consecutive_failure_disable_threshold', to_jsonb(5)),
    -- Phase 2/3 placeholders (spec §System settings) — no Phase 1 consumer
    ('intel.render.service_url', to_jsonb(''::text)),
    ('intel.render.timeout_ms', to_jsonb(30000)),
    ('intel.search.provider', to_jsonb(''::text)),
    ('intel.search.api_key', to_jsonb(''::text)),
    ('intel.email.inbound_domain', to_jsonb(''::text)),
    ('intel.email.allowed_senders', '[]'::jsonb),
    ('intel.social.enabled_providers', '[]'::jsonb)
ON CONFLICT (key) DO NOTHING;

COMMIT;
