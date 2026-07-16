-- 000136_menus_email_templates_redirects.up.sql
-- Creates three new entities: menus (navigation menus with RBAC-gated items),
-- email_templates (tenant-customizable transactional emails with hardcoded fallback),
-- and redirects (admin-managed URL redirect rules with Valkey-cached resolution).
-- Ref: ADR-054

BEGIN;

-- ============================================================
-- 1. Menus — Navigation menus with hierarchical RBAC-gated items
-- ============================================================

CREATE TABLE menus (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    slug       text NOT NULL,
    name       text NOT NULL,
    placement  text NOT NULL DEFAULT 'sidebar' CHECK (placement IN ('sidebar', 'bottom_bar')),
    status     text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, slug)
);

CREATE TABLE menu_items (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    menu_id        uuid NOT NULL REFERENCES menus(id) ON DELETE CASCADE,
    parent_id      uuid REFERENCES menu_items(id) ON DELETE CASCADE,
    label          text NOT NULL,
    url            text NOT NULL,
    icon           text,
    sort_order     int NOT NULL DEFAULT 0,
    permission_node text,
    visibility     text NOT NULL DEFAULT 'all' CHECK (visibility IN ('all', 'authenticated', 'guest')),
    target         text NOT NULL DEFAULT '_self' CHECK (target IN ('_self', '_blank')),
    is_ad_slot     boolean NOT NULL DEFAULT false,
    ad_slot_config jsonb,
    status         text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_menu_items_menu   ON menu_items(tenant_id, menu_id, sort_order);
CREATE INDEX idx_menu_items_parent ON menu_items(parent_id);

-- Seed default menus for the default tenant.
INSERT INTO menus (tenant_id, slug, name, placement) VALUES
    ('00000000-0000-0000-0000-000000000001', 'main_sidebar', 'Main Sidebar', 'sidebar'),
    ('00000000-0000-0000-0000-000000000001', 'main_bottom_bar', 'Mobile Bottom Bar', 'bottom_bar')
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- Seed default guest menu items.
INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Explore', '/explore', 'compass', 0, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'main_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Learn', '/learn', 'graduation-cap', 1, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'main_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Blog', '/blog', 'newspaper', 2, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'main_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Login', '/login', 'log-in', 10, 'guest', '_self', 'active'
FROM menus m WHERE m.slug = 'main_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

-- Seed default authenticated menu items.
INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Home', '/', 'home', 0, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'main_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Notifications', '/notifications', 'bell', 1, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'main_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Messages', '/messages', 'mail', 2, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'main_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Bookmarks', '/bookmarks', 'bookmark', 3, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'main_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Profile', '/profile', 'user', 20, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'main_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

-- Seed bottom bar items.
INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Explore', '/explore', 'compass', 0, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'main_bottom_bar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Home', '/', 'home', 0, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'main_bottom_bar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Search', '/search', 'search', 1, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'main_bottom_bar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Notifications', '/notifications', 'bell', 2, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'main_bottom_bar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Profile', '/profile', 'user', 3, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'main_bottom_bar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- 2. Email Templates — Tenant-customizable transactional emails
-- ============================================================

CREATE TABLE email_templates (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    type            text NOT NULL,
    name            text NOT NULL,
    subject         text NOT NULL,
    html_body       text NOT NULL,
    plain_text_body text NOT NULL DEFAULT '',
    variables       jsonb NOT NULL DEFAULT '[]',
    status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, type)
);

-- Seed all 22 template types for the default tenant.
-- Each template uses Go html/template syntax.
-- Variables are documented per type for the admin UI; the actual hardcoded Go
-- constants in internal/email/html.go serve as the fallback when html_body is
-- empty or rendering fails.

-- Auth: Password Reset
INSERT INTO email_templates (tenant_id, type, name, subject, html_body, plain_text_body, variables) VALUES
('00000000-0000-0000-0000-000000000001', 'password_reset', 'Password Reset',
 'Reset your {{.SiteName}} password',
 '',
 'Click here to reset your password: {{.ActionURL}} (expires in {{.Expiry}})',
 '[{"name":"SiteName","description":"Brand name","required":true},{"name":"UserName","description":"Recipient display name","required":false},{"name":"ActionURL","description":"Password reset link","required":true},{"name":"Expiry","description":"Link expiry duration","required":true},{"name":"SupportURL","description":"Support contact URL","required":true}]'
);

