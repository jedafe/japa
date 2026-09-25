-- Internationalization (i18n) translation system

CREATE TABLE translations (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    locale     varchar(10) NOT NULL,
    "key"      varchar(255) NOT NULL,
    value      text NOT NULL,
    context    varchar(50),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, locale, "key")
);

CREATE INDEX idx_translations_tenant_locale ON translations(tenant_id, locale);
CREATE INDEX idx_translations_tenant_key ON translations(tenant_id, "key");
