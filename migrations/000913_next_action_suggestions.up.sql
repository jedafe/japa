-- 000913_next_action_suggestions.up.sql — §122 Next-Action Suggestion Engine
-- (ADR-132). Rule catalog + per-user state; seeds RBAC nodes, the
-- next_action.suggested notification type, and the six launch rules.
-- Ref: .jeos/sections/122-next-action-suggestions.md

BEGIN;

CREATE TABLE public.next_action_rules (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug              text NOT NULL UNIQUE,
    name              text NOT NULL,
    description       text NOT NULL DEFAULT '',
    enabled           boolean NOT NULL DEFAULT false,
    priority          integer NOT NULL DEFAULT 100,
    audience          jsonb NOT NULL DEFAULT '{}',
    conditions        jsonb NOT NULL DEFAULT '{}',
    payload           jsonb NOT NULL,
    channels          jsonb NOT NULL DEFAULT '[]',
    completion_signal jsonb,
    frequency         jsonb NOT NULL DEFAULT '{}',
    valid_from        timestamptz,
    valid_until       timestamptz,
    created_by        uuid REFERENCES users(id),
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_next_action_rules_enabled ON public.next_action_rules (enabled, priority);

CREATE TABLE public.user_next_actions (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rule_id        uuid NOT NULL REFERENCES next_action_rules(id) ON DELETE CASCADE,
    status         text NOT NULL DEFAULT 'pending'
                   CHECK (status IN ('pending','shown','dismissed','snoozed','completed','expired')),
    impressions    integer NOT NULL DEFAULT 0,
    first_shown_at timestamptz,
    last_shown_at  timestamptz,
    snoozed_until  timestamptz,
    dismissed_at   timestamptz,
    completed_at   timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, rule_id)
);
CREATE INDEX idx_user_next_actions_user ON public.user_next_actions (user_id, status);

-- RBAC nodes (granted to super admins; other roles via the RBAC admin UI —
-- same convention as migration 000826).
INSERT INTO permissions (key, description) VALUES
    ('nextactions.manage', 'Create, edit, and delete next-action suggestion rules'),
    ('nextactions.view', 'View next-action suggestion rules and stats')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('super_admin', 'super_admin_role')
  AND p.key IN ('nextactions.manage', 'nextactions.view')
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- Notification type (in-app + push default on, email off — §35 seed style).
INSERT INTO notification_types (key, label, description, default_in_app, default_email, default_push, category) VALUES
    ('next_action.suggested', 'Next step suggestion', 'Personalized next-step suggestions based on your journey', true, false, true, 'system')
ON CONFLICT (key) DO NOTHING;

-- Seed rules (§122). payload.cta_url values are internal paths.
INSERT INTO next_action_rules
    (slug, name, description, enabled, priority, audience, conditions, payload, channels, completion_signal, frequency)
VALUES
    ('member-take-readiness-assessment', 'Take the readiness assessment',
     'New members who have not completed the readiness assessment',
     true, 10,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"account_age_days","lt":30},{"predicate":"assessment_completed","eq":false}]}',
     '{"title":"How ready are you to japa?","body":"Take the readiness assessment to get a personalised roadmap.","cta_label":"Take assessment","cta_url":"/tools/readiness-assessment","icon":"clipboard-check"}',
     '["popup","notification"]',
     '{"all":[{"predicate":"assessment_completed","eq":true}]}',
     '{"mode":"recurring","max_impressions":5,"cooldown_hours":24,"snooze_default_hours":168}'),

    ('canada-crs-calculator', 'Calculate your CRS score',
     'Canada-bound users who have not used the CRS calculator',
     true, 20,
     '{"user_class":["regular","pro","business","guest"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"destination_country","in":["CA"]},{"predicate":"tool_not_used","slug":"crs-calculator"}]}',
     '{"title":"Estimate your Express Entry score","body":"Use the CRS calculator to see where you stand.","cta_label":"Calculate score","cta_url":"/tools/crs-calculator","icon":"calculator"}',
     '["announcement_bar","sidebar","in_article"]',
     '{"all":[{"predicate":"tool_used","slug":"crs-calculator"}]}',
     '{"mode":"recurring","max_impressions":8,"cooldown_hours":72,"snooze_default_hours":168}'),

    ('guest-register-after-views', 'Create a free account',
     'Guests who have viewed several pieces of content',
     true, 30,
     '{"user_class":["guest"],"surfaces":["main","tools"]}',
     '{"all":[{"predicate":"event_count","type":"content.viewed","gte":3,"window":"7d"}]}',
     '{"title":"Join JapaConnect","body":"Create a free account to save your progress and get personalised guidance.","cta_label":"Sign up free","cta_url":"/register","icon":"user-plus"}',
     '["popup"]',
     NULL,
     '{"mode":"recurring","max_impressions":5,"cooldown_hours":48,"snooze_default_hours":168}'),

    ('guest-register-save-results', 'Save your results',
     'Guests who just used a tool',
     true, 40,
     '{"user_class":["guest"],"surfaces":["main","tools"]}',
     '{"all":[{"predicate":"tool_used","slug":"*"}]}',
     '{"title":"Keep your results","body":"Register to save your tool results and pick up where you left off.","cta_label":"Create account","cta_url":"/register","icon":"bookmark"}',
     '["announcement_bar","in_article"]',
     NULL,
     '{"mode":"recurring","max_impressions":4,"cooldown_hours":72,"snooze_default_hours":168}'),

    ('free-upgrade-gate-hit', 'Unlock with Pro',
     'Free users who hit a tier-gated feature in the last 7 days',
     true, 50,
     '{"user_class":["regular"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_gate_hit","within_days":7}]}',
     '{"title":"You hit a Pro feature","body":"Upgrade to unlock higher limits and member-only tools.","cta_label":"See plans","cta_url":"/pricing","icon":"zap"}',
     '["popup"]',
     '{"all":[{"predicate":"account_tier","in":["pro","business"]}]}',
     '{"mode":"recurring","max_impressions":6,"cooldown_hours":72,"snooze_default_hours":168}'),

    ('free-upgrade-mature', 'Upgrade your account',
     'Free users active for more than 14 days',
     true, 60,
     '{"user_class":["regular"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"account_age_days","gt":14}]}',
     '{"title":"Get more from JapaConnect","body":"Pro members unlock unlimited tools and priority features.","cta_label":"View plans","cta_url":"/pricing","icon":"star"}',
     '["announcement_bar","sidebar"]',
     '{"all":[{"predicate":"account_tier","in":["pro","business"]}]}',
     '{"mode":"recurring","max_impressions":8,"cooldown_hours":168,"snooze_default_hours":336}');

COMMIT;