-- Auth: Email Verification
INSERT INTO email_templates (tenant_id, type, name, subject, html_body, plain_text_body, variables) VALUES
('00000000-0000-0000-0000-000000000001', 'email_verification', 'Email Verification',
 'Verify your email for {{.SiteName}}',
 '',
 'Verify your email: {{.ActionURL}} (expires in {{.Expiry}})',
 '[{"name":"SiteName","description":"Brand name","required":true},{"name":"UserName","description":"Recipient display name","required":false},{"name":"ActionURL","description":"Verification link","required":true},{"name":"Expiry","description":"Link expiry duration","required":true}]'
);

-- Auth: Welcome
INSERT INTO email_templates (tenant_id, type, name, subject, html_body, plain_text_body, variables) VALUES
('00000000-0000-0000-0000-000000000001', 'welcome', 'Welcome Email',
 'Welcome to {{.SiteName}}, {{.UserName}}!',
 '',
 'Welcome to {{.SiteName}}, {{.UserName}}! Visit us at {{.SiteURL}}.',
 '[{"name":"SiteName","description":"Brand name","required":true},{"name":"UserName","description":"Recipient display name","required":true},{"name":"SiteURL","description":"Site home URL","required":true}]'
);

-- Auth: Login Alert
INSERT INTO email_templates (tenant_id, type, name, subject, html_body, plain_text_body, variables) VALUES
('00000000-0000-0000-0000-000000000001', 'login_alert', 'New Login Alert',
 'New sign-in to your {{.SiteName}} account',
 '',
 'A new sign-in to your {{.SiteName}} account was detected.\nDevice: {{.DeviceInfo}}\nLocation: {{.Location}}\nTime: {{.Timestamp}}\n\nIf this was not you, please reset your password immediately.',
 '[{"name":"SiteName","description":"Brand name","required":true},{"name":"UserName","description":"Recipient display name","required":true},{"name":"DeviceInfo","description":"Browser/device info","required":true},{"name":"Location","description":"Geographic location","required":true},{"name":"Timestamp","description":"Login timestamp","required":true},{"name":"SupportURL","description":"Support contact URL","required":true}]'
);

-- Auth: Account Deletion Confirmed
INSERT INTO email_templates (tenant_id, type, name, subject, html_body, plain_text_body, variables) VALUES
('00000000-0000-0000-0000-000000000001', 'account_deletion_confirmed', 'Account Deletion Confirmed',
 'Your {{.SiteName}} account has been deleted',
 '',
 'Your {{.SiteName}} account has been permanently deleted. If this was a mistake, please contact support at {{.SupportURL}}.',
 '[{"name":"SiteName","description":"Brand name","required":true},{"name":"UserName","description":"Recipient display name","required":true},{"name":"SupportURL","description":"Support contact URL","required":true}]'
);

-- Auth: Email Changed
INSERT INTO email_templates (tenant_id, type, name, subject, html_body, plain_text_body, variables) VALUES
('00000000-0000-0000-0000-000000000001', 'email_changed', 'Email Address Changed',
 'Your {{.SiteName}} email address has been updated',
 '',
 'Your {{.SiteName}} account email has been changed. If you did not make this change, please contact support immediately at {{.SupportURL}}.',
 '[{"name":"SiteName","description":"Brand name","required":true},{"name":"UserName","description":"Recipient display name","required":true},{"name":"SupportURL","description":"Support contact URL","required":true}]'
);

-- Payments: Receipt
INSERT INTO email_templates (tenant_id, type, name, subject, html_body, plain_text_body, variables) VALUES
('00000000-0000-0000-0000-000000000001', 'payment_receipt', 'Payment Receipt',
 'Receipt from {{.SiteName}} — {{.Amount}} {{.Currency}}',
 '',
 'Payment receipt from {{.SiteName}}\n\nAmount: {{.Amount}} {{.Currency}}\nDescription: {{.Description}}\nTransaction ID: {{.TransactionID}}\nPayment method: {{.PaymentMethod}}\n\nThank you!',
 '[{"name":"SiteName","description":"Brand name","required":true},{"name":"UserName","description":"Recipient display name","required":true},{"name":"Amount","description":"Payment amount","required":true},{"name":"Currency","description":"Currency code","required":true},{"name":"Description","description":"Payment description","required":true},{"name":"TransactionID","description":"Transaction reference","required":true},{"name":"PaymentMethod","description":"Payment method used","required":true}]'
);

-- Payments: Failed
INSERT INTO email_templates (tenant_id, type, name, subject, html_body, plain_text_body, variables) VALUES
('00000000-0000-0000-0000-000000000001', 'payment_failed', 'Payment Failed',
 'Payment to {{.SiteName}} was unsuccessful',
 '',
 'Your payment of {{.Amount}} {{.Currency}} to {{.SiteName}} was unsuccessful.\nReason: {{.FailureReason}}\n\nPlease update your payment method or try again.',
 '[{"name":"SiteName","description":"Brand name","required":true},{"name":"UserName","description":"Recipient display name","required":true},{"name":"Amount","description":"Payment amount","required":true},{"name":"Currency","description":"Currency code","required":true},{"name":"FailureReason","description":"Reason for failure","required":true}]'
);

-- Payments: Subscription Created
INSERT INTO email_templates (tenant_id, type, name, subject, html_body, plain_text_body, variables) VALUES
('00000000-0000-0000-0000-000000000001', 'subscription_created', 'Subscription Started',
 'Your {{.SiteName}} subscription is active',
 '',
 'Your {{.PlanName}} subscription is now active. Thank you for subscribing!\n\nNext billing date: {{.NextBillingDate}}',
 '[{"name":"SiteName","description":"Brand name","required":true},{"name":"UserName","description":"Recipient display name","required":true},{"name":"PlanName","description":"Subscription plan name","required":true},{"name":"NextBillingDate","description":"Next billing date","required":true},{"name":"SiteURL","description":"Site URL","required":true}]'
);

-- Payments: Subscription Cancelled
INSERT INTO email_templates (tenant_id, type, name, subject, html_body, plain_text_body, variables) VALUES
('00000000-0000-0000-0000-000000000001', 'subscription_cancelled', 'Subscription Cancelled',
 'Your {{.SiteName}} subscription has been cancelled',
 '',
 'Your {{.SiteName}} {{.PlanName}} subscription has been cancelled. You will have access until {{.AccessUntil}}.',
 '[{"name":"SiteName","description":"Brand name","required":true},{"name":"UserName","description":"Recipient display name","required":true},{"name":"PlanName","description":"Subscription plan name","required":true},{"name":"AccessUntil","description":"Access end date","required":true},{"name":"SiteURL","description":"Site URL","required":true}]'
);

-- Payments: Subscription Expiring
INSERT INTO email_templates (tenant_id, type, name, subject, html_body, plain_text_body, variables) VALUES
('00000000-0000-0000-0000-000000000001', 'subscription_expiring', 'Subscription Expiring Soon',
 'Your {{.SiteName}} subscription expires soon',
 '',
 'Your {{.SiteName}} {{.PlanName}} subscription will expire on {{.ExpiryDate}}. Renew to maintain access.',
 '[{"name":"SiteName","description":"Brand name","required":true},{"name":"UserName","description":"Recipient display name","required":true},{"name":"PlanName","description":"Subscription plan name","required":true},{"name":"ExpiryDate","description":"Expiry date","required":true},{"name":"SiteURL","description":"Renewal URL","required":true}]'
);

-- Learning: Certificate Issued
INSERT INTO email_templates (tenant_id, type, name, subject, html_body, plain_text_body, variables) VALUES
('00000000-0000-0000-0000-000000000001', 'certificate_issued', 'Certificate Issued',
 'Your certificate for {{.CourseName}} is ready!',
 '',
 'Congratulations, {{.UserName}}! Your certificate for {{.CourseName}} is ready.\n\nView it here: {{.CertificateURL}}',
 '[{"name":"SiteName","description":"Brand name","required":true},{"name":"UserName","description":"Recipient display name","required":true},{"name":"CourseName","description":"Course or track name","required":true},{"name":"CertificateURL","description":"Certificate download URL","required":true}]'
);

-- Learning: Course Completed
INSERT INTO email_templates (tenant_id, type, name, subject, html_body, plain_text_body, variables) VALUES
('00000000-0000-0000-0000-000000000001', 'course_completed', 'Course Completed',
 'You completed {{.CourseName}}!',
 '',
 'Congratulations, {{.UserName}}! You have completed {{.CourseName}}.',
 '[{"name":"SiteName","description":"Brand name","required":true},{"name":"UserName","description":"Recipient display name","required":true},{"name":"CourseName","description":"Course or track name","required":true},{"name":"SiteURL","description":"Site URL","required":true}]'
);

-- Support: Ticket Confirmation
INSERT INTO email_templates (tenant_id, type, name, subject, html_body, plain_text_body, variables) VALUES
('00000000-0000-0000-0000-000000000001', 'ticket_confirmation', 'Support Ticket Confirmation',
 'Ticket #{{.TicketID}} received — {{.Subject}}',
 '',
 'Your support ticket has been received.\n\nTicket ID: {{.TicketID}}\nSubject: {{.Subject}}\n\nWe will respond shortly.',
 '[{"name":"SiteName","description":"Brand name","required":true},{"name":"UserName","description":"Recipient display name","required":true},{"name":"TicketID","description":"Ticket identifier","required":true},{"name":"Subject","description":"Ticket subject","required":true}]'
);

-- Support: Ticket Status Update
INSERT INTO email_templates (tenant_id, type, name, subject, html_body, plain_text_body, variables) VALUES
('00000000-0000-0000-0000-000000000001', 'ticket_status_update', 'Ticket Status Updated',
 'Ticket #{{.TicketID}} status updated to {{.NewStatus}}',
 '',
 'Your ticket #{{.TicketID}} has been updated:\n\nSubject: {{.Subject}}\nStatus: {{.OldStatus}} → {{.NewStatus}}\n\n{{.Message}}',
 '[{"name":"SiteName","description":"Brand name","required":true},{"name":"UserName","description":"Recipient display name","required":true},{"name":"TicketID","description":"Ticket identifier","required":true},{"name":"Subject","description":"Ticket subject","required":true},{"name":"OldStatus","description":"Previous status","required":true},{"name":"NewStatus","description":"New status","required":true},{"name":"Message","description":"Status change message","required":false}]'
);

-- Support: Ticket Response
INSERT INTO email_templates (tenant_id, type, name, subject, html_body, plain_text_body, variables) VALUES
('00000000-0000-0000-0000-000000000001', 'ticket_response', 'New Response to Your Ticket',
 'New response on ticket #{{.TicketID}}',
 '',
 'A staff member has responded to your ticket #{{.TicketID}}.\n\n{{.Message}}',
 '[{"name":"SiteName","description":"Brand name","required":true},{"name":"UserName","description":"Recipient display name","required":true},{"name":"TicketID","description":"Ticket identifier","required":true},{"name":"Subject","description":"Ticket subject","required":true},{"name":"Message","description":"Response content","required":true}]'
);

-- Digest: Daily
INSERT INTO email_templates (tenant_id, type, name, subject, html_body, plain_text_body, variables) VALUES
('00000000-0000-0000-0000-000000000001', 'daily_digest', 'Daily Digest',
 'Your daily {{.SiteName}} digest',
 '',
 'Your daily {{.SiteName}} digest\n\n{{.DigestContent}}',
 '[{"name":"SiteName","description":"Brand name","required":true},{"name":"UserName","description":"Recipient display name","required":true},{"name":"DigestContent","description":"Rendered digest items","required":true},{"name":"SiteURL","description":"Site URL","required":true}]'
);

-- Digest: Weekly
INSERT INTO email_templates (tenant_id, type, name, subject, html_body, plain_text_body, variables) VALUES
('00000000-0000-0000-0000-000000000001', 'weekly_digest', 'Weekly Digest',
 'Your weekly {{.SiteName}} digest',
 '',
 'Your weekly {{.SiteName}} digest\n\n{{.DigestContent}}',
 '[{"name":"SiteName","description":"Brand name","required":true},{"name":"UserName","description":"Recipient display name","required":true},{"name":"DigestContent","description":"Rendered digest items","required":true},{"name":"SiteURL","description":"Site URL","required":true}]'
);

-- Admin: New User
INSERT INTO email_templates (tenant_id, type, name, subject, html_body, plain_text_body, variables) VALUES
('00000000-0000-0000-0000-000000000001', 'admin_new_user', 'New User Registration',
 '[{{.SiteName}}] New user registered: {{.NewUsername}}',
 '',
 'A new user has registered on {{.SiteName}}:\n\nUsername: {{.NewUsername}}\nEmail: {{.NewEmail}}\nRegistered at: {{.Timestamp}}',
 '[{"name":"SiteName","description":"Brand name","required":true},{"name":"NewUsername","description":"New user username","required":true},{"name":"NewEmail","description":"New user email","required":true},{"name":"Timestamp","description":"Registration time","required":true}]'
);

-- Admin: Content Reported
INSERT INTO email_templates (tenant_id, type, name, subject, html_body, plain_text_body, variables) VALUES
('00000000-0000-0000-0000-000000000001', 'admin_content_reported', 'Content Reported',
 '[{{.SiteName}}] Content reported by {{.ReporterUsername}}',
 '',
 'Content has been reported on {{.SiteName}}:\n\nReported by: {{.ReporterUsername}}\nReason: {{.Reason}}\nContent URL: {{.ContentURL}}',
 '[{"name":"SiteName","description":"Brand name","required":true},{"name":"ReporterUsername","description":"User who reported","required":true},{"name":"Reason","description":"Report reason","required":true},{"name":"ContentURL","description":"Reported content URL","required":true}]'
);

-- Admin: Appeal Submitted
INSERT INTO email_templates (tenant_id, type, name, subject, html_body, plain_text_body, variables) VALUES
('00000000-0000-0000-0000-000000000001', 'admin_appeal_submitted', 'Moderation Appeal Submitted',
 '[{{.SiteName}}] Moderation appeal from {{.AppellantUsername}}',
 '',
 'A user has appealed a moderation decision:\n\nUser: {{.AppellantUsername}}\nAction appealed: {{.ActionType}}\nReason: {{.Reason}}',
 '[{"name":"SiteName","description":"Brand name","required":true},{"name":"AppellantUsername","description":"Appellant username","required":true},{"name":"ActionType","description":"Moderation action appealed","required":true},{"name":"Reason","description":"Appeal reason","required":true}]'
);

-- Marketing: Newsletter (placeholder — actual subject/HTML from campaign, not template)
INSERT INTO email_templates (tenant_id, type, name, subject, html_body, plain_text_body, variables) VALUES
('00000000-0000-0000-0000-000000000001', 'newsletter', 'Newsletter',
 '{{.Subject}}',
 '',
 '{{.Content}}',
 '[{"name":"SiteName","description":"Brand name","required":true},{"name":"Subject","description":"Campaign subject","required":true},{"name":"Content","description":"Campaign HTML content","required":true}]'
);

-- ============================================================
-- 3. Redirects — Admin-managed URL redirect rules
-- ============================================================

CREATE TABLE redirects (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    source_path text NOT NULL,
    target_url  text NOT NULL,
    http_status smallint NOT NULL DEFAULT 301 CHECK (http_status IN (301, 302, 307)),
    is_active   boolean NOT NULL DEFAULT true,
    notes       text,
    hit_count   int NOT NULL DEFAULT 0,
    last_hit_at timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, source_path)
);

CREATE INDEX idx_redirects_source ON redirects(tenant_id, source_path) WHERE is_active = true;
CREATE INDEX idx_redirects_status  ON redirects(tenant_id, is_active);

COMMIT;
