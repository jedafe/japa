-- 000735_baseline_schema.up.sql
-- Squashed schema baseline (regenerated 2026-09-25 by scripts/gen_baseline.py).
-- Replaces the historical migration chain (archived in migrations/archive/).
-- Pair with 000735_baseline_seed.up.sql, which loads the platform reference/seed data.

--
-- PostgreSQL database dump
--


-- Dumped from database version 15.17 (Homebrew)
-- Dumped by pg_dump version 15.17 (Homebrew)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: pg_stat_statements; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS pg_stat_statements WITH SCHEMA public;


--
-- Name: EXTENSION pg_stat_statements; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON EXTENSION pg_stat_statements IS 'track planning and execution statistics of all SQL statements executed';


--
-- Name: pg_trgm; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;


--
-- Name: EXTENSION pg_trgm; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON EXTENSION pg_trgm IS 'text similarity measurement and index searching based on trigrams';


--
-- Name: pgcrypto; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;


--
-- Name: EXTENSION pgcrypto; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON EXTENSION pgcrypto IS 'cryptographic functions';


--
-- Name: uuid-ossp; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA public;


--
-- Name: EXTENSION "uuid-ossp"; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON EXTENSION "uuid-ossp" IS 'generate universally unique identifiers (UUIDs)';


--
-- Name: vector; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;


--
-- Name: EXTENSION vector; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON EXTENSION vector IS 'vector data type and ivfflat and hnsw access methods';


--
-- Name: ban_operation; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.ban_operation AS ENUM (
    'post',
    'comment',
    'discussion',
    'blog',
    'message'
);


--
-- Name: date_utc(timestamp with time zone); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.date_utc(ts timestamp with time zone) RETURNS date
    LANGUAGE sql IMMUTABLE
    AS $$
    SELECT (ts AT TIME ZONE 'UTC')::date
$$;


--
-- Name: fn_tags_content_count_sync(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.fn_tags_content_count_sync() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        UPDATE tags SET content_count = content_count + 1 WHERE id = NEW.tag_id;
        RETURN NEW;
    ELSIF TG_OP = 'DELETE' THEN
        UPDATE tags SET content_count = content_count - 1 WHERE id = OLD.tag_id;
        RETURN OLD;
    END IF;
    RETURN NULL;
END;
$$;


--
-- Name: increment_reply_view_count(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.increment_reply_view_count() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    UPDATE forum_replies SET view_count = view_count + 1 WHERE id = NEW.reply_id;
    RETURN NEW;
END;
$$;


--
-- Name: trg_comment_views_increment(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.trg_comment_views_increment() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    UPDATE comments SET view_count = view_count + 1 WHERE id = NEW.comment_id;
    RETURN NEW;
END;
$$;


--
-- Name: trg_intel_records_post_count_sync(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.trg_intel_records_post_count_sync() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.status = 'published' THEN
            UPDATE intel_sources
            SET post_count = post_count + 1
            WHERE id = NEW.source_id;
        END IF;
        RETURN NEW;

    ELSIF TG_OP = 'UPDATE' THEN
        IF OLD.status IS DISTINCT FROM NEW.status THEN
            IF NEW.status = 'published' AND OLD.status IS DISTINCT FROM 'published' THEN
                UPDATE intel_sources
                SET post_count = post_count + 1
                WHERE id = NEW.source_id;
            ELSIF OLD.status = 'published' AND NEW.status IS DISTINCT FROM 'published' THEN
                UPDATE intel_sources
                SET post_count = GREATEST(0, post_count - 1)
                WHERE id = NEW.source_id;
            END IF;
        END IF;
        RETURN NEW;

    ELSIF TG_OP = 'DELETE' THEN
        IF OLD.status = 'published' THEN
            UPDATE intel_sources
            SET post_count = GREATEST(0, post_count - 1)
            WHERE id = OLD.source_id;
        END IF;
        RETURN OLD;
    END IF;

    RETURN NULL;
END;
$$;


--
-- Name: trg_soft_delete_forum_thread_content(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.trg_soft_delete_forum_thread_content() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF OLD.content_id IS NOT NULL THEN
        UPDATE contents
        SET deleted_at = now(),
            updated_at = now(),
            status = 'archived'
        WHERE id = OLD.content_id
          AND deleted_at IS NULL;
    END IF;
    RETURN OLD;
END;
$$;


--
-- Name: trg_update_active_mentee_count(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.trg_update_active_mentee_count() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  -- On DELETE, use OLD.mentor_id; on INSERT/UPDATE, use NEW.mentor_id.
  -- COALESCE handles both cases: NEW is NULL on DELETE, OLD is NULL on INSERT.
  UPDATE user_profiles
  SET active_mentee_count = (
    SELECT COUNT(*) FROM mentorship_connections
    WHERE mentor_id = COALESCE(NEW.mentor_id, OLD.mentor_id)
    AND status = 'active'
  )
  WHERE user_id = COALESCE(NEW.mentor_id, OLD.mentor_id);
  RETURN COALESCE(NEW, OLD);
END;
$$;


--
-- Name: update_ad_networks_updated_at(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.update_ad_networks_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$;


--
-- Name: week_start_utc(timestamp with time zone); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.week_start_utc(ts timestamp with time zone) RETURNS date
    LANGUAGE sql IMMUTABLE
    AS $$
    SELECT ((ts AT TIME ZONE 'UTC')::date - ((EXTRACT(dow FROM (ts AT TIME ZONE 'UTC')::date)::int + 6) % 7))
$$;


SET default_table_access_method = heap;

--
-- Name: account_deletion_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.account_deletion_requests (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    reason text,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    progress_step integer DEFAULT 0 NOT NULL
);


--
-- Name: achievement_meta; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.achievement_meta (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    content_id uuid NOT NULL,
    milestone_type text NOT NULL,
    destination_space_id uuid NOT NULL,
    origin_space_id uuid,
    is_anonymous boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: COLUMN achievement_meta.milestone_type; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.achievement_meta.milestone_type IS 'visa_approved|job_offer|university_accepted|arrived|one_year|citizenship|other';


--
-- Name: activity_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.activity_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    activity_key text NOT NULL,
    reference text NOT NULL,
    payload jsonb DEFAULT '{}'::jsonb,
    processed boolean DEFAULT false,
    processed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: activity_point_rules; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.activity_point_rules (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    activity_key text NOT NULL,
    point_value integer NOT NULL,
    is_enabled boolean DEFAULT true,
    cap_per_day integer,
    cap_per_week integer,
    description text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    one_time boolean DEFAULT false NOT NULL
);


--
-- Name: ad_analytics_daily; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ad_analytics_daily (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    campaign_id uuid NOT NULL,
    slot_type_id uuid NOT NULL,
    date date NOT NULL,
    impressions integer DEFAULT 0 NOT NULL,
    clicks integer DEFAULT 0 NOT NULL,
    spent bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: ad_campaigns; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ad_campaigns (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    slot_type_id uuid NOT NULL,
    content jsonb DEFAULT '{}'::jsonb NOT NULL,
    target_url text,
    budget bigint,
    spent bigint DEFAULT 0 NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    priority integer DEFAULT 0 NOT NULL,
    frequency_cap_per_user integer,
    start_date timestamp with time zone,
    end_date timestamp with time zone,
    created_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    target_geo text[] DEFAULT '{}'::text[],
    target_space_ids uuid[] DEFAULT '{}'::uuid[],
    organization_id uuid,
    flat_rate integer DEFAULT 0 NOT NULL,
    budget_credits integer DEFAULT 0 NOT NULL
);


--
-- Name: COLUMN ad_campaigns.target_geo; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.ad_campaigns.target_geo IS 'ISO country codes for geo-targeted serving (e.g., {NG,CA,GB}). Empty = all countries.';


--
-- Name: COLUMN ad_campaigns.target_space_ids; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.ad_campaigns.target_space_ids IS 'Specific space UUIDs for fine-grained targeting. Empty = all spaces.';


--
-- Name: ad_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ad_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    campaign_id uuid NOT NULL,
    user_id uuid,
    event_type text NOT NULL,
    occurred_at timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ad_events_event_type_check CHECK ((event_type = ANY (ARRAY['impression'::text, 'click'::text])))
);


--
-- Name: COLUMN ad_events.user_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.ad_events.user_id IS 'The user who triggered the event. NULL for anonymous/guest events (public /ads/events endpoint).';


--
-- Name: ad_networks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ad_networks (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    provider text NOT NULL,
    name text NOT NULL,
    client_id text,
    api_key_encrypted text,
    slot_mapping jsonb DEFAULT '{}'::jsonb,
    config jsonb DEFAULT '{}'::jsonb,
    is_active boolean DEFAULT false NOT NULL,
    priority integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ad_networks_provider_check CHECK ((provider = ANY (ARRAY['adsense'::text, 'ezoic'::text, 'media_net'::text, 'custom'::text])))
);


--
-- Name: ad_slot_types; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ad_slot_types (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    slot_name text NOT NULL,
    description text,
    max_display_per_page integer DEFAULT 1 NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    flat_rate integer DEFAULT 100 NOT NULL
);


--
-- Name: admin_broadcasts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.admin_broadcasts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    title text NOT NULL,
    body text NOT NULL,
    sent_by uuid,
    sent_at timestamp with time zone DEFAULT now() NOT NULL,
    recipient_count integer DEFAULT 0 NOT NULL
);


--
-- Name: ads_campaigns; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ads_campaigns (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    advertiser_id uuid NOT NULL,
    name text NOT NULL,
    placement text NOT NULL,
    budget_credits integer DEFAULT 0 NOT NULL,
    spent_credits integer DEFAULT 0 NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    start_date timestamp with time zone,
    end_date timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: affiliate_content_commissions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.affiliate_content_commissions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    content_id uuid NOT NULL,
    provider_id uuid NOT NULL,
    author_user_id uuid NOT NULL,
    external_commission numeric(12,2) NOT NULL,
    author_share numeric(12,2) NOT NULL,
    author_commission_pct numeric(5,2) NOT NULL,
    event_description text,
    credited boolean DEFAULT false NOT NULL,
    credited_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: TABLE affiliate_content_commissions; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.affiliate_content_commissions IS 'Tracks external affiliate commissions earned from content with affiliate links, and the author revenue share.';


--
-- Name: affiliate_link_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.affiliate_link_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    link_id uuid NOT NULL,
    event_type text NOT NULL,
    referrer_url text,
    ip_hash text,
    user_agent_hash text,
    converted_payment_id uuid,
    commission_amount numeric(12,2),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT affiliate_link_events_event_type_check CHECK ((event_type = ANY (ARRAY['click'::text, 'conversion'::text])))
);


--
-- Name: affiliate_program_members; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.affiliate_program_members (
    user_id uuid NOT NULL,
    enrolled_at timestamp with time zone DEFAULT now() NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    total_clicks integer DEFAULT 0 NOT NULL,
    total_conversions integer DEFAULT 0 NOT NULL,
    total_earned_credits numeric(12,2) DEFAULT 0 NOT NULL,
    payout_method text,
    notes text,
    CONSTRAINT affiliate_program_members_status_check CHECK ((status = ANY (ARRAY['active'::text, 'suspended'::text, 'banned'::text])))
);


--
-- Name: affiliate_program_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.affiliate_program_settings (
    commission_rates jsonb DEFAULT '{}'::jsonb NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: affiliate_providers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.affiliate_providers (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    slug text NOT NULL,
    description text,
    provider_type text DEFAULT 'affiliate'::text NOT NULL,
    logo_media_id uuid,
    base_url text,
    tracking_param text,
    tracking_value text,
    commission_rate numeric(5,2),
    is_active boolean DEFAULT true NOT NULL,
    is_featured boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    author_commission_pct numeric(5,2) DEFAULT 0 NOT NULL,
    CONSTRAINT affiliate_providers_provider_type_check CHECK ((provider_type = ANY (ARRAY['affiliate'::text, 'coupon'::text, 'both'::text])))
);


--
-- Name: COLUMN affiliate_providers.author_commission_pct; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.affiliate_providers.author_commission_pct IS 'Percentage of external affiliate commission shared with the content author who embedded the link. 0 = no sharing.';


--
-- Name: ai_agent_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ai_agent_sessions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    prompt text NOT NULL,
    geo_space_id uuid,
    presearch_level text DEFAULT 'none'::text NOT NULL,
    status text DEFAULT 'running'::text NOT NULL,
    tool_call_count integer DEFAULT 0 NOT NULL,
    result_summary text,
    error_message text,
    total_tokens integer DEFAULT 0 NOT NULL,
    estimated_cost numeric(10,4) DEFAULT 0 NOT NULL,
    started_at timestamp with time zone DEFAULT now() NOT NULL,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ai_agent_sessions_presearch_level_check CHECK ((presearch_level = ANY (ARRAY['full'::text, 'partial'::text, 'none'::text]))),
    CONSTRAINT ai_agent_sessions_status_check CHECK ((status = ANY (ARRAY['running'::text, 'completed'::text, 'failed'::text, 'cancelled'::text])))
);


--
-- Name: ai_agent_tool_calls; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ai_agent_tool_calls (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    session_id uuid NOT NULL,
    tool_name text NOT NULL,
    arguments jsonb DEFAULT '{}'::jsonb NOT NULL,
    result jsonb,
    status text DEFAULT 'pending'::text NOT NULL,
    error_message text,
    latency_ms integer,
    rbac_checked boolean DEFAULT true NOT NULL,
    privacy_checked boolean DEFAULT true NOT NULL,
    hitl_review_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ai_agent_tool_calls_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'success'::text, 'error'::text, 'unauthorized'::text, 'privacy_blocked'::text, 'hitl_queued'::text])))
);


--
-- Name: ai_hitl_reviews; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ai_hitl_reviews (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    content_id uuid,
    content_type text NOT NULL,
    ai_output jsonb NOT NULL,
    human_feedback jsonb,
    status text DEFAULT 'pending'::text NOT NULL,
    reviewer_id uuid,
    reason text,
    reviewed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    source text DEFAULT 'manual'::text NOT NULL,
    target_table text DEFAULT 'contents'::text NOT NULL,
    CONSTRAINT ai_hitl_reviews_source_check CHECK ((source = ANY (ARRAY['manual'::text, 'agent_draft_request'::text, 'agent_tool'::text, 'studio'::text])))
);


--
-- Name: ai_personas; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ai_personas (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    profile_id uuid,
    disclosure_mode text DEFAULT 'disclosed'::text NOT NULL,
    autonomy_mode text DEFAULT 'queued'::text NOT NULL,
    voice_prompt text,
    activity_policy jsonb DEFAULT '{}'::jsonb NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    voice_id uuid,
    CONSTRAINT ai_personas_autonomy_mode_check CHECK ((autonomy_mode = ANY (ARRAY['autonomous'::text, 'queued'::text]))),
    CONSTRAINT ai_personas_disclosure_mode_check CHECK ((disclosure_mode = ANY (ARRAY['disclosed'::text, 'undisclosed'::text]))),
    CONSTRAINT ai_personas_status_check CHECK ((status = ANY (ARRAY['active'::text, 'paused'::text, 'retired'::text])))
);


--
-- Name: ai_providers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ai_providers (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    provider_type text NOT NULL,
    api_key_encrypted text DEFAULT ''::text NOT NULL,
    base_url text DEFAULT ''::text NOT NULL,
    model text DEFAULT ''::text NOT NULL,
    categories jsonb DEFAULT '[]'::jsonb NOT NULL,
    priority integer DEFAULT 0 NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ai_providers_provider_type_check CHECK ((provider_type = ANY (ARRAY['xiaomi'::text, 'openai'::text, 'groq'::text, 'ollama'::text, 'anthropic'::text, 'devin'::text, 'cursor'::text, 'perplexity'::text, 'custom'::text])))
);


--
-- Name: ai_tool_usage; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ai_tool_usage (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tool_id uuid NOT NULL,
    user_id uuid,
    tokens_used integer DEFAULT 0 NOT NULL,
    cost_cents integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: ai_tools; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ai_tools (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    category text NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    is_enabled boolean DEFAULT true NOT NULL,
    required_permission text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: ai_usage_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ai_usage_log (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid,
    provider text NOT NULL,
    model text NOT NULL,
    task_type text NOT NULL,
    prompt_tokens integer DEFAULT 0 NOT NULL,
    completion_tokens integer DEFAULT 0 NOT NULL,
    total_tokens integer DEFAULT 0 NOT NULL,
    cost_usd numeric(12,6) DEFAULT 0 NOT NULL,
    request_ref text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: api_key_ip_allowlist; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.api_key_ip_allowlist (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    api_key_id uuid NOT NULL,
    cidr inet NOT NULL,
    description text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: api_keys; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.api_keys (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    name text NOT NULL,
    key_prefix text NOT NULL,
    key_hash text NOT NULL,
    scopes text[] DEFAULT '{api.read.*}'::text[] NOT NULL,
    rate_limit_tier text DEFAULT 'basic'::text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    last_used_at timestamp with time zone,
    request_count bigint DEFAULT 0 NOT NULL,
    expires_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    environment text DEFAULT 'live'::text NOT NULL,
    revoked_at timestamp with time zone,
    revoked_by uuid,
    notes text
);


--
-- Name: appointment_alerts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.appointment_alerts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    destination_space_id uuid NOT NULL,
    embassy_slug text NOT NULL,
    visa_category text,
    date_range_start date NOT NULL,
    date_range_end date NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    last_checked_at timestamp with time zone,
    fired_at timestamp with time zone,
    expires_at date,
    notification_channels text[] DEFAULT '{in_app,email}'::text[] NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);


--
-- Name: arrival_checklist_guides; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.arrival_checklist_guides (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    destination_space_id uuid NOT NULL,
    visa_category text,
    applicant_type text DEFAULT 'individual'::text NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    status text DEFAULT 'published'::text NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    ai_generated boolean DEFAULT false NOT NULL,
    ai_model text,
    ai_prompt_hash text,
    hitl_reviewed_by uuid,
    hitl_reviewed_at timestamp with time zone,
    hitl_review_notes text,
    is_japabot_indexed boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: arrival_checklist_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.arrival_checklist_items (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    guide_id uuid NOT NULL,
    phase text NOT NULL,
    title text NOT NULL,
    description text,
    official_link text,
    origin_country_modifier jsonb,
    sort_order integer DEFAULT 0 NOT NULL,
    is_flagged boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: assessment_results; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.assessment_results (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid,
    submission_id uuid NOT NULL,
    total_score integer NOT NULL,
    dimension_scores jsonb DEFAULT '{}'::jsonb NOT NULL,
    threshold_label text NOT NULL,
    roadmap_content jsonb DEFAULT '[]'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: attestation_community_updates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.attestation_community_updates (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    pathway_id uuid NOT NULL,
    user_id uuid NOT NULL,
    step_index integer,
    fee_estimate text,
    turnaround_days integer,
    note text,
    status text DEFAULT 'pending'::text NOT NULL,
    reviewed_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);


--
-- Name: attestation_pathways; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.attestation_pathways (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    document_type text NOT NULL,
    issuing_space_id uuid NOT NULL,
    destination_space_id uuid NOT NULL,
    apostille_required boolean NOT NULL,
    pathway_steps jsonb NOT NULL,
    community_last_verified date,
    status text DEFAULT 'published'::text NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    ai_generated boolean DEFAULT false NOT NULL,
    ai_model text,
    ai_prompt_hash text,
    hitl_reviewed_by uuid,
    hitl_reviewed_at timestamp with time zone,
    hitl_review_notes text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    is_japabot_indexed boolean GENERATED ALWAYS AS (((status = 'published'::text) AND (is_active = true))) STORED
);


--
-- Name: audit_logs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.audit_logs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    actor_user_id uuid,
    impersonated_user_id uuid,
    action_type text NOT NULL,
    target_type text,
    target_id uuid,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    ip_address inet,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: badge_definitions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.badge_definitions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    slug text NOT NULL,
    name text NOT NULL,
    description text,
    icon_media_id uuid,
    trigger_type text NOT NULL,
    trigger_value integer,
    trigger_activity_key text,
    is_active boolean DEFAULT true NOT NULL,
    is_system boolean DEFAULT false NOT NULL,
    display_order integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT badge_definitions_trigger_type_check CHECK ((trigger_type = ANY (ARRAY['lifetime_points'::text, 'activity_count'::text, 'manual'::text])))
);


--
-- Name: blog_series; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.blog_series (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    title text NOT NULL,
    slug text NOT NULL,
    description text,
    cover_image_id uuid,
    author_user_id uuid NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    CONSTRAINT blog_series_status_check CHECK ((status = ANY (ARRAY['active'::text, 'archived'::text])))
);


--
-- Name: blog_series_posts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.blog_series_posts (
    series_id uuid NOT NULL,
    content_id uuid NOT NULL,
    "position" integer DEFAULT 0 NOT NULL
);


--
-- Name: booking_slots; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.booking_slots (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    service_id uuid NOT NULL,
    start_time timestamp with time zone NOT NULL,
    end_time timestamp with time zone NOT NULL,
    is_available boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: bookings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.bookings (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    service_id uuid NOT NULL,
    booker_id uuid NOT NULL,
    slot_start timestamp with time zone NOT NULL,
    slot_end timestamp with time zone NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    deposit_paid boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: bookmark_folders; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.bookmark_folders (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    name text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    "position" integer DEFAULT 0 NOT NULL
);


--
-- Name: bookmarks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.bookmarks (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    entity_type text NOT NULL,
    entity_id uuid NOT NULL,
    folder text DEFAULT 'default'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    folder_id uuid
);


--
-- Name: calendar_entries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.calendar_entries (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    title text NOT NULL,
    description text,
    category text DEFAULT 'other'::text NOT NULL,
    destination_space_id uuid,
    origin_space_id uuid,
    occurs_at timestamp with time zone NOT NULL,
    ends_at timestamp with time zone,
    is_recurring boolean DEFAULT false NOT NULL,
    recurrence_pattern text,
    source_url text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    submitted_by uuid,
    reviewed_by uuid,
    reviewed_at timestamp with time zone,
    staleness_check_due timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    ai_generated boolean DEFAULT false NOT NULL,
    ai_model text,
    hitl_reviewed_by uuid,
    ai_generated_at timestamp with time zone,
    ai_prompt_hash text,
    hitl_reviewed_at timestamp with time zone,
    hitl_review_notes text
);


--
-- Name: calendar_saves; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.calendar_saves (
    user_id uuid NOT NULL,
    entry_id uuid NOT NULL,
    saved_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: calendar_subscriptions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.calendar_subscriptions (
    user_id uuid NOT NULL,
    destination_space_id uuid NOT NULL,
    category text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: censored_words; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.censored_words (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    pattern text NOT NULL,
    action text DEFAULT 'block'::text NOT NULL,
    replacement text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: challenge_definitions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.challenge_definitions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    slug text NOT NULL,
    name text NOT NULL,
    description text,
    image_media_id uuid,
    activity_key text NOT NULL,
    target_count integer NOT NULL,
    reward_points integer DEFAULT 0 NOT NULL,
    reward_badge_id uuid,
    starts_at timestamp with time zone DEFAULT now() NOT NULL,
    ends_at timestamp with time zone,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: checklist_progress; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.checklist_progress (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    guide_id uuid NOT NULL,
    item_key text NOT NULL,
    completed boolean DEFAULT false NOT NULL,
    completed_at timestamp with time zone,
    notes text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    list_type text DEFAULT 'visa'::text NOT NULL
);


--
-- Name: comment_views; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.comment_views (
    comment_id uuid NOT NULL,
    user_id uuid NOT NULL,
    viewed_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: comment_votes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.comment_votes (
    comment_id uuid NOT NULL,
    user_id uuid NOT NULL,
    value smallint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT comment_votes_value_check CHECK ((value = ANY (ARRAY['-1'::integer, 1])))
);


--
-- Name: comments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.comments (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    content_id uuid NOT NULL,
    author_user_id uuid,
    parent_id uuid,
    body jsonb DEFAULT '{"type": "doc", "content": [{"type": "paragraph", "content": []}]}'::jsonb NOT NULL,
    depth integer DEFAULT 0 NOT NULL,
    vote_score integer DEFAULT 0 NOT NULL,
    is_pinned boolean DEFAULT false NOT NULL,
    is_edited boolean DEFAULT false NOT NULL,
    edited_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    is_anonymous boolean DEFAULT false NOT NULL,
    reply_count integer DEFAULT 0 NOT NULL,
    status text DEFAULT 'published'::text NOT NULL,
    removed_by uuid,
    removed_reason text,
    view_count integer DEFAULT 0 NOT NULL,
    upvote_count integer DEFAULT 0 NOT NULL,
    downvote_count integer DEFAULT 0 NOT NULL,
    CONSTRAINT comments_status_check CHECK ((status = ANY (ARRAY['published'::text, 'flagged'::text, 'removed'::text, 'spam'::text])))
);


--
-- Name: community_organisations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.community_organisations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    slug text NOT NULL,
    org_type text NOT NULL,
    origin_community text NOT NULL,
    destination_space_id uuid NOT NULL,
    city_space_id uuid,
    address text,
    latitude numeric(9,6),
    longitude numeric(9,6),
    website_url text,
    meeting_frequency text,
    contact_info text,
    japaconnect_space_id uuid,
    japaconnect_org_id uuid,
    claim_status text DEFAULT 'unclaimed'::text NOT NULL,
    is_verified boolean DEFAULT false NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    is_community_submitted boolean DEFAULT true NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    submitted_by uuid,
    reviewed_by uuid,
    reviewed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    ai_generated boolean DEFAULT false NOT NULL,
    ai_model text,
    ai_prompt_hash text,
    hitl_reviewed_by uuid,
    hitl_reviewed_at timestamp with time zone,
    hitl_review_notes text,
    is_japabot_indexed boolean GENERATED ALWAYS AS (((status = 'published'::text) AND (is_active = true))) STORED,
    intel_record_id uuid,
    guide_id uuid,
    confidence_overall real DEFAULT 0 NOT NULL,
    verification_status text DEFAULT 'verified'::text NOT NULL,
    freshness_score integer DEFAULT 100 NOT NULL,
    override_source text DEFAULT 'ingestion'::text NOT NULL,
    editorial_overrides jsonb DEFAULT '{}'::jsonb NOT NULL,
    CONSTRAINT community_organisations_override_source_check CHECK ((override_source = ANY (ARRAY['ingestion'::text, 'editorial'::text, 'mixed'::text]))),
    CONSTRAINT community_organisations_verification_status_check CHECK ((verification_status = ANY (ARRAY['verified'::text, 'needs_review'::text, 'conflict'::text, 'stale'::text])))
);


--
-- Name: comparison_content_metadata; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.comparison_content_metadata (
    content_id uuid NOT NULL,
    depth text DEFAULT 'quick'::text NOT NULL,
    promoted_from_id uuid,
    promoted_at timestamp with time zone,
    smart_mode text DEFAULT 'none'::text,
    score_version integer DEFAULT 1 NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT comparison_content_metadata_depth_check CHECK ((depth = ANY (ARRAY['quick'::text, 'detailed'::text]))),
    CONSTRAINT comparison_content_metadata_smart_mode_check CHECK ((smart_mode = ANY (ARRAY['none'::text, 'eligibility'::text, 'cost'::text, 'pathway'::text, 'ai_advisor'::text])))
);


--
-- Name: comparison_data_ingestion_jobs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.comparison_data_ingestion_jobs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    content_id uuid NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    source_kind text,
    refreshed_count integer DEFAULT 0 NOT NULL,
    error text,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT comparison_data_ingestion_jobs_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'running'::text, 'completed'::text, 'failed'::text, 'cancelled'::text])))
);


--
-- Name: comparison_data_sources; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.comparison_data_sources (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    entity_type text,
    entity_id uuid,
    dimension_slug text,
    source_name text NOT NULL,
    source_url text,
    source_kind text NOT NULL,
    trust_level integer DEFAULT 3 NOT NULL,
    freshness_score integer DEFAULT 0 NOT NULL,
    last_verified_at timestamp with time zone,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    created_by_user_id uuid,
    source_id uuid,
    CONSTRAINT comparison_data_sources_freshness_score_check CHECK (((freshness_score >= 0) AND (freshness_score <= 100))),
    CONSTRAINT comparison_data_sources_source_kind_check CHECK ((source_kind = ANY (ARRAY['official'::text, 'third_party'::text, 'ai_estimate'::text]))),
    CONSTRAINT comparison_data_sources_trust_level_check CHECK (((trust_level >= 1) AND (trust_level <= 5)))
);


--
-- Name: comparison_dimensions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.comparison_dimensions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    content_id uuid,
    label text NOT NULL,
    dimension_type text NOT NULL,
    "position" integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    type_id uuid,
    deleted_at timestamp with time zone,
    slug text,
    source_field_id uuid,
    CONSTRAINT comparison_dimensions_dimension_type_check CHECK ((dimension_type = ANY (ARRAY['text'::text, 'rating'::text, 'boolean'::text, 'number'::text, 'currency'::text])))
);


--
-- Name: comparison_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.comparison_items (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    content_id uuid,
    title text NOT NULL,
    description text,
    image_id uuid,
    geo_space_id uuid,
    "position" integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    type_id uuid,
    deleted_at timestamp with time zone,
    is_baseline boolean DEFAULT false NOT NULL,
    last_updated_at timestamp with time zone DEFAULT now() NOT NULL,
    intel_record_id uuid,
    intel_cost_data jsonb,
    confidence_overall real DEFAULT 0 NOT NULL,
    verification_status text DEFAULT 'verified'::text NOT NULL,
    freshness_score integer DEFAULT 100 NOT NULL,
    override_source text DEFAULT 'ingestion'::text NOT NULL,
    editorial_overrides jsonb DEFAULT '{}'::jsonb NOT NULL,
    item_ref jsonb DEFAULT '{}'::jsonb NOT NULL,
    CONSTRAINT comparison_items_override_source_check CHECK ((override_source = ANY (ARRAY['ingestion'::text, 'editorial'::text, 'mixed'::text]))),
    CONSTRAINT comparison_items_verification_status_check CHECK ((verification_status = ANY (ARRAY['verified'::text, 'needs_review'::text, 'conflict'::text, 'stale'::text])))
);


--
-- Name: comparison_profiles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.comparison_profiles (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid,
    title text NOT NULL,
    source_space_id uuid,
    target_space_ids uuid[] DEFAULT '{}'::uuid[] NOT NULL,
    goals jsonb DEFAULT '[]'::jsonb NOT NULL,
    preferences jsonb DEFAULT '{}'::jsonb NOT NULL,
    demographics jsonb DEFAULT '{}'::jsonb NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    is_template boolean DEFAULT false NOT NULL,
    deleted_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: comparison_recommendations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.comparison_recommendations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    content_id uuid NOT NULL,
    item_id uuid,
    profile_id uuid,
    factor_id uuid,
    text text NOT NULL,
    action_type text,
    action_url text,
    priority integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: comparison_score_factors; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.comparison_score_factors (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    comparison_type_id uuid,
    factor_key text NOT NULL,
    label text NOT NULL,
    description text,
    dimension_slug text,
    data_source text NOT NULL,
    default_weight numeric DEFAULT 0 NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    is_system boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT comparison_score_factors_data_source_check CHECK ((data_source = ANY (ARRAY['dimension'::text, 'resolver_metadata'::text, 'profile'::text, 'eligibility'::text, 'aggregate'::text]))),
    CONSTRAINT comparison_score_factors_default_weight_check CHECK (((default_weight >= (0)::numeric) AND (default_weight <= (1)::numeric)))
);


--
-- Name: comparison_scores; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.comparison_scores (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    content_id uuid NOT NULL,
    item_id uuid NOT NULL,
    factor_id uuid NOT NULL,
    dimension_id uuid,
    profile_id uuid,
    score_value numeric DEFAULT 0 NOT NULL,
    max_value numeric DEFAULT 100 NOT NULL,
    explanation text,
    eligible_status text,
    calculated_at timestamp with time zone DEFAULT now() NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    CONSTRAINT comparison_scores_eligible_status_check CHECK ((eligible_status = ANY (ARRAY['eligible'::text, 'limited'::text, 'ineligible'::text])))
);


--
-- Name: comparison_type_access; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.comparison_type_access (
    type_id uuid NOT NULL,
    user_group_id uuid,
    group_key uuid GENERATED ALWAYS AS (COALESCE(user_group_id, '00000000-0000-0000-0000-000000000000'::uuid)) STORED NOT NULL,
    can_create boolean DEFAULT false NOT NULL,
    can_fill boolean DEFAULT true NOT NULL,
    ai_can_fill boolean DEFAULT false NOT NULL
);


--
-- Name: comparison_type_definitions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.comparison_type_definitions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    slug text NOT NULL,
    description text,
    icon text,
    field_group_id uuid NOT NULL,
    item_label text DEFAULT 'Item'::text NOT NULL,
    is_system boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL
);


--
-- Name: comparison_values; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.comparison_values (
    dimension_id uuid NOT NULL,
    item_id uuid NOT NULL,
    value text,
    value_rating numeric(3,2),
    value_boolean boolean,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    updated_at timestamp with time zone,
    data_source_id uuid
);


--
-- Name: consent_records; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.consent_records (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid,
    consent_type text NOT NULL,
    granted boolean NOT NULL,
    policy_version text NOT NULL,
    ip_address inet,
    user_agent text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: content_affiliate_scan_results; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.content_affiliate_scan_results (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    content_id uuid NOT NULL,
    url text NOT NULL,
    domain text NOT NULL,
    matched_provider_id uuid,
    detection_method text NOT NULL,
    action_taken text DEFAULT 'flagged'::text NOT NULL,
    reviewed_by uuid,
    reviewed_at timestamp with time zone,
    notes text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT content_affiliate_scan_results_action_taken_check CHECK ((action_taken = ANY (ARRAY['flagged'::text, 'blocked'::text, 'stripped'::text, 'approved'::text]))),
    CONSTRAINT content_affiliate_scan_results_detection_method_check CHECK ((detection_method = ANY (ARRAY['param_match'::text, 'domain_match'::text, 'manual'::text])))
);


--
-- Name: TABLE content_affiliate_scan_results; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.content_affiliate_scan_results IS 'Tracks affiliate links found in content that are not registered in affiliate_providers. Used for moderation and revenue tracking.';


--
-- Name: content_campaign_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.content_campaign_items (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    campaign_id uuid NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    persona_id uuid NOT NULL,
    content_type text NOT NULL,
    topic_prompt text NOT NULL,
    title text,
    target_space_id uuid,
    target_tags text[],
    status text DEFAULT 'planned'::text NOT NULL,
    studio_job_id uuid,
    content_id uuid,
    review_id uuid,
    error_message text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT content_campaign_items_status_check CHECK ((status = ANY (ARRAY['planned'::text, 'queued'::text, 'generating'::text, 'draft_ready'::text, 'in_review'::text, 'approved'::text, 'published'::text, 'rejected'::text, 'failed'::text])))
);


--
-- Name: content_campaigns; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.content_campaigns (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    prompt text NOT NULL,
    status text DEFAULT 'planning'::text NOT NULL,
    content_type text,
    requested_count integer NOT NULL,
    persona_pool uuid[],
    auto_provision_personas boolean DEFAULT true NOT NULL,
    auto_assign boolean DEFAULT true NOT NULL,
    target_space_id uuid,
    target_tags text[],
    plan_json jsonb,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT content_campaigns_requested_count_check CHECK (((requested_count > 0) AND (requested_count <= 100))),
    CONSTRAINT content_campaigns_status_check CHECK ((status = ANY (ARRAY['planning'::text, 'planned'::text, 'generating'::text, 'completed'::text, 'failed'::text, 'cancelled'::text])))
);


--
-- Name: content_embeddings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.content_embeddings (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    entity_type text NOT NULL,
    entity_id uuid NOT NULL,
    chunk_index integer DEFAULT 0 NOT NULL,
    chunk_text text NOT NULL,
    embedding public.vector(1536) NOT NULL,
    language text DEFAULT 'en'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    visibility text NOT NULL,
    space_id uuid,
    author_id uuid,
    CONSTRAINT content_embeddings_visibility_chk CHECK ((visibility = ANY (ARRAY['public'::text, 'members'::text, 'private'::text])))
);


--
-- Name: content_engagement; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.content_engagement (
    content_id uuid NOT NULL,
    views integer DEFAULT 0 NOT NULL,
    comments_count integer DEFAULT 0 NOT NULL,
    votes_score integer DEFAULT 0 NOT NULL,
    bookmarks_count integer DEFAULT 0 NOT NULL,
    shares_count integer DEFAULT 0 NOT NULL,
    velocity_score double precision DEFAULT 0 NOT NULL,
    last_calculated_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: content_featured; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.content_featured (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    content_id uuid NOT NULL,
    featured_order integer DEFAULT 0 NOT NULL,
    featured_until timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: content_imports; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.content_imports (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    source_type text NOT NULL,
    source_url text,
    status text DEFAULT 'pending'::text NOT NULL,
    total_items integer DEFAULT 0 NOT NULL,
    imported_count integer DEFAULT 0 NOT NULL,
    failed_count integer DEFAULT 0 NOT NULL,
    error_log jsonb DEFAULT '[]'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    completed_at timestamp with time zone,
    CONSTRAINT content_imports_source_type_check CHECK ((source_type = ANY (ARRAY['medium'::text, 'substack'::text, 'rss'::text, 'zip'::text]))),
    CONSTRAINT content_imports_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'processing'::text, 'completed'::text, 'failed'::text, 'partial'::text])))
);


--
-- Name: content_reports; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.content_reports (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    reporter_user_id uuid,
    entity_type text NOT NULL,
    entity_id uuid NOT NULL,
    reason text NOT NULL,
    notes text,
    evidence_urls text[],
    status text DEFAULT 'pending'::text NOT NULL,
    reviewed_by uuid,
    resolution_note text,
    resolved_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT content_reports_entity_type_check CHECK ((entity_type = ANY (ARRAY['content'::text, 'comment'::text, 'user'::text, 'space'::text, 'message'::text, 'source'::text]))),
    CONSTRAINT content_reports_reason_check CHECK ((reason = ANY (ARRAY['spam'::text, 'misinformation'::text, 'harassment'::text, 'hate_speech'::text, 'illegal_content'::text, 'self_harm'::text, 'copyright'::text, 'other'::text]))),
    CONSTRAINT content_reports_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'reviewing'::text, 'actioned'::text, 'dismissed'::text])))
);


--
-- Name: content_reviews; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.content_reviews (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    content_id uuid NOT NULL,
    reviewer_user_id uuid,
    status text DEFAULT 'pending'::text NOT NULL,
    feedback text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    source text DEFAULT 'editorial'::text NOT NULL,
    persona_payload jsonb,
    CONSTRAINT content_reviews_source_check CHECK ((source = ANY (ARRAY['editorial'::text, 'persona_action'::text]))),
    CONSTRAINT content_reviews_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'approved'::text, 'changes_requested'::text, 'rejected'::text])))
);


--
-- Name: content_revisions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.content_revisions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    content_id uuid NOT NULL,
    field_values jsonb DEFAULT '{}'::jsonb NOT NULL,
    title text NOT NULL,
    body text NOT NULL,
    body_html text NOT NULL,
    edited_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: content_scores; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.content_scores (
    content_id uuid NOT NULL,
    vote_score integer DEFAULT 0 NOT NULL,
    comment_count integer DEFAULT 0 NOT NULL,
    view_count integer DEFAULT 0 NOT NULL,
    bookmark_count integer DEFAULT 0 NOT NULL,
    velocity_score numeric DEFAULT 0 NOT NULL,
    relevance_embedding public.vector(1536),
    last_scored_at timestamp with time zone DEFAULT now() NOT NULL,
    reaction_count integer DEFAULT 0 NOT NULL
);


--
-- Name: content_shares; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.content_shares (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    content_id uuid NOT NULL,
    user_id uuid NOT NULL,
    platform text DEFAULT 'link'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: content_spaces; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.content_spaces (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    content_id uuid NOT NULL,
    space_id uuid NOT NULL,
    is_primary boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: content_tags; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.content_tags (
    content_id uuid NOT NULL,
    tag_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: content_thread_links; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.content_thread_links (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    content_id uuid NOT NULL,
    thread_id uuid NOT NULL,
    link_count integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: content_type_jsonld_templates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.content_type_jsonld_templates (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    content_type text NOT NULL,
    template jsonb DEFAULT '{}'::jsonb NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: content_views; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.content_views (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    content_id uuid NOT NULL,
    user_id uuid,
    space_id uuid,
    viewed_at timestamp with time zone DEFAULT now() NOT NULL,
    ip_hash text
);


--
-- Name: contents; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.contents (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    author_id uuid NOT NULL,
    space_id uuid,
    content_type text DEFAULT 'discussion'::text NOT NULL,
    title text NOT NULL,
    slug text NOT NULL,
    body text DEFAULT ''::text NOT NULL,
    body_html text DEFAULT ''::text NOT NULL,
    field_values jsonb DEFAULT '{}'::jsonb NOT NULL,
    content_hash text,
    geo_space_id uuid,
    header_image_id uuid,
    featured_image_id uuid,
    canonical_url text,
    status text DEFAULT 'draft'::text NOT NULL,
    visibility text DEFAULT 'public'::text NOT NULL,
    is_locked boolean DEFAULT false NOT NULL,
    is_pinned boolean DEFAULT false NOT NULL,
    read_time_minutes integer DEFAULT 0 NOT NULL,
    view_count integer DEFAULT 0 NOT NULL,
    comment_count integer DEFAULT 0 NOT NULL,
    share_count integer DEFAULT 0 NOT NULL,
    reshare_count integer DEFAULT 0 NOT NULL,
    publish_at timestamp with time zone,
    published_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    auto_hidden boolean DEFAULT false NOT NULL,
    auto_hidden_at timestamp with time zone,
    deleted_at timestamp with time zone,
    organization_id uuid,
    meta_title text,
    meta_description text,
    og_image_url text,
    seo_indexable boolean DEFAULT true NOT NULL,
    guide_category text,
    ai_exempt boolean DEFAULT false NOT NULL,
    ai_generated boolean DEFAULT false NOT NULL,
    ai_model text,
    ai_generated_at timestamp with time zone,
    hitl_reviewed_by uuid,
    hitl_reviewed_at timestamp with time zone,
    journey_stage text,
    visa_route text,
    source_country text,
    target_country text,
    video_source_id uuid,
    intel_source_id uuid,
    intel_record_id uuid,
    CONSTRAINT contents_status_check CHECK ((status = ANY (ARRAY['draft'::text, 'in_review'::text, 'scheduled'::text, 'published'::text, 'archived'::text]))),
    CONSTRAINT contents_visibility_check CHECK ((visibility = ANY (ARRAY['public'::text, 'members'::text, 'private'::text])))
);


--
-- Name: coupon_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.coupon_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    coupon_id uuid NOT NULL,
    event_type text NOT NULL,
    user_id uuid,
    content_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT coupon_events_event_type_check CHECK ((event_type = ANY (ARRAY['click'::text, 'copy'::text])))
);


--
-- Name: coupons; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.coupons (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    provider_id uuid,
    coupon_type text DEFAULT 'external'::text NOT NULL,
    title text NOT NULL,
    description text,
    code text NOT NULL,
    discount_type text,
    discount_value numeric,
    currency text,
    terms_url text,
    referral_url text,
    is_verified boolean DEFAULT false NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    valid_from timestamp with time zone,
    expires_at timestamp with time zone,
    total_clicks integer DEFAULT 0 NOT NULL,
    total_copies integer DEFAULT 0 NOT NULL,
    created_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT coupons_coupon_type_check CHECK ((coupon_type = ANY (ARRAY['external'::text, 'internal'::text]))),
    CONSTRAINT coupons_discount_type_check CHECK ((discount_type = ANY (ARRAY['percent'::text, 'fixed'::text, 'free_shipping'::text, NULL::text]))),
    CONSTRAINT coupons_status_check CHECK ((status = ANY (ARRAY['active'::text, 'expired'::text, 'suspended'::text])))
);


--
-- Name: course_access_grants; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.course_access_grants (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    content_id uuid NOT NULL,
    grant_type text NOT NULL,
    payment_id uuid,
    granted_at timestamp with time zone DEFAULT now() NOT NULL,
    revoked_at timestamp with time zone,
    CONSTRAINT course_access_grants_grant_type_check CHECK ((grant_type = ANY (ARRAY['purchase'::text, 'subscription'::text, 'admin'::text])))
);


--
-- Name: credential_community_notes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.credential_community_notes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    pathway_id uuid NOT NULL,
    user_id uuid NOT NULL,
    note text NOT NULL,
    upvotes integer DEFAULT 0 NOT NULL,
    status text DEFAULT 'published'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);


--
-- Name: credential_pathways; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.credential_pathways (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    profession_id uuid NOT NULL,
    issuing_space_id uuid NOT NULL,
    destination_space_id uuid NOT NULL,
    regulatory_body text NOT NULL,
    regulatory_body_url text,
    pathway_type text NOT NULL,
    timeline_estimate text,
    cost_estimate_usd text,
    pathway_description text NOT NULL,
    official_application_url text,
    source_url_last_checked date,
    status text DEFAULT 'published'::text NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    ai_generated boolean DEFAULT false NOT NULL,
    ai_model text,
    ai_prompt_hash text,
    hitl_reviewed_by uuid,
    hitl_reviewed_at timestamp with time zone,
    hitl_review_notes text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    is_japabot_indexed boolean GENERATED ALWAYS AS (((status = 'published'::text) AND (is_active = true))) STORED,
    guide_id uuid,
    intel_record_id uuid,
    confidence_overall real DEFAULT 0 NOT NULL,
    verification_status text DEFAULT 'verified'::text NOT NULL,
    freshness_score integer DEFAULT 100 NOT NULL,
    override_source text DEFAULT 'ingestion'::text NOT NULL,
    editorial_overrides jsonb DEFAULT '{}'::jsonb NOT NULL,
    CONSTRAINT credential_pathways_override_source_check CHECK ((override_source = ANY (ARRAY['ingestion'::text, 'editorial'::text, 'mixed'::text]))),
    CONSTRAINT credential_pathways_verification_status_check CHECK ((verification_status = ANY (ARRAY['verified'::text, 'needs_review'::text, 'conflict'::text, 'stale'::text])))
);


--
-- Name: credential_professions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.credential_professions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    slug text NOT NULL,
    name text NOT NULL,
    category text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: credit_packs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.credit_packs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    description text,
    credit_amount integer NOT NULL,
    bonus_credits integer DEFAULT 0 NOT NULL,
    min_user_level text,
    is_active boolean DEFAULT true NOT NULL,
    "position" integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT credit_packs_bonus_credits_check CHECK ((bonus_credits >= 0)),
    CONSTRAINT credit_packs_credit_amount_check CHECK ((credit_amount > 0))
);


--
-- Name: crs_draws; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.crs_draws (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    draw_number integer,
    draw_date date NOT NULL,
    category text DEFAULT 'general'::text NOT NULL,
    crs_cutoff integer NOT NULL,
    invitations_issued integer NOT NULL,
    tie_break_rule text,
    source_url text,
    status text DEFAULT 'published'::text NOT NULL,
    hitl_reviewed_by uuid,
    hitl_reviewed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    is_japabot_indexed boolean GENERATED ALWAYS AS ((status = 'published'::text)) STORED
);


--
-- Name: crs_factor_points; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.crs_factor_points (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    ruleset_id uuid NOT NULL,
    factor_group text NOT NULL,
    factor_key text NOT NULL,
    input_descriptor jsonb NOT NULL,
    label text NOT NULL,
    points_single integer NOT NULL,
    points_with_spouse integer,
    sort_order integer DEFAULT 0
);


--
-- Name: crs_rulesets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.crs_rulesets (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    version_label text NOT NULL,
    effective_date date NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    source_url text,
    source_url_last_checked date,
    ai_generated boolean DEFAULT false NOT NULL,
    ai_model text,
    ai_prompt_hash text,
    hitl_reviewed_by uuid,
    hitl_reviewed_at timestamp with time zone,
    hitl_review_notes text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    is_japabot_indexed boolean GENERATED ALWAYS AS ((status = 'active'::text)) STORED
);


--
-- Name: crs_score_history; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.crs_score_history (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    ruleset_id uuid NOT NULL,
    inputs jsonb NOT NULL,
    score integer NOT NULL,
    breakdown jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);


--
-- Name: currencies; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.currencies (
    code text NOT NULL,
    name text NOT NULL,
    symbol text NOT NULL,
    credits_per_unit integer NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    "position" integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT currencies_credits_per_unit_check CHECK ((credits_per_unit > 0))
);


--
-- Name: custom_field_entity_types; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.custom_field_entity_types (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    value text NOT NULL,
    label text NOT NULL,
    is_system boolean DEFAULT false NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: custom_field_group_assignments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.custom_field_group_assignments (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    group_id uuid NOT NULL,
    entity_class text NOT NULL,
    entity_id uuid,
    "position" integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: custom_field_groups; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.custom_field_groups (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    slug text NOT NULL,
    description text,
    is_system boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: custom_field_permissions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.custom_field_permissions (
    field_id uuid NOT NULL,
    user_group_id uuid NOT NULL,
    can_view boolean DEFAULT true NOT NULL,
    can_edit boolean DEFAULT false NOT NULL
);


--
-- Name: custom_field_templates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.custom_field_templates (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    description text,
    field_group_id uuid NOT NULL,
    is_system boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: custom_field_values; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.custom_field_values (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    field_id uuid NOT NULL,
    entity_type text NOT NULL,
    entity_id uuid NOT NULL,
    value_text text,
    value_number numeric,
    value_bool boolean,
    value_date timestamp with time zone,
    value_json jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    ai_exempt boolean DEFAULT false NOT NULL
);


--
-- Name: custom_fields; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.custom_fields (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    group_id uuid NOT NULL,
    label text NOT NULL,
    key text NOT NULL,
    field_type text NOT NULL,
    options jsonb,
    embed_platforms text[],
    placeholder text,
    default_value text,
    is_required boolean DEFAULT false NOT NULL,
    is_system boolean DEFAULT false NOT NULL,
    show_in_frontend boolean DEFAULT true NOT NULL,
    filterable boolean DEFAULT false NOT NULL,
    sortable boolean DEFAULT false NOT NULL,
    searchable boolean DEFAULT false NOT NULL,
    "position" integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL
);


--
-- Name: COLUMN custom_fields.key; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.custom_fields.key IS 'destination_country: ISO 3166-1 alpha-2 code of the country the user is relocating to (ADR-064 VisitorContext).';


--
-- Name: cv_format_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cv_format_sessions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    origin_space_id uuid,
    destination_space_id uuid NOT NULL,
    input_text text NOT NULL,
    output_text text,
    output_format text DEFAULT 'docx'::text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    ai_model text,
    tokens_used integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone DEFAULT (now() + '24:00:00'::interval) NOT NULL,
    ai_generated boolean DEFAULT true NOT NULL,
    ai_prompt_hash text,
    hitl_reviewed_by uuid,
    hitl_reviewed_at timestamp with time zone,
    hitl_review_notes text,
    original_filename text,
    file_storage_path text,
    file_mime_type text,
    file_size_bytes bigint
);


--
-- Name: COLUMN cv_format_sessions.original_filename; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.cv_format_sessions.original_filename IS 'Original filename of the uploaded CV file (if submitted via file upload)';


--
-- Name: COLUMN cv_format_sessions.file_storage_path; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.cv_format_sessions.file_storage_path IS 'S3/SeaweedFS object key for the uploaded file (if submitted via file upload)';


--
-- Name: COLUMN cv_format_sessions.file_mime_type; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.cv_format_sessions.file_mime_type IS 'MIME type of the uploaded file (application/pdf, application/vnd.openxmlformats-officedocument.wordprocessingml.document)';


--
-- Name: COLUMN cv_format_sessions.file_size_bytes; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.cv_format_sessions.file_size_bytes IS 'Size of the uploaded file in bytes';


--
-- Name: cv_standards; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cv_standards (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    destination_space_id uuid NOT NULL,
    recommended_length_pages text,
    photo_policy text,
    date_format text,
    recommended_sections text[],
    tone_guidance text,
    common_pitfalls text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: data_export_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.data_export_requests (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    download_url text,
    expires_at timestamp with time zone,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: device_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.device_tokens (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    device_id text NOT NULL,
    token text NOT NULL,
    platform text NOT NULL,
    app_version text,
    is_active boolean DEFAULT true NOT NULL,
    last_used_at timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT device_tokens_platform_check CHECK ((platform = ANY (ARRAY['ios'::text, 'android'::text, 'web'::text])))
);


--
-- Name: digest_editions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.digest_editions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    space_id uuid,
    cadence text DEFAULT 'weekly'::text NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    scheduled_for timestamp with time zone,
    published_at timestamp with time zone,
    content jsonb DEFAULT '{}'::jsonb NOT NULL,
    email_sent boolean DEFAULT false NOT NULL,
    email_sent_at timestamp with time zone,
    generated_by text DEFAULT 'system'::text NOT NULL,
    reviewed_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: COLUMN digest_editions.status; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.digest_editions.status IS 'draft|review|published|cancelled (§84)';


--
-- Name: discount_code_uses; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.discount_code_uses (
    code_id uuid NOT NULL,
    user_id uuid NOT NULL,
    payment_id uuid,
    used_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: discount_codes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.discount_codes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    code text NOT NULL,
    discount_type text NOT NULL,
    discount_value numeric NOT NULL,
    applicable_to text NOT NULL,
    max_uses integer,
    uses_per_user integer DEFAULT 1 NOT NULL,
    group_id uuid,
    valid_from timestamp with time zone DEFAULT now() NOT NULL,
    valid_until timestamp with time zone,
    created_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT discount_codes_applicable_to_check CHECK ((applicable_to = ANY (ARRAY['course_access'::text, 'pro_membership'::text, 'all'::text]))),
    CONSTRAINT discount_codes_discount_type_check CHECK ((discount_type = ANY (ARRAY['percent'::text, 'fixed'::text, 'percentage'::text])))
);


--
-- Name: dm_conversations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.dm_conversations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    participant_1_id uuid NOT NULL,
    participant_2_id uuid NOT NULL,
    last_message_at timestamp with time zone,
    last_message_preview text,
    p1_unread_count integer DEFAULT 0 NOT NULL,
    p2_unread_count integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT dm_conversations_check CHECK ((participant_1_id < participant_2_id))
);


--
-- Name: dm_messages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.dm_messages (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    conversation_id uuid NOT NULL,
    sender_id uuid NOT NULL,
    body text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    read_at timestamp with time zone,
    deleted_at timestamp with time zone,
    edited_at timestamp with time zone
);


--
-- Name: dm_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.dm_requests (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    requester_id uuid NOT NULL,
    recipient_id uuid NOT NULL,
    conversation_id uuid,
    preview_message text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    resolved_at timestamp with time zone,
    CONSTRAINT dm_requests_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'accepted'::text, 'declined'::text, 'blocked'::text])))
);


--
-- Name: doc_pages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.doc_pages (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    title text NOT NULL,
    slug text NOT NULL,
    audience text NOT NULL,
    content jsonb DEFAULT '{}'::jsonb NOT NULL,
    ai_generated boolean DEFAULT false NOT NULL,
    review_status text DEFAULT 'draft'::text NOT NULL,
    last_reviewed_by uuid,
    last_reviewed_at timestamp with time zone,
    parent_id uuid,
    "position" integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    helpful_yes integer DEFAULT 0 NOT NULL,
    helpful_no integer DEFAULT 0 NOT NULL
);


--
-- Name: eca_community_updates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.eca_community_updates (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    guide_id uuid NOT NULL,
    user_id uuid NOT NULL,
    step_index integer,
    fee_estimate text,
    turnaround_days integer,
    note text,
    status text DEFAULT 'pending'::text NOT NULL,
    reviewed_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);


--
-- Name: eca_guides; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.eca_guides (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    destination_space_id uuid NOT NULL,
    provider_id uuid NOT NULL,
    purpose text NOT NULL,
    pathway_steps jsonb NOT NULL,
    fee_estimate text,
    currency character(3),
    timeline_estimate text,
    documents_required jsonb,
    accepted_for jsonb,
    official_url text,
    source_url_last_checked date,
    status text DEFAULT 'published'::text NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    ai_generated boolean DEFAULT false NOT NULL,
    ai_model text,
    ai_prompt_hash text,
    hitl_reviewed_by uuid,
    hitl_reviewed_at timestamp with time zone,
    hitl_review_notes text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    is_japabot_indexed boolean GENERATED ALWAYS AS (((status = 'published'::text) AND (is_active = true))) STORED
);


--
-- Name: eca_providers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.eca_providers (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    slug text NOT NULL,
    name text NOT NULL,
    website text,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: editor_block_permissions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.editor_block_permissions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    block_type text NOT NULL,
    user_group_id uuid NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: email_templates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.email_templates (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    type text NOT NULL,
    name text NOT NULL,
    subject text NOT NULL,
    html_body text NOT NULL,
    plain_text_body text DEFAULT ''::text NOT NULL,
    variables jsonb DEFAULT '[]'::jsonb NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    slug text DEFAULT ''::text NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    CONSTRAINT email_templates_status_check CHECK ((status = ANY (ARRAY['active'::text, 'inactive'::text])))
);


--
-- Name: email_verifications; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.email_verifications (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    email text NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    verified_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: enrollments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.enrollments (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    track_id uuid NOT NULL,
    access_type text DEFAULT 'free'::text NOT NULL,
    progress_pct numeric DEFAULT 0 NOT NULL,
    enrolled_at timestamp with time zone DEFAULT now() NOT NULL,
    completed_at timestamp with time zone,
    payment_id uuid
);


--
-- Name: COLUMN enrollments.payment_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.enrollments.payment_id IS 'Section 17 payment for paid enrollments; NULL for free/Pro/pro_stream (ADR-063).';


--
-- Name: external_data_sources; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.external_data_sources (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    source_type text NOT NULL,
    api_endpoint text,
    sync_frequency text DEFAULT 'daily'::text NOT NULL,
    last_synced_at timestamp with time zone,
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: feature_flags; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.feature_flags (
    key text NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_by uuid
);


--
-- Name: feeds; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.feeds (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    content_id uuid NOT NULL,
    space_id uuid,
    content_type text NOT NULL,
    score numeric DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: follows; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.follows (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    follower_id uuid NOT NULL,
    entity_type text NOT NULL,
    entity_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT follows_entity_type_check CHECK ((entity_type = ANY (ARRAY['user'::text, 'space'::text, 'tag'::text, 'organization'::text, 'professional_profile'::text, 'source'::text])))
);


--
-- Name: form_steps; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.form_steps (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    form_id uuid NOT NULL,
    title text NOT NULL,
    "position" integer NOT NULL
);


--
-- Name: form_submission_triggers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.form_submission_triggers (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    form_id uuid NOT NULL,
    trigger_type text NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    "position" integer DEFAULT 0 NOT NULL
);


--
-- Name: form_submissions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.form_submissions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    form_id uuid NOT NULL,
    user_id uuid,
    ip_address inet,
    field_values jsonb DEFAULT '{}'::jsonb NOT NULL,
    status text DEFAULT 'new'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    payment_id uuid
);


--
-- Name: forms; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.forms (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    slug text NOT NULL,
    description text,
    is_multi_step boolean DEFAULT false NOT NULL,
    submit_label text DEFAULT 'Submit'::text NOT NULL,
    success_message text,
    status text DEFAULT 'active'::text NOT NULL,
    is_system boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    space_id uuid,
    form_type text DEFAULT 'standard'::text NOT NULL,
    scoring_config jsonb DEFAULT '{}'::jsonb NOT NULL,
    submission_fee_credits integer DEFAULT 0 NOT NULL,
    CONSTRAINT forms_form_type_check CHECK ((form_type = ANY (ARRAY['standard'::text, 'assessment'::text])))
);


--
-- Name: forum_bookmarks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.forum_bookmarks (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    entity_type text NOT NULL,
    entity_id uuid NOT NULL,
    folder text DEFAULT 'default'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT forum_bookmarks_entity_type_check CHECK ((entity_type = ANY (ARRAY['thread'::text, 'reply'::text])))
);


--
-- Name: forum_poll_options; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.forum_poll_options (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    poll_id uuid NOT NULL,
    label text NOT NULL,
    "position" integer DEFAULT 0 NOT NULL
);


--
-- Name: forum_poll_votes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.forum_poll_votes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    poll_id uuid NOT NULL,
    option_id uuid NOT NULL,
    user_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: forum_polls; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.forum_polls (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    thread_id uuid NOT NULL,
    question text NOT NULL,
    is_multi_select boolean DEFAULT false NOT NULL,
    close_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: forum_replies; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.forum_replies (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    thread_id uuid NOT NULL,
    parent_reply_id uuid,
    author_user_id uuid,
    body jsonb DEFAULT '[]'::jsonb NOT NULL,
    vote_score integer DEFAULT 0 NOT NULL,
    view_count integer DEFAULT 0 NOT NULL,
    is_anonymous boolean DEFAULT false NOT NULL,
    is_edited boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    ama_pending boolean DEFAULT false NOT NULL,
    ama_approved_at timestamp with time zone
);


--
-- Name: forum_tags; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.forum_tags (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    slug text NOT NULL,
    description text,
    rules text,
    cover_image_id uuid,
    follower_count integer DEFAULT 0 NOT NULL,
    moderator_user_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);


--
-- Name: forum_thread_tags; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.forum_thread_tags (
    thread_id uuid NOT NULL,
    tag_id uuid NOT NULL
);


--
-- Name: forum_threads; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.forum_threads (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    content_id uuid,
    space_id uuid,
    author_user_id uuid,
    title text NOT NULL,
    slug text NOT NULL,
    body jsonb DEFAULT '[]'::jsonb NOT NULL,
    is_anonymous boolean DEFAULT false NOT NULL,
    solved boolean DEFAULT false NOT NULL,
    solved_at timestamp with time zone,
    solved_by_user_id uuid,
    vote_score integer DEFAULT 0 NOT NULL,
    reply_count integer DEFAULT 0 NOT NULL,
    view_count integer DEFAULT 0 NOT NULL,
    bookmark_count integer DEFAULT 0 NOT NULL,
    edit_window_minutes integer DEFAULT 30 NOT NULL,
    is_edited boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    parent_reply_id uuid,
    ama_config jsonb
);


--
-- Name: forum_votes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.forum_votes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    entity_type text NOT NULL,
    entity_id uuid NOT NULL,
    value smallint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT forum_votes_entity_type_check CHECK ((entity_type = ANY (ARRAY['thread'::text, 'reply'::text]))),
    CONSTRAINT forum_votes_value_check CHECK ((value = ANY (ARRAY['-1'::integer, 1])))
);


--
-- Name: group_roles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.group_roles (
    group_id uuid NOT NULL,
    role_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: housing_guides; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.housing_guides (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    destination_space_id uuid NOT NULL,
    city_space_id uuid,
    accommodation_type text,
    search_platforms jsonb,
    landlord_requirements text,
    rent_ranges jsonb,
    scam_categories text[],
    tips text,
    status text DEFAULT 'published'::text NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    ai_generated boolean DEFAULT false NOT NULL,
    ai_model text,
    ai_prompt_hash text,
    hitl_reviewed_by uuid,
    hitl_reviewed_at timestamp with time zone,
    hitl_review_notes text,
    last_verified_at date,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    is_japabot_indexed boolean GENERATED ALWAYS AS (((status = 'published'::text) AND (is_active = true))) STORED,
    intel_record_id uuid,
    confidence_overall real DEFAULT 0 NOT NULL,
    verification_status text DEFAULT 'verified'::text NOT NULL,
    freshness_score integer DEFAULT 100 NOT NULL,
    override_source text DEFAULT 'ingestion'::text NOT NULL,
    editorial_overrides jsonb DEFAULT '{}'::jsonb NOT NULL,
    guide_id uuid,
    CONSTRAINT housing_guides_override_source_check CHECK ((override_source = ANY (ARRAY['ingestion'::text, 'editorial'::text, 'mixed'::text]))),
    CONSTRAINT housing_guides_verification_status_check CHECK ((verification_status = ANY (ARRAY['verified'::text, 'needs_review'::text, 'conflict'::text, 'stale'::text])))
);


--
-- Name: instructor_payouts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.instructor_payouts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    instructor_id uuid,
    period_start date NOT NULL,
    period_end date NOT NULL,
    gross_kobo bigint DEFAULT 0 NOT NULL,
    platform_commission_kobo bigint DEFAULT 0 NOT NULL,
    net_kobo bigint DEFAULT 0 NOT NULL,
    currency text DEFAULT 'NGN'::text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    payout_reference text,
    failure_reason text,
    processed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT instructor_payouts_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'processing'::text, 'paid'::text, 'failed'::text])))
);


--
-- Name: TABLE instructor_payouts; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.instructor_payouts IS 'Aggregated periodic creator payout; rolls up revenue_splits (ADR-063).';


--
-- Name: COLUMN instructor_payouts.instructor_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.instructor_payouts.instructor_id IS 'Owning instructor; NULL after GDPR erasure (anonymized, regulatory retention).';


--
-- Name: instructor_profiles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.instructor_profiles (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    display_name text NOT NULL,
    headline text,
    bio text,
    social_links jsonb DEFAULT '{}'::jsonb NOT NULL,
    tier text DEFAULT 'standard'::text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    commission_pct_override numeric(5,2),
    payout_method jsonb,
    kyc_status text DEFAULT 'unverified'::text NOT NULL,
    application_note text,
    reviewed_by uuid,
    reviewed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT instructor_profiles_commission_pct_override_check CHECK (((commission_pct_override IS NULL) OR ((commission_pct_override >= (0)::numeric) AND (commission_pct_override <= (100)::numeric)))),
    CONSTRAINT instructor_profiles_kyc_status_check CHECK ((kyc_status = ANY (ARRAY['unverified'::text, 'pending'::text, 'verified'::text, 'failed'::text]))),
    CONSTRAINT instructor_profiles_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'approved'::text, 'rejected'::text, 'suspended'::text]))),
    CONSTRAINT instructor_profiles_tier_check CHECK ((tier = ANY (ARRAY['standard'::text, 'verified'::text, 'partner'::text])))
);


--
-- Name: TABLE instructor_profiles; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.instructor_profiles IS 'Japa University creator identity; gated application/approval (ADR-063).';


--
-- Name: COLUMN instructor_profiles.commission_pct_override; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.instructor_profiles.commission_pct_override IS 'Creator revenue % override; NULL falls back to tier/system default.';


--
-- Name: intel_inbound_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.intel_inbound_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    channel_id uuid NOT NULL,
    payload bytea NOT NULL,
    headers jsonb,
    content_type text,
    content_hash text NOT NULL,
    received_at timestamp with time zone DEFAULT now() NOT NULL,
    processed_at timestamp with time zone,
    status text DEFAULT 'received'::text NOT NULL,
    claimed_at timestamp with time zone,
    CONSTRAINT intel_inbound_events_status_check CHECK ((status = ANY (ARRAY['received'::text, 'processing'::text, 'processed'::text, 'failed'::text, 'ignored'::text])))
);


--
-- Name: intel_jobs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.intel_jobs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    intel_record_id uuid NOT NULL,
    guide_id uuid,
    source_url text NOT NULL,
    source_type text NOT NULL,
    trust_level smallint DEFAULT 0 NOT NULL,
    title text NOT NULL,
    description text,
    country text NOT NULL,
    country_space_id uuid,
    city text,
    employer text,
    job_type text,
    experience_level text,
    salary_min numeric(18,2),
    salary_max numeric(18,2),
    currency text,
    skills text[],
    application_url text,
    deadline date,
    remote text,
    confidence_overall real DEFAULT 0 NOT NULL,
    verification_status text DEFAULT 'verified'::text NOT NULL,
    freshness_score integer DEFAULT 100 NOT NULL,
    override_source text DEFAULT 'ingestion'::text NOT NULL,
    editorial_overrides jsonb DEFAULT '{}'::jsonb NOT NULL,
    last_verified_at timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT intel_jobs_override_source_check CHECK ((override_source = ANY (ARRAY['ingestion'::text, 'editorial'::text, 'mixed'::text]))),
    CONSTRAINT intel_jobs_verification_status_check CHECK ((verification_status = ANY (ARRAY['verified'::text, 'needs_review'::text, 'conflict'::text, 'stale'::text])))
);


--
-- Name: intel_raw_documents; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.intel_raw_documents (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    source_id uuid NOT NULL,
    url text NOT NULL,
    http_status integer NOT NULL,
    http_headers jsonb DEFAULT '{}'::jsonb NOT NULL,
    content_type text,
    body bytea NOT NULL,
    content_hash text NOT NULL,
    retrieved_at timestamp with time zone DEFAULT now() NOT NULL,
    channel_id uuid,
    parent_document_id uuid
);


--
-- Name: intel_records; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.intel_records (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    source_id uuid NOT NULL,
    raw_document_id uuid NOT NULL,
    entity_type text NOT NULL,
    external_key text NOT NULL,
    extracted jsonb NOT NULL,
    confidence jsonb NOT NULL,
    evidence jsonb NOT NULL,
    confidence_overall real NOT NULL,
    status text DEFAULT 'extracted'::text NOT NULL,
    freshness_score integer DEFAULT 100 NOT NULL,
    geo_space_ids uuid[] DEFAULT '{}'::uuid[] NOT NULL,
    extracted_at timestamp with time zone DEFAULT now() NOT NULL,
    last_verified_at timestamp with time zone DEFAULT now() NOT NULL,
    promoted_record_id uuid,
    CONSTRAINT intel_records_status_check CHECK ((status = ANY (ARRAY['extracted'::text, 'deduped'::text, 'auto_published'::text, 'review_required'::text, 'approved'::text, 'rejected'::text, 'published'::text, 'stale'::text, 'off_topic'::text])))
);


--
-- Name: intel_review_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.intel_review_log (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    intel_record_id uuid NOT NULL,
    actor_user_id uuid NOT NULL,
    action text NOT NULL,
    field_changes jsonb DEFAULT '{}'::jsonb NOT NULL,
    note text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: intel_source_channels; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.intel_source_channels (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    source_id uuid NOT NULL,
    label text NOT NULL,
    entry_url text NOT NULL,
    crawl_method text NOT NULL,
    entity_types text[] DEFAULT '{}'::text[] NOT NULL,
    pipeline text DEFAULT 'extract'::text NOT NULL,
    provider_config jsonb DEFAULT '{}'::jsonb NOT NULL,
    crawl_frequency_mins integer DEFAULT 1440 NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    origin text DEFAULT 'probe'::text NOT NULL,
    relevance_mode text DEFAULT 'filtered'::text NOT NULL,
    relevance_topics text[] DEFAULT '{}'::text[] NOT NULL,
    relevance_url_patterns text[] DEFAULT '{}'::text[] NOT NULL,
    last_crawled_at timestamp with time zone,
    last_success_at timestamp with time zone,
    last_error_at timestamp with time zone,
    last_error_message text,
    last_changed_at timestamp with time zone,
    consecutive_failures integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT intel_source_channels_origin_check CHECK ((origin = ANY (ARRAY['probe'::text, 'admin'::text, 'user'::text]))),
    CONSTRAINT intel_source_channels_pipeline_check CHECK ((pipeline = ANY (ARRAY['extract'::text, 'curate'::text, 'hybrid'::text]))),
    CONSTRAINT intel_source_channels_relevance_mode_check CHECK ((relevance_mode = ANY (ARRAY['open'::text, 'filtered'::text, 'custom'::text])))
);


--
-- Name: intel_sources; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.intel_sources (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    domain text NOT NULL,
    source_url text NOT NULL,
    category text NOT NULL,
    source_type text NOT NULL,
    trust_level smallint NOT NULL,
    crawl_method text NOT NULL,
    crawl_frequency text NOT NULL,
    crawl_frequency_mins integer DEFAULT 1440 NOT NULL,
    robots_policy jsonb DEFAULT '{}'::jsonb NOT NULL,
    source_license text,
    attribution_required boolean DEFAULT true NOT NULL,
    usage_restriction text,
    provider_config jsonb DEFAULT '{}'::jsonb NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    last_crawled_at timestamp with time zone,
    last_success_at timestamp with time zone,
    last_error_at timestamp with time zone,
    last_error_message text,
    last_changed_at timestamp with time zone,
    consecutive_failures integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    persona_id uuid,
    status text DEFAULT 'active'::text NOT NULL,
    proposed_by uuid,
    proposed_at timestamp with time zone,
    approved_by uuid,
    approved_at timestamp with time zone,
    slug text,
    description text,
    logo_url text,
    follower_count integer DEFAULT 0 NOT NULL,
    post_count integer DEFAULT 0 NOT NULL,
    source_kind text DEFAULT 'publisher'::text NOT NULL,
    keywords text[],
    categories text[] DEFAULT '{}'::text[] NOT NULL,
    probe_report jsonb DEFAULT '{}'::jsonb NOT NULL,
    detected_methods text[] DEFAULT '{}'::text[] NOT NULL,
    last_probed_at timestamp with time zone,
    CONSTRAINT intel_sources_source_kind_check CHECK ((source_kind = ANY (ARRAY['publisher'::text, 'institution'::text, 'individual'::text, 'aggregator'::text, 'channel_profile'::text, 'institution_site'::text, 'personal_blog'::text, 'job_board'::text, 'gov_portal'::text, 'community'::text])))
);


--
-- Name: COLUMN intel_sources.status; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.intel_sources.status IS 'active | proposed | disabled | rejected';


--
-- Name: ip_blocks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ip_blocks (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    cidr inet NOT NULL,
    block_type text DEFAULT 'full'::text NOT NULL,
    reason text,
    blocked_by uuid,
    expires_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: job_applications; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.job_applications (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    job_id uuid NOT NULL,
    applicant_id uuid NOT NULL,
    submission_id uuid,
    status text DEFAULT 'applied'::text NOT NULL,
    employer_notes text,
    applied_at timestamp with time zone DEFAULT now() NOT NULL,
    status_updated_at timestamp with time zone,
    CONSTRAINT job_applications_status_check CHECK ((status = ANY (ARRAY['applied'::text, 'reviewing'::text, 'shortlisted'::text, 'rejected'::text, 'offered'::text, 'withdrawn'::text])))
);


--
-- Name: jwt_signing_keys; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.jwt_signing_keys (
    kid text NOT NULL,
    secret text NOT NULL,
    active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    retired_at timestamp with time zone
);


--
-- Name: known_bots; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.known_bots (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    user_agent_pattern text NOT NULL,
    bot_type text DEFAULT 'unknown'::text NOT NULL,
    policy text DEFAULT 'allow'::text NOT NULL,
    is_system boolean DEFAULT false NOT NULL,
    crawl_delay integer DEFAULT 0 NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: language_score_stories; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.language_score_stories (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    test_id uuid NOT NULL,
    starting_level text,
    final_score text NOT NULL,
    destination_space_id uuid,
    prep_duration_weeks integer,
    resources_used text[],
    advice text,
    status text DEFAULT 'published'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);


--
-- Name: language_test_requirements; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.language_test_requirements (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    destination_space_id uuid NOT NULL,
    visa_category text,
    test_id uuid NOT NULL,
    minimum_score text NOT NULL,
    score_breakdown jsonb,
    notes text,
    source_url text,
    status text DEFAULT 'published'::text NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    ai_generated boolean DEFAULT false NOT NULL,
    ai_model text,
    ai_prompt_hash text,
    hitl_reviewed_by uuid,
    hitl_reviewed_at timestamp with time zone,
    hitl_review_notes text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    is_japabot_indexed boolean DEFAULT false NOT NULL,
    intel_record_id uuid,
    guide_id uuid,
    confidence_overall real DEFAULT 0 NOT NULL,
    verification_status text DEFAULT 'verified'::text NOT NULL,
    freshness_score integer DEFAULT 100 NOT NULL,
    override_source text DEFAULT 'ingestion'::text NOT NULL,
    editorial_overrides jsonb DEFAULT '{}'::jsonb NOT NULL,
    CONSTRAINT language_test_requirements_override_source_check CHECK ((override_source = ANY (ARRAY['ingestion'::text, 'editorial'::text, 'mixed'::text]))),
    CONSTRAINT language_test_requirements_verification_status_check CHECK ((verification_status = ANY (ARRAY['verified'::text, 'needs_review'::text, 'conflict'::text, 'stale'::text])))
);


--
-- Name: language_test_resources; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.language_test_resources (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    test_id uuid NOT NULL,
    title text NOT NULL,
    resource_url text NOT NULL,
    resource_type text,
    community_rating numeric(3,2),
    rating_count integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: language_tests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.language_tests (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    slug text NOT NULL,
    name text NOT NULL,
    official_url text,
    validity_months integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: learning_certificates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.learning_certificates (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    track_id uuid NOT NULL,
    certificate_number text NOT NULL,
    media_id uuid,
    issued_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: learning_lessons; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.learning_lessons (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    module_id uuid NOT NULL,
    title text NOT NULL,
    slug text NOT NULL,
    lesson_type text DEFAULT 'article'::text NOT NULL,
    content text,
    media_id uuid,
    duration_minutes integer,
    sort_order integer DEFAULT 0 NOT NULL,
    is_published boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    is_preview boolean DEFAULT false NOT NULL,
    CONSTRAINT learning_lessons_slug_url_safe CHECK ((slug ~ '^[a-z0-9-]+$'::text))
);


--
-- Name: COLUMN learning_lessons.is_preview; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.learning_lessons.is_preview IS 'True when the lesson can be previewed without enrollment (ADR-060 access gate).';


--
-- Name: learning_modules; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.learning_modules (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    track_id uuid NOT NULL,
    title text NOT NULL,
    slug text NOT NULL,
    description text,
    sort_order integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT learning_modules_slug_url_safe CHECK ((slug ~ '^[a-z0-9-]+$'::text))
);


--
-- Name: learning_tracks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.learning_tracks (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    title text NOT NULL,
    slug text NOT NULL,
    description text,
    difficulty text DEFAULT 'beginner'::text NOT NULL,
    cover_media_id uuid,
    is_published boolean DEFAULT false NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    instructor_id uuid,
    price_kobo bigint,
    currency text DEFAULT 'NGN'::text NOT NULL,
    is_published_to_university boolean DEFAULT false NOT NULL,
    category text,
    language text DEFAULT 'en'::text NOT NULL,
    total_enrolled integer DEFAULT 0 NOT NULL,
    geo_space_id uuid,
    university_submission_status text DEFAULT 'draft'::text NOT NULL,
    university_submitted_at timestamp with time zone,
    university_reviewed_at timestamp with time zone,
    university_rejection_reason text,
    subscription_eligible boolean DEFAULT false NOT NULL,
    CONSTRAINT learning_tracks_slug_url_safe CHECK ((slug ~ '^[a-z0-9-]+$'::text)),
    CONSTRAINT learning_tracks_university_submission_status_check CHECK ((university_submission_status = ANY (ARRAY['draft'::text, 'pending'::text, 'approved'::text, 'rejected'::text])))
);


--
-- Name: COLUMN learning_tracks.instructor_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.learning_tracks.instructor_id IS 'Owning creator; NULL = platform/staff-authored course (ADR-063).';


--
-- Name: COLUMN learning_tracks.price_kobo; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.learning_tracks.price_kobo IS 'À la carte price; NULL = free or Pro-only.';


--
-- Name: COLUMN learning_tracks.is_published_to_university; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.learning_tracks.is_published_to_university IS 'Marketplace visibility; distinct from is_published (draft/live).';


--
-- Name: COLUMN learning_tracks.university_submission_status; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.learning_tracks.university_submission_status IS 'Marketplace submission lifecycle: draft|pending|approved|rejected (ADR-063 §5).';


--
-- Name: COLUMN learning_tracks.university_rejection_reason; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.learning_tracks.university_rejection_reason IS 'Admin rejection reason; cleared on re-submit (ADR-063 §5).';


--
-- Name: lesson_progress; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.lesson_progress (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    lesson_id uuid NOT NULL,
    status text DEFAULT 'not_started'::text NOT NULL,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: lesson_quizzes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.lesson_quizzes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    lesson_id uuid NOT NULL,
    question text NOT NULL,
    options text NOT NULL,
    answer text NOT NULL,
    explanation text,
    "position" integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: media; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.media (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    uploader_user_id uuid NOT NULL,
    original_filename text NOT NULL,
    mime_type text NOT NULL,
    file_size_bytes bigint NOT NULL,
    width_px integer,
    height_px integer,
    duration_secs numeric,
    storage_path text NOT NULL,
    storage_bucket text DEFAULT 'media'::text NOT NULL,
    alt_text text,
    caption text,
    confirmed boolean DEFAULT false NOT NULL,
    deleted_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    quarantine_reason text,
    quarantined_at timestamp with time zone,
    reviewed_by uuid,
    reviewed_at timestamp with time zone,
    user_group_id uuid,
    source_platform text,
    source_photographer text,
    source_photographer_url text,
    source_license text,
    source_license_url text,
    source_photo_id text,
    rejected_at timestamp with time zone,
    flagged boolean DEFAULT false NOT NULL
);


--
-- Name: COLUMN media.source_platform; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.media.source_platform IS 'Stock provider name: unsplash, pexels, pixabay, openverse';


--
-- Name: COLUMN media.source_photographer; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.media.source_photographer IS 'Photographer display name for attribution';


--
-- Name: COLUMN media.source_photographer_url; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.media.source_photographer_url IS 'Link to photographer profile page';


--
-- Name: COLUMN media.source_license; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.media.source_license IS 'License identifier: cc0, cc-by, cc-by-sa, pixabay';


--
-- Name: COLUMN media.source_license_url; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.media.source_license_url IS 'Link to license deed';


--
-- Name: COLUMN media.source_photo_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.media.source_photo_id IS 'Provider-specific photo ID for dedup';


--
-- Name: mentor_reviews; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mentor_reviews (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    session_id uuid NOT NULL,
    reviewer_id uuid NOT NULL,
    rating integer NOT NULL,
    comment text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT mentor_reviews_rating_check CHECK (((rating >= 1) AND (rating <= 5)))
);


--
-- Name: mentor_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mentor_sessions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    mentor_id uuid NOT NULL,
    mentee_id uuid NOT NULL,
    status text DEFAULT 'scheduled'::text NOT NULL,
    scheduled_at timestamp with time zone NOT NULL,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    notes text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: mentorship_connections; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mentorship_connections (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    mentor_id uuid NOT NULL,
    mentee_id uuid NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    goals text,
    destination_space_id uuid,
    responded_at timestamp with time zone,
    completed_at timestamp with time zone,
    cancelled_at timestamp with time zone,
    cancel_reason text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: mentorship_matches; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mentorship_matches (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    program_id uuid NOT NULL,
    mentor_id uuid NOT NULL,
    mentee_id uuid NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: mentorship_profiles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mentorship_profiles (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    bio text,
    skills jsonb DEFAULT '[]'::jsonb NOT NULL,
    goals jsonb DEFAULT '[]'::jsonb NOT NULL,
    is_mentor boolean DEFAULT true NOT NULL,
    is_available boolean DEFAULT true NOT NULL,
    rating_avg numeric(3,2) DEFAULT 0,
    session_count integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: mentorship_programs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mentorship_programs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: menu_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.menu_items (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    menu_id uuid NOT NULL,
    parent_id uuid,
    label text NOT NULL,
    url text NOT NULL,
    icon text,
    sort_order integer DEFAULT 0 NOT NULL,
    permission_node text,
    visibility text DEFAULT 'all'::text NOT NULL,
    target text DEFAULT '_self'::text NOT NULL,
    is_ad_slot boolean DEFAULT false NOT NULL,
    ad_slot_config jsonb,
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    required_group_ids jsonb DEFAULT '[]'::jsonb NOT NULL,
    feature_flag text,
    CONSTRAINT menu_items_required_group_ids_is_array CHECK ((jsonb_typeof(required_group_ids) = 'array'::text)),
    CONSTRAINT menu_items_status_check CHECK ((status = ANY (ARRAY['active'::text, 'inactive'::text]))),
    CONSTRAINT menu_items_target_check CHECK ((target = ANY (ARRAY['_self'::text, '_blank'::text]))),
    CONSTRAINT menu_items_visibility_check CHECK ((visibility = ANY (ARRAY['all'::text, 'authenticated'::text, 'guest'::text])))
);


--
-- Name: COLUMN menu_items.feature_flag; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.menu_items.feature_flag IS 'Optional feature flag key (feature_flags.key). If set, the item is shown only when the flag is enabled for the tenant.';


--
-- Name: menus; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.menus (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    slug text NOT NULL,
    name text NOT NULL,
    placement text DEFAULT 'sidebar'::text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT menus_placement_check CHECK ((placement = ANY (ARRAY['sidebar'::text, 'bottom_bar'::text, 'dropdown'::text, 'footer'::text, 'header'::text, 'dashboard_rail'::text]))),
    CONSTRAINT menus_status_check CHECK ((status = ANY (ARRAY['active'::text, 'inactive'::text])))
);


--
-- Name: moderation_alerts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.moderation_alerts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    alert_type text NOT NULL,
    severity text DEFAULT 'medium'::text NOT NULL,
    title text NOT NULL,
    description text,
    status text DEFAULT 'open'::text NOT NULL,
    meta jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    acknowledged_at timestamp with time zone,
    resolved_at timestamp with time zone,
    acknowledged_by uuid,
    resolved_by uuid
);


--
-- Name: moderation_appeals; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.moderation_appeals (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    entity_type text NOT NULL,
    entity_id uuid NOT NULL,
    reason text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT moderation_appeals_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'approved'::text, 'rejected'::text])))
);


--
-- Name: monitored_embassies; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.monitored_embassies (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    destination_space_id uuid NOT NULL,
    slug text NOT NULL,
    name text NOT NULL,
    booking_url text NOT NULL,
    poll_interval_seconds integer DEFAULT 900 NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    last_polled_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: mutual_aid_posts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mutual_aid_posts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    post_type text NOT NULL,
    category text NOT NULL,
    title text NOT NULL,
    description text NOT NULL,
    destination_space_id uuid NOT NULL,
    city_space_id uuid,
    expires_at timestamp with time zone NOT NULL,
    is_fulfilled boolean DEFAULT false NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    CONSTRAINT mutual_aid_posts_category_check CHECK ((category = ANY (ARRAY['housing'::text, 'transport'::text, 'documents'::text, 'job_search'::text, 'language'::text, 'social'::text, 'other'::text]))),
    CONSTRAINT mutual_aid_posts_post_type_check CHECK ((post_type = ANY (ARRAY['need'::text, 'offer'::text, 'skill_swap'::text]))),
    CONSTRAINT mutual_aid_posts_status_check CHECK ((status = ANY (ARRAY['active'::text, 'expired'::text, 'fulfilled'::text, 'removed'::text])))
);


--
-- Name: platform_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.platform_events (
    id bigint NOT NULL,
    event_type text NOT NULL,
    user_id uuid,
    entity_type text,
    entity_id uuid,
    meta jsonb DEFAULT '{}'::jsonb NOT NULL,
    country_code text,
    session_id text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    is_synthetic boolean DEFAULT false NOT NULL
);


--
-- Name: mv_daily_content_stats; Type: MATERIALIZED VIEW; Schema: public; Owner: -
--

CREATE MATERIALIZED VIEW public.mv_daily_content_stats AS
 SELECT (date_trunc('day'::text, platform_events.created_at))::date AS day,
    platform_events.entity_id AS content_id,
    count(*) FILTER (WHERE (platform_events.event_type = 'content.viewed'::text)) AS views,
    count(*) FILTER (WHERE (platform_events.event_type = 'reaction.given'::text)) AS reactions,
    count(*) FILTER (WHERE (platform_events.event_type = 'comment.created'::text)) AS comments,
    count(DISTINCT platform_events.user_id) AS unique_viewers
   FROM public.platform_events
  WHERE ((platform_events.event_type = ANY (ARRAY['content.viewed'::text, 'reaction.given'::text, 'comment.created'::text])) AND (platform_events.is_synthetic = false))
  GROUP BY ((date_trunc('day'::text, platform_events.created_at))::date), platform_events.entity_id
  WITH NO DATA;


--
-- Name: mv_daily_space_stats; Type: MATERIALIZED VIEW; Schema: public; Owner: -
--

CREATE MATERIALIZED VIEW public.mv_daily_space_stats AS
 SELECT (date_trunc('day'::text, platform_events.created_at))::date AS day,
    platform_events.entity_id AS space_id,
    count(*) FILTER (WHERE (platform_events.event_type = 'space.joined'::text)) AS joins,
    count(*) FILTER (WHERE (platform_events.event_type = 'content.created'::text)) AS posts,
    count(DISTINCT platform_events.user_id) AS active_users
   FROM public.platform_events
  WHERE ((platform_events.event_type = ANY (ARRAY['space.joined'::text, 'content.created'::text])) AND (platform_events.is_synthetic = false))
  GROUP BY ((date_trunc('day'::text, platform_events.created_at))::date), platform_events.entity_id
  WITH NO DATA;


--
-- Name: payments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.payments (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    provider text NOT NULL,
    provider_ref text,
    purpose_type text NOT NULL,
    purpose_id uuid,
    amount bigint NOT NULL,
    currency text DEFAULT 'NGN'::text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    subscription_id uuid,
    discount_code_id uuid,
    discount_amount bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    idempotency_key text,
    CONSTRAINT payments_provider_check CHECK ((provider = ANY (ARRAY['stripe'::text, 'paystack'::text, 'nowpayments'::text, 'manual_transfer'::text, 'credit'::text]))),
    CONSTRAINT payments_purpose_type_check CHECK ((purpose_type = ANY (ARRAY['pro_membership'::text, 'org_boost'::text, 'donation'::text, 'course_access'::text, 'credit_purchase'::text, 'ad_slot'::text, 'form_submission'::text]))),
    CONSTRAINT payments_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'processing'::text, 'succeeded'::text, 'failed'::text, 'refunded'::text, 'cancelled'::text, 'disputed'::text])))
);


--
-- Name: mv_revenue_streams; Type: MATERIALIZED VIEW; Schema: public; Owner: -
--

CREATE MATERIALIZED VIEW public.mv_revenue_streams AS
 SELECT (date_trunc('month'::text, payments.created_at))::date AS month,
    sum(payments.amount) FILTER (WHERE (payments.purpose_type = 'pro_subscription'::text)) AS subscription_revenue,
    sum(payments.amount) FILTER (WHERE (payments.purpose_type = 'course_access'::text)) AS course_revenue,
    sum(payments.amount) FILTER (WHERE (payments.purpose_type = 'ad_campaign'::text)) AS ad_revenue,
    sum(payments.amount) FILTER (WHERE (payments.purpose_type = 'credit_pack'::text)) AS credit_revenue
   FROM public.payments
  WHERE (payments.status = 'completed'::text)
  GROUP BY ((date_trunc('month'::text, payments.created_at))::date)
  WITH NO DATA;


--
-- Name: mv_top_content_30d; Type: MATERIALIZED VIEW; Schema: public; Owner: -
--

CREATE MATERIALIZED VIEW public.mv_top_content_30d AS
 SELECT mv_daily_content_stats.content_id,
    sum(mv_daily_content_stats.views) AS total_views,
    sum(mv_daily_content_stats.reactions) AS total_reactions,
    sum(mv_daily_content_stats.comments) AS total_comments,
    ((sum(mv_daily_content_stats.views) + (sum(mv_daily_content_stats.reactions) * (3)::numeric)) + (sum(mv_daily_content_stats.comments) * (5)::numeric)) AS engagement_score
   FROM public.mv_daily_content_stats
  WHERE (mv_daily_content_stats.day >= (CURRENT_DATE - 30))
  GROUP BY mv_daily_content_stats.content_id
  ORDER BY ((sum(mv_daily_content_stats.views) + (sum(mv_daily_content_stats.reactions) * (3)::numeric)) + (sum(mv_daily_content_stats.comments) * (5)::numeric)) DESC
  WITH NO DATA;


--
-- Name: mv_top_content_7d; Type: MATERIALIZED VIEW; Schema: public; Owner: -
--

CREATE MATERIALIZED VIEW public.mv_top_content_7d AS
 SELECT mv_daily_content_stats.content_id,
    sum(mv_daily_content_stats.views) AS total_views,
    sum(mv_daily_content_stats.reactions) AS total_reactions,
    sum(mv_daily_content_stats.comments) AS total_comments,
    ((sum(mv_daily_content_stats.views) + (sum(mv_daily_content_stats.reactions) * (3)::numeric)) + (sum(mv_daily_content_stats.comments) * (5)::numeric)) AS engagement_score
   FROM public.mv_daily_content_stats
  WHERE (mv_daily_content_stats.day >= (CURRENT_DATE - 7))
  GROUP BY mv_daily_content_stats.content_id
  ORDER BY ((sum(mv_daily_content_stats.views) + (sum(mv_daily_content_stats.reactions) * (3)::numeric)) + (sum(mv_daily_content_stats.comments) * (5)::numeric)) DESC
  WITH NO DATA;


--
-- Name: mv_top_content_90d; Type: MATERIALIZED VIEW; Schema: public; Owner: -
--

CREATE MATERIALIZED VIEW public.mv_top_content_90d AS
 SELECT mv_daily_content_stats.content_id,
    sum(mv_daily_content_stats.views) AS total_views,
    sum(mv_daily_content_stats.reactions) AS total_reactions,
    sum(mv_daily_content_stats.comments) AS total_comments,
    ((sum(mv_daily_content_stats.views) + (sum(mv_daily_content_stats.reactions) * (3)::numeric)) + (sum(mv_daily_content_stats.comments) * (5)::numeric)) AS engagement_score
   FROM public.mv_daily_content_stats
  WHERE (mv_daily_content_stats.day >= (CURRENT_DATE - 90))
  GROUP BY mv_daily_content_stats.content_id
  ORDER BY ((sum(mv_daily_content_stats.views) + (sum(mv_daily_content_stats.reactions) * (3)::numeric)) + (sum(mv_daily_content_stats.comments) * (5)::numeric)) DESC
  WITH NO DATA;


--
-- Name: mv_user_engagement_stats; Type: MATERIALIZED VIEW; Schema: public; Owner: -
--

CREATE MATERIALIZED VIEW public.mv_user_engagement_stats AS
 SELECT (date_trunc('day'::text, platform_events.created_at))::date AS day,
    platform_events.user_id,
    count(*) FILTER (WHERE (platform_events.event_type = 'content.created'::text)) AS posts_created,
    count(*) FILTER (WHERE (platform_events.event_type = 'reaction.given'::text)) AS reactions_given,
    count(*) FILTER (WHERE (platform_events.event_type = 'comment.created'::text)) AS comments_made,
    count(*) FILTER (WHERE (platform_events.event_type = 'content.viewed'::text)) AS views
   FROM public.platform_events
  WHERE ((platform_events.user_id IS NOT NULL) AND (platform_events.is_synthetic = false))
  GROUP BY ((date_trunc('day'::text, platform_events.created_at))::date), platform_events.user_id
  WITH NO DATA;


--
-- Name: newsletter_campaigns; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.newsletter_campaigns (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    subject text NOT NULL,
    html text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    sent_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    preview_text text,
    body jsonb DEFAULT '{}'::jsonb NOT NULL,
    segment text DEFAULT 'all'::text NOT NULL,
    segment_id uuid,
    scheduled_at timestamp with time zone,
    recipient_count integer DEFAULT 0 NOT NULL,
    open_count integer DEFAULT 0 NOT NULL,
    click_count integer DEFAULT 0 NOT NULL,
    created_by uuid,
    CONSTRAINT newsletter_campaigns_segment_check CHECK ((segment = ANY (ARRAY['all'::text, 'space'::text, 'tag'::text, 'pro_subscribers'::text]))),
    CONSTRAINT newsletter_campaigns_status_check CHECK ((status = ANY (ARRAY['draft'::text, 'scheduled'::text, 'sending'::text, 'sent'::text, 'cancelled'::text])))
);


--
-- Name: newsletter_list_subscribers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.newsletter_list_subscribers (
    list_id uuid NOT NULL,
    subscriber_id uuid NOT NULL
);


--
-- Name: newsletter_lists; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.newsletter_lists (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    description text,
    is_default boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: newsletter_send_state; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.newsletter_send_state (
    campaign_id uuid NOT NULL,
    recipient text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    sent_at timestamp with time zone,
    error text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: newsletter_subscribers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.newsletter_subscribers (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    email text NOT NULL,
    user_id uuid,
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    digest_enabled boolean DEFAULT true NOT NULL,
    campaign_enabled boolean DEFAULT true NOT NULL,
    subscribed_at timestamp with time zone DEFAULT now() NOT NULL,
    unsubscribed_at timestamp with time zone,
    source text DEFAULT 'signup'::text NOT NULL,
    CONSTRAINT newsletter_subscribers_source_check CHECK ((source = ANY (ARRAY['signup'::text, 'onboarding'::text, 'footer'::text, 'import'::text, 'api'::text]))),
    CONSTRAINT newsletter_subscribers_status_check CHECK ((status = ANY (ARRAY['subscribed'::text, 'unsubscribed'::text, 'bounced'::text, 'complained'::text])))
);


--
-- Name: newsletter_unsubscribe_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.newsletter_unsubscribe_tokens (
    token text NOT NULL,
    subscriber_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: notification_batch_windows; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.notification_batch_windows (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    recipient_id uuid NOT NULL,
    notification_type text NOT NULL,
    subject_type text NOT NULL,
    subject_id uuid NOT NULL,
    actor_ids uuid[] DEFAULT '{}'::uuid[] NOT NULL,
    actor_count integer DEFAULT 1 NOT NULL,
    last_actor_id uuid,
    window_open_at timestamp with time zone DEFAULT now() NOT NULL,
    window_close_at timestamp with time zone NOT NULL,
    dispatched boolean DEFAULT false NOT NULL
);


--
-- Name: notification_preferences; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.notification_preferences (
    user_id uuid NOT NULL,
    notification_type text NOT NULL,
    in_app boolean DEFAULT true NOT NULL,
    email boolean DEFAULT false NOT NULL,
    push boolean DEFAULT false NOT NULL
);


--
-- Name: notification_types; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.notification_types (
    key text NOT NULL,
    label text NOT NULL,
    description text,
    enabled boolean DEFAULT true NOT NULL,
    default_in_app boolean DEFAULT true NOT NULL,
    default_email boolean DEFAULT false NOT NULL,
    default_push boolean DEFAULT false NOT NULL,
    category text DEFAULT 'system'::text NOT NULL
);


--
-- Name: notifications; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.notifications (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    recipient_id uuid NOT NULL,
    notification_type text NOT NULL,
    actor_type text,
    actor_id uuid,
    subject_type text,
    subject_id uuid,
    title text NOT NULL,
    body text,
    action_url text,
    read_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: oauth_accounts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.oauth_accounts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    provider text NOT NULL,
    provider_user_id text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: onboarding_state; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.onboarding_state (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    current_step integer DEFAULT 1 NOT NULL,
    completed boolean DEFAULT false NOT NULL,
    step_data jsonb DEFAULT '{}'::jsonb NOT NULL,
    tour_dismissed boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: onboarding_templates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.onboarding_templates (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    description text,
    steps jsonb DEFAULT '[]'::jsonb NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    is_system boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: organization_boosts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.organization_boosts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    organization_id uuid NOT NULL,
    campaign_id uuid NOT NULL,
    boost_type text DEFAULT 'featured_placement'::text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    start_date timestamp with time zone DEFAULT now() NOT NULL,
    end_date timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: organization_invitations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.organization_invitations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    organization_id uuid NOT NULL,
    email text NOT NULL,
    role text DEFAULT 'member'::text NOT NULL,
    invited_by uuid,
    token_hash text NOT NULL,
    expires_at timestamp with time zone DEFAULT (now() + '7 days'::interval) NOT NULL,
    accepted_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT organization_invitations_role_check CHECK ((role = ANY (ARRAY['admin'::text, 'editor'::text, 'member'::text])))
);


--
-- Name: organization_members; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.organization_members (
    organization_id uuid NOT NULL,
    user_id uuid NOT NULL,
    role text DEFAULT 'member'::text NOT NULL,
    invited_by uuid,
    joined_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT organization_members_role_check CHECK ((role = ANY (ARRAY['owner'::text, 'admin'::text, 'editor'::text, 'member'::text])))
);


--
-- Name: organizations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.organizations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_user_id uuid NOT NULL,
    name text NOT NULL,
    slug text NOT NULL,
    tagline text,
    description text,
    logo_media_id uuid,
    header_media_id uuid,
    website_url text,
    country_code text,
    geo_space_id uuid,
    org_type text DEFAULT 'business'::text NOT NULL,
    verified boolean DEFAULT false NOT NULL,
    verification_type text,
    follower_count integer DEFAULT 0 NOT NULL,
    member_count integer DEFAULT 0 NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    field_values jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    source_id uuid,
    CONSTRAINT organizations_status_check CHECK ((status = ANY (ARRAY['active'::text, 'suspended'::text, 'pending_verification'::text])))
);


--
-- Name: ownership_claims; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ownership_claims (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    claimant_user_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    evidence_url text,
    notes text,
    status text DEFAULT 'pending'::text NOT NULL,
    reviewed_by uuid,
    resolution_note text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    reviewed_at timestamp with time zone,
    form_submission_id uuid,
    CONSTRAINT ownership_claims_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'approved'::text, 'rejected'::text, 'withdrawn'::text])))
);


--
-- Name: page_builder_pages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.page_builder_pages (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    title text NOT NULL,
    slug text NOT NULL,
    description text,
    layout_data jsonb DEFAULT '{}'::jsonb NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    created_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: pagebuilder_blocks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pagebuilder_blocks (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    slug text NOT NULL,
    category text DEFAULT 'general'::text NOT NULL,
    component text NOT NULL,
    default_config jsonb DEFAULT '{}'::jsonb,
    config_schema jsonb DEFAULT '{}'::jsonb,
    preview_image text,
    is_system boolean DEFAULT false NOT NULL,
    is_enabled boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: pagebuilder_templates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pagebuilder_templates (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    slug text NOT NULL,
    description text,
    page_type text DEFAULT 'landing'::text NOT NULL,
    blocks jsonb DEFAULT '[]'::jsonb NOT NULL,
    is_system boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    thumbnail_media_id uuid,
    created_by uuid,
    schema_version integer DEFAULT 1 NOT NULL
);


--
-- Name: pagebuilder_themes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pagebuilder_themes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    slug text NOT NULL,
    description text,
    colors jsonb DEFAULT '{}'::jsonb,
    typography jsonb DEFAULT '{}'::jsonb,
    spacing jsonb DEFAULT '{}'::jsonb,
    is_default boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: pages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pages (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    title text NOT NULL,
    slug text NOT NULL,
    description text,
    template_id uuid,
    theme_id uuid,
    blocks jsonb DEFAULT '[]'::jsonb NOT NULL,
    meta jsonb DEFAULT '{}'::jsonb,
    status text DEFAULT 'draft'::text NOT NULL,
    published_at timestamp with time zone,
    created_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    page_type text DEFAULT 'page'::text NOT NULL,
    CONSTRAINT pages_page_type_check CHECK ((page_type = ANY (ARRAY['page'::text, 'sidebar'::text, 'landing'::text, 'about'::text, 'service'::text, 'event'::text, 'custom'::text]))),
    CONSTRAINT pages_status_check CHECK ((status = ANY (ARRAY['draft'::text, 'published'::text, 'archived'::text])))
);


--
-- Name: password_history; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.password_history (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    password_hash text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: password_reset_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.password_reset_tokens (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: payment_refunds; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.payment_refunds (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    payment_id uuid NOT NULL,
    amount bigint NOT NULL,
    reason text,
    status text DEFAULT 'pending'::text NOT NULL,
    provider_ref text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: payment_webhook_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.payment_webhook_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    provider text NOT NULL,
    event_id text NOT NULL,
    raw_payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    processed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    event_type text DEFAULT ''::text NOT NULL
);


--
-- Name: payout_providers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.payout_providers (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    type text NOT NULL,
    name text NOT NULL,
    is_active boolean DEFAULT false NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    supported_currencies text[] DEFAULT '{}'::text[] NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: permissions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.permissions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    key text NOT NULL,
    description text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: persona_voices; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.persona_voices (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    slug text NOT NULL,
    description text,
    voice_prompt text NOT NULL,
    is_system boolean DEFAULT false NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: planning_group_checkins; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.planning_group_checkins (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    group_id uuid NOT NULL,
    user_id uuid NOT NULL,
    note text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: planning_group_members; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.planning_group_members (
    group_id uuid NOT NULL,
    user_id uuid NOT NULL,
    departure_date date,
    joined_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: planning_group_milestones; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.planning_group_milestones (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    group_id uuid NOT NULL,
    user_id uuid NOT NULL,
    title text NOT NULL,
    completed boolean DEFAULT false NOT NULL,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: planning_groups; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.planning_groups (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    description text,
    destination_space_id uuid NOT NULL,
    visa_category text,
    target_departure_window text,
    target_departure_at date,
    max_members integer DEFAULT 50 NOT NULL,
    is_open_join boolean DEFAULT true NOT NULL,
    owner_id uuid NOT NULL,
    expires_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);


--
-- Name: platform_data_exports; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.platform_data_exports (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    export_type text NOT NULL,
    format text DEFAULT 'csv'::text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    file_url text,
    file_size bigint,
    include_metadata boolean DEFAULT true NOT NULL,
    error_message text,
    created_by uuid,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: platform_data_imports; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.platform_data_imports (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    import_type text NOT NULL,
    format text DEFAULT 'csv'::text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    file_url text NOT NULL,
    total_rows integer DEFAULT 0 NOT NULL,
    imported_rows integer DEFAULT 0 NOT NULL,
    failed_rows integer DEFAULT 0 NOT NULL,
    error_log jsonb DEFAULT '[]'::jsonb NOT NULL,
    mode text DEFAULT 'create'::text NOT NULL,
    created_by uuid,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: platform_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.platform_events ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.platform_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: pof_requirements; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pof_requirements (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    destination_space_id uuid NOT NULL,
    visa_type text NOT NULL,
    program_label text NOT NULL,
    amount_schedule jsonb NOT NULL,
    currency character(3) NOT NULL,
    funds_must_be_liquid boolean DEFAULT true NOT NULL,
    accepted_proof jsonb,
    notes text,
    effective_date date NOT NULL,
    source_url text,
    source_url_last_checked date,
    status text DEFAULT 'published'::text NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    ai_generated boolean DEFAULT false NOT NULL,
    ai_model text,
    ai_prompt_hash text,
    hitl_reviewed_by uuid,
    hitl_reviewed_at timestamp with time zone,
    hitl_review_notes text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    is_japabot_indexed boolean GENERATED ALWAYS AS (((status = 'published'::text) AND (is_active = true))) STORED
);


--
-- Name: professional_documents; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.professional_documents (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    title text NOT NULL,
    document_type text NOT NULL,
    description text,
    file_url text,
    verified boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: professional_relocations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.professional_relocations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    country text NOT NULL,
    city text,
    status text DEFAULT 'planned'::text NOT NULL,
    year integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    country_code text
);


--
-- Name: professional_verifiers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.professional_verifiers (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    role text,
    status text DEFAULT 'pending'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: push_subscriptions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.push_subscriptions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    endpoint text NOT NULL,
    p256dh text NOT NULL,
    auth_key text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: push_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.push_tokens (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    token text NOT NULL,
    platform text NOT NULL,
    device_name text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    last_used_at timestamp with time zone,
    CONSTRAINT push_tokens_platform_check CHECK ((platform = ANY (ARRAY['fcm'::text, 'apns'::text, 'web'::text])))
);


--
-- Name: reaction_types; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reaction_types (
    key character varying(50) NOT NULL,
    emoji text NOT NULL,
    label text NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    "position" integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: reactions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reactions (
    entity_type text NOT NULL,
    entity_id uuid NOT NULL,
    user_id uuid NOT NULL,
    reaction_key character varying(50) NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: redirects; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.redirects (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    source_path text NOT NULL,
    target_url text NOT NULL,
    http_status smallint DEFAULT 301 NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    notes text,
    hit_count integer DEFAULT 0 NOT NULL,
    last_hit_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT redirects_http_status_check CHECK ((http_status = ANY (ARRAY[301, 302, 307])))
);


--
-- Name: referral_codes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.referral_codes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    code text NOT NULL,
    click_count integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: referral_programs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.referral_programs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    referrer_reward_cents integer DEFAULT 500 NOT NULL,
    referee_reward_cents integer DEFAULT 500 NOT NULL,
    max_referrals integer DEFAULT 0 NOT NULL,
    cookie_days integer DEFAULT 30 NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: referrals; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.referrals (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    referrer_id uuid NOT NULL,
    referee_id uuid,
    referral_code text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    reward_paid_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    qualified_payment_id uuid,
    reward_amount numeric(12,2),
    flagged_reason text,
    qualified_at timestamp with time zone,
    credited_at timestamp with time zone,
    CONSTRAINT referrals_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'qualified'::text, 'credited'::text, 'flagged'::text, 'rejected'::text])))
);


--
-- Name: refresh_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.refresh_tokens (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    device_name text,
    ip_address text,
    last_used_at timestamp with time zone
);


--
-- Name: remittance_providers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.remittance_providers (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    slug text NOT NULL,
    logo_media_id uuid,
    website_url text,
    data_source text NOT NULL,
    api_config jsonb,
    is_active boolean DEFAULT true NOT NULL,
    display_order integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT remittance_providers_data_source_check CHECK ((data_source = ANY (ARRAY['api'::text, 'manual'::text])))
);


--
-- Name: remittance_rate_alerts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.remittance_rate_alerts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    send_currency text NOT NULL,
    receive_currency text NOT NULL,
    target_rate numeric(18,6) NOT NULL,
    direction text DEFAULT 'above'::text NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    last_triggered_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT remittance_rate_alerts_direction_check CHECK ((direction = ANY (ARRAY['above'::text, 'below'::text])))
);


--
-- Name: remittance_rates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.remittance_rates (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    provider_id uuid NOT NULL,
    send_currency text NOT NULL,
    receive_currency text NOT NULL,
    rate numeric(18,6) NOT NULL,
    fee_flat numeric(10,2),
    fee_pct numeric(5,4),
    transfer_time text,
    fetched_at timestamp with time zone DEFAULT now() NOT NULL,
    is_manual boolean DEFAULT false NOT NULL,
    source_url text,
    intel_record_id uuid,
    confidence_overall real DEFAULT 0 NOT NULL,
    verification_status text DEFAULT 'verified'::text NOT NULL,
    freshness_score integer DEFAULT 100 NOT NULL,
    override_source text DEFAULT 'ingestion'::text NOT NULL,
    editorial_overrides jsonb DEFAULT '{}'::jsonb NOT NULL,
    CONSTRAINT remittance_rates_override_source_check CHECK ((override_source = ANY (ARRAY['ingestion'::text, 'editorial'::text, 'mixed'::text]))),
    CONSTRAINT remittance_rates_verification_status_check CHECK ((verification_status = ANY (ARRAY['verified'::text, 'needs_review'::text, 'conflict'::text, 'stale'::text])))
);


--
-- Name: reply_views; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reply_views (
    reply_id uuid NOT NULL,
    user_id uuid NOT NULL,
    viewed_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: revenue_splits; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.revenue_splits (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    instructor_id uuid,
    track_id uuid NOT NULL,
    enrollment_id uuid,
    payment_id uuid,
    payout_id uuid,
    source text DEFAULT 'purchase'::text NOT NULL,
    gross_kobo bigint DEFAULT 0 NOT NULL,
    creator_pct numeric(5,2) NOT NULL,
    platform_pct numeric(5,2) NOT NULL,
    creator_kobo bigint DEFAULT 0 NOT NULL,
    platform_kobo bigint DEFAULT 0 NOT NULL,
    currency text DEFAULT 'NGN'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT revenue_splits_source_check CHECK ((source = ANY (ARRAY['purchase'::text, 'pro_stream'::text])))
);


--
-- Name: TABLE revenue_splits; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.revenue_splits IS 'Immutable per-event revenue split record; aggregated into instructor_payouts (ADR-063).';


--
-- Name: COLUMN revenue_splits.instructor_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.revenue_splits.instructor_id IS 'Owning instructor; NULL after GDPR erasure (anonymized, regulatory retention).';


--
-- Name: review_aggregates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.review_aggregates (
    entity_type text NOT NULL,
    entity_id uuid NOT NULL,
    review_count integer DEFAULT 0 NOT NULL,
    average_rating numeric(3,2) DEFAULT 0 NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: reviews; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reviews (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    entity_type text NOT NULL,
    entity_id uuid NOT NULL,
    reviewer_user_id uuid NOT NULL,
    rating smallint NOT NULL,
    title text,
    body text,
    status text DEFAULT 'published'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    removed_by uuid,
    CONSTRAINT reviews_rating_check CHECK (((rating >= 1) AND (rating <= 5))),
    CONSTRAINT reviews_status_check CHECK ((status = ANY (ARRAY['published'::text, 'flagged'::text, 'removed'::text])))
);


--
-- Name: role_permissions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.role_permissions (
    role_id uuid NOT NULL,
    permission_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: saved_scholarships; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.saved_scholarships (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    guide_id uuid NOT NULL,
    status text DEFAULT 'saved'::text NOT NULL,
    notes text,
    remind_before_days integer DEFAULT 14 NOT NULL,
    last_reminded_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT saved_scholarships_status_check CHECK ((status = ANY (ARRAY['saved'::text, 'researching'::text, 'applied'::text, 'accepted'::text, 'rejected'::text, 'withdrawn'::text])))
);


--
-- Name: scam_alert_subscriptions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.scam_alert_subscriptions (
    user_id uuid NOT NULL,
    destination_space_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: scam_reports; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.scam_reports (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    reporter_id uuid,
    scam_type text NOT NULL,
    destination_space_id uuid NOT NULL,
    origin_space_id uuid,
    origin_community text,
    platform_used text,
    approximate_date date,
    amount_range text,
    description text NOT NULL,
    reported_to_authorities boolean DEFAULT false NOT NULL,
    status text DEFAULT 'under_review'::text NOT NULL,
    moderator_note text,
    is_pinned boolean DEFAULT false NOT NULL,
    pinned_space_id uuid,
    reviewed_by uuid,
    reviewed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    ai_generated boolean DEFAULT false NOT NULL,
    ai_model text,
    ai_generated_at timestamp with time zone,
    hitl_reviewed_by uuid,
    hitl_reviewed_at timestamp with time zone,
    CONSTRAINT scam_reports_platform_used_check CHECK (((platform_used IS NULL) OR (platform_used = ANY (ARRAY['whatsapp'::text, 'instagram'::text, 'linkedin'::text, 'email'::text, 'website'::text, 'other'::text])))),
    CONSTRAINT scam_reports_scam_type_check CHECK ((scam_type = ANY (ARRAY['fraudulent_agent'::text, 'fake_job'::text, 'housing_fraud'::text, 'fake_scholarship'::text, 'whatsapp_telegram'::text, 'other'::text]))),
    CONSTRAINT scam_reports_status_check CHECK ((status = ANY (ARRAY['under_review'::text, 'confirmed'::text, 'debunked'::text, 'duplicate'::text])))
);
-- Name: scholarships; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.scholarships (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    intel_record_id uuid,
    guide_id uuid,
    source_url text NOT NULL,
    source_type text NOT NULL,
    trust_level smallint NOT NULL,
    title text NOT NULL,
    description text,
    country text NOT NULL,
    country_space_id uuid,
    university text,
    degree_level text,
    fields_of_study text[] DEFAULT '{}'::text[] NOT NULL,
    eligible_countries text[] DEFAULT '{}'::text[] NOT NULL,
    funding jsonb DEFAULT '{}'::jsonb NOT NULL,
    deadline date,
    application_url text,
    required_documents text[] DEFAULT '{}'::text[] NOT NULL,
    minimum_gpa numeric(3,2),
    language_requirements jsonb DEFAULT '{}'::jsonb NOT NULL,
    confidence_overall real NOT NULL,
    verification_status text DEFAULT 'verified'::text NOT NULL,
    freshness_score integer DEFAULT 100 NOT NULL,
    override_source text DEFAULT 'ingestion'::text NOT NULL,
    editorial_overrides jsonb DEFAULT '{}'::jsonb NOT NULL,
    last_verified_at timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT scholarships_override_source_check CHECK ((override_source = ANY (ARRAY['ingestion'::text, 'editorial'::text, 'mixed'::text]))),
    CONSTRAINT scholarships_verification_status_check CHECK ((verification_status = ANY (ARRAY['verified'::text, 'needs_review'::text, 'conflict'::text, 'stale'::text])))
);


--
-- Name: search_analytics; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.search_analytics (
    id bigint NOT NULL,
    query text NOT NULL,
    results_count integer DEFAULT 0 NOT NULL,
    user_id uuid,
    content_type text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: search_analytics_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.search_analytics ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.search_analytics_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: security_scan_results; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.security_scan_results (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    scan_type text NOT NULL,
    severity text DEFAULT 'info'::text NOT NULL,
    finding text NOT NULL,
    meta jsonb DEFAULT '{}'::jsonb NOT NULL,
    resolved_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: service_listing_meta; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.service_listing_meta (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    content_id uuid NOT NULL,
    org_id uuid,
    service_category text NOT NULL,
    destination_space_id uuid,
    city_space_id uuid,
    is_remote boolean DEFAULT false NOT NULL,
    website_url text,
    booking_url text,
    price_range text,
    is_verified boolean DEFAULT false NOT NULL,
    verification_type text,
    claim_status text DEFAULT 'unclaimed'::text NOT NULL,
    claimed_by_org_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    origin_communities text[] DEFAULT ARRAY[]::text[] NOT NULL,
    verified_by uuid,
    verified_at timestamp with time zone
);


--
-- Name: share_platforms; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.share_platforms (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    key text NOT NULL,
    label text NOT NULL,
    icon text DEFAULT ''::text NOT NULL,
    url_template text NOT NULL,
    color text DEFAULT ''::text NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    "position" integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: sidebar_widgets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sidebar_widgets (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    sidebar_id uuid NOT NULL,
    widget_type text NOT NULL,
    label text DEFAULT ''::text NOT NULL,
    props jsonb DEFAULT '{}'::jsonb NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    visibility text DEFAULT 'all'::text NOT NULL,
    required_group_ids jsonb DEFAULT '[]'::jsonb NOT NULL,
    permission_node text,
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT sidebar_blocks_status_check CHECK ((status = ANY (ARRAY['active'::text, 'inactive'::text]))),
    CONSTRAINT sidebar_blocks_visibility_check CHECK ((visibility = ANY (ARRAY['all'::text, 'authenticated'::text, 'guest'::text])))
);


--
-- Name: sidebars; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sidebars (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    app text NOT NULL,
    "position" text NOT NULL,
    slug text NOT NULL,
    name text NOT NULL,
    page_pattern text DEFAULT '*'::text NOT NULL,
    visibility text DEFAULT 'all'::text NOT NULL,
    required_group_ids jsonb DEFAULT '[]'::jsonb NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT sidebars_app_check CHECK ((app = ANY (ARRAY['main'::text, 'docs'::text, 'learn'::text]))),
    CONSTRAINT sidebars_page_pattern_check CHECK ((page_pattern <> ''::text)),
    CONSTRAINT sidebars_position_check CHECK (("position" = ANY (ARRAY['left'::text, 'right'::text]))),
    CONSTRAINT sidebars_status_check CHECK ((status = ANY (ARRAY['active'::text, 'inactive'::text]))),
    CONSTRAINT sidebars_visibility_check CHECK ((visibility = ANY (ARRAY['all'::text, 'authenticated'::text, 'guest'::text])))
);


--
-- Name: slug_history; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.slug_history (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    entity_type text NOT NULL,
    entity_id uuid NOT NULL,
    old_slug text NOT NULL,
    new_slug text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: sop_drafts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sop_drafts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    destination_space_id uuid,
    purpose text NOT NULL,
    program text,
    institution text,
    title text,
    inputs jsonb NOT NULL,
    generated_text text,
    status text DEFAULT 'queued'::text NOT NULL,
    ai_generated boolean DEFAULT true NOT NULL,
    ai_model text,
    ai_prompt_hash text,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);


--
-- Name: sop_samples; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sop_samples (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    destination_space_id uuid NOT NULL,
    program_level text NOT NULL,
    field_of_study text,
    title text NOT NULL,
    body text NOT NULL,
    word_count integer,
    status text DEFAULT 'draft'::text NOT NULL,
    ai_generated boolean DEFAULT false NOT NULL,
    ai_model text,
    ai_prompt_hash text,
    hitl_reviewed_by uuid,
    hitl_reviewed_at timestamp with time zone,
    hitl_review_notes text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    is_japabot_indexed boolean GENERATED ALWAYS AS ((status = 'published'::text)) STORED
);


--
-- Name: sop_templates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sop_templates (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    destination_space_id uuid,
    purpose text NOT NULL,
    title text NOT NULL,
    structure jsonb NOT NULL,
    status text DEFAULT 'published'::text NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    ai_generated boolean DEFAULT false NOT NULL,
    ai_model text,
    ai_prompt_hash text,
    hitl_reviewed_by uuid,
    hitl_reviewed_at timestamp with time zone,
    hitl_review_notes text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: source_collection_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.source_collection_items (
    collection_id uuid NOT NULL,
    source_id uuid NOT NULL,
    "position" integer DEFAULT 0 NOT NULL
);


--
-- Name: source_collections; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.source_collections (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    slug text NOT NULL,
    name text NOT NULL,
    description text,
    collection_type text NOT NULL,
    query_json jsonb,
    sort text,
    "limit" integer DEFAULT 10 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT source_collections_collection_type_check CHECK ((collection_type = ANY (ARRAY['manual'::text, 'query'::text])))
);


--
-- Name: space_members; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.space_members (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    space_id uuid NOT NULL,
    user_id uuid NOT NULL,
    role text DEFAULT 'member'::text NOT NULL,
    joined_at timestamp with time zone DEFAULT now() NOT NULL,
    status text DEFAULT 'approved'::text NOT NULL
);


--
-- Name: space_moderators; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.space_moderators (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    space_id uuid NOT NULL,
    user_id uuid NOT NULL,
    assigned_by text DEFAULT 'site_admin'::text NOT NULL,
    assigned_by_user_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: space_mutes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.space_mutes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    space_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: space_parent_spaces; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.space_parent_spaces (
    space_id uuid NOT NULL,
    parent_space_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT space_parent_spaces_check CHECK ((space_id <> parent_space_id))
);


--
-- Name: space_permission_presets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.space_permission_presets (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    visibility text NOT NULL,
    posting_perm text NOT NULL,
    replying_perm text NOT NULL,
    commenting_perm text NOT NULL,
    joining_perm text NOT NULL,
    is_system boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: space_pinned_posts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.space_pinned_posts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    space_id uuid NOT NULL,
    content_id uuid NOT NULL,
    pinned_by uuid NOT NULL,
    "position" integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: space_rules; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.space_rules (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    space_id uuid NOT NULL,
    title text NOT NULL,
    body text NOT NULL,
    "position" integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: space_types; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.space_types (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    slug text NOT NULL,
    description text,
    icon text,
    field_group_id uuid,
    is_system boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    category text
);


--
-- Name: spaces; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.spaces (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    parent_id uuid,
    space_type_id uuid NOT NULL,
    name text NOT NULL,
    slug text NOT NULL,
    description text,
    wiki_content jsonb,
    logo_media_id uuid,
    header_image_id uuid,
    geo_code text,
    level text NOT NULL,
    member_count integer DEFAULT 0 NOT NULL,
    post_count integer DEFAULT 0 NOT NULL,
    follower_count integer DEFAULT 0 NOT NULL,
    visibility text DEFAULT 'public'::text NOT NULL,
    posting_perm text DEFAULT 'members'::text NOT NULL,
    replying_perm text DEFAULT 'members'::text NOT NULL,
    commenting_perm text DEFAULT 'members'::text NOT NULL,
    joining_perm text DEFAULT 'open'::text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    currency_code text,
    keywords text[] DEFAULT '{}'::text[] NOT NULL,
    creator_user_id uuid,
    region text,
    subregion text,
    source_id uuid
);


--
-- Name: stock_providers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.stock_providers (
    provider_name text NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    api_key_encrypted text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: story_meta; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.story_meta (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    content_id uuid NOT NULL,
    origin_space_id uuid,
    destination_space_id uuid NOT NULL,
    relocation_year smallint,
    visa_category text,
    profession text,
    biggest_challenge text,
    most_useful_resource text,
    best_advice text,
    verified boolean DEFAULT false NOT NULL,
    ai_generated boolean DEFAULT false NOT NULL,
    ai_model text,
    hitl_reviewed_by uuid,
    ai_generated_at timestamp with time zone,
    hitl_reviewed_at timestamp with time zone,
    is_japabot_indexed boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    ai_prompt_hash text,
    hitl_review_notes text
);


--
-- Name: studio_batches; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.studio_batches (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    description text,
    created_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: studio_jobs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.studio_jobs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    batch_id uuid,
    persona_id uuid NOT NULL,
    content_type text NOT NULL,
    guide_category text,
    topic_prompt text NOT NULL,
    target_space_id uuid,
    target_tags text[],
    status text DEFAULT 'queued'::text NOT NULL,
    draft_content_id uuid,
    review_id uuid,
    generated_at timestamp with time zone,
    reviewed_at timestamp with time zone,
    published_at timestamp with time zone,
    error_message text,
    created_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    intel_record_id uuid,
    ai_generated boolean DEFAULT true NOT NULL,
    ai_model text,
    ai_generated_at timestamp with time zone,
    hitl_reviewed_by uuid,
    hitl_reviewed_at timestamp with time zone,
    voice_id uuid,
    CONSTRAINT studio_jobs_status_check CHECK ((status = ANY (ARRAY['queued'::text, 'generating'::text, 'draft_ready'::text, 'in_review'::text, 'approved'::text, 'published'::text, 'rejected'::text, 'failed'::text])))
);


--
-- Name: subscription_plan_tracks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.subscription_plan_tracks (
    plan_id uuid NOT NULL,
    track_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: subscription_plans; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.subscription_plans (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    slug text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    features jsonb DEFAULT '[]'::jsonb NOT NULL,
    monthly_amount bigint DEFAULT 0 NOT NULL,
    yearly_amount bigint DEFAULT 0 NOT NULL,
    currency text DEFAULT 'NGN'::text NOT NULL,
    highlighted boolean DEFAULT false NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    tier text,
    CONSTRAINT subscription_plans_tier_check CHECK ((tier = ANY (ARRAY['regular'::text, 'pro'::text, 'business'::text])))
);


--
-- Name: subscriptions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.subscriptions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    plan text NOT NULL,
    provider text NOT NULL,
    provider_ref text,
    status text DEFAULT 'active'::text NOT NULL,
    current_period_start timestamp with time zone DEFAULT now() NOT NULL,
    current_period_end timestamp with time zone NOT NULL,
    cancel_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    provider_sub_id text,
    cancel_at_period_end boolean DEFAULT false NOT NULL,
    cancelled_at timestamp with time zone,
    CONSTRAINT subscriptions_plan_check CHECK ((plan = ANY (ARRAY['pro_monthly'::text, 'pro_annual'::text, 'pro_yearly'::text, 'business_monthly'::text, 'business_yearly'::text]))),
    CONSTRAINT subscriptions_provider_check CHECK ((provider = ANY (ARRAY['stripe'::text, 'paystack'::text]))),
    CONSTRAINT subscriptions_status_check CHECK ((status = ANY (ARRAY['active'::text, 'past_due'::text, 'cancelled'::text, 'canceled'::text, 'expired'::text, 'trialing'::text])))
);


--
-- Name: system_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.system_settings (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    key text NOT NULL,
    value jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    is_public boolean DEFAULT false NOT NULL
);


--
-- Name: tag_moderators; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tag_moderators (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tag_id uuid NOT NULL,
    user_id uuid NOT NULL,
    assigned_by text DEFAULT 'site_admin'::text NOT NULL,
    assigned_by_user_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: tags; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tags (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    slug text NOT NULL,
    description text,
    color text,
    follower_count integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    logo_media_id uuid,
    rules text,
    cover_media_id uuid,
    content_count integer DEFAULT 0 NOT NULL,
    is_system boolean DEFAULT false NOT NULL,
    field_values jsonb DEFAULT '{}'::jsonb NOT NULL
);


--
-- Name: tax_obligation_guides; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tax_obligation_guides (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    origin_space_id uuid NOT NULL,
    destination_space_id uuid NOT NULL,
    employment_status text NOT NULL,
    residency_trigger text,
    worldwide_income_taxed boolean,
    first_year_obligations text,
    action_checklist jsonb,
    status text DEFAULT 'published'::text NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    ai_generated boolean DEFAULT false NOT NULL,
    ai_model text,
    ai_prompt_hash text,
    hitl_reviewed_by uuid,
    hitl_reviewed_at timestamp with time zone,
    hitl_review_notes text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    intel_record_id uuid,
    guide_id uuid,
    confidence_overall real DEFAULT 0 NOT NULL,
    verification_status text DEFAULT 'verified'::text NOT NULL,
    freshness_score integer DEFAULT 100 NOT NULL,
    override_source text DEFAULT 'ingestion'::text NOT NULL,
    editorial_overrides jsonb DEFAULT '{}'::jsonb NOT NULL,
    CONSTRAINT tax_obligation_guides_override_source_check CHECK ((override_source = ANY (ARRAY['ingestion'::text, 'editorial'::text, 'mixed'::text]))),
    CONSTRAINT tax_obligation_guides_verification_status_check CHECK ((verification_status = ANY (ARRAY['verified'::text, 'needs_review'::text, 'conflict'::text, 'stale'::text])))
);


--
-- Name: tax_treaty_pairs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tax_treaty_pairs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    origin_space_id uuid NOT NULL,
    destination_space_id uuid NOT NULL,
    treaty_exists boolean NOT NULL,
    treaty_summary text,
    treaty_source_url text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: template_literals; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.template_literals (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    key text NOT NULL,
    kind text DEFAULT 'expression'::text NOT NULL,
    value text DEFAULT ''::text NOT NULL,
    fallback text,
    label text,
    description text,
    category text,
    is_system boolean DEFAULT false NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);


--
-- Name: ticket_attachments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ticket_attachments (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    ticket_id uuid NOT NULL,
    comment_id uuid,
    media_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: ticket_comments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ticket_comments (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    ticket_id uuid NOT NULL,
    user_id uuid NOT NULL,
    body text NOT NULL,
    is_internal boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: ticket_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ticket_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    ticket_id uuid NOT NULL,
    user_id uuid,
    event_type text NOT NULL,
    old_value text,
    new_value text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: ticket_types; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ticket_types (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    slug text NOT NULL,
    description text,
    field_group_id uuid,
    default_priority text DEFAULT 'normal'::text NOT NULL,
    auto_assign_group_id uuid,
    sla_hours integer,
    is_public boolean DEFAULT true NOT NULL,
    is_system boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: tickets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tickets (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    type_id uuid NOT NULL,
    subject text NOT NULL,
    description text NOT NULL,
    status text DEFAULT 'open'::text NOT NULL,
    priority text DEFAULT 'normal'::text NOT NULL,
    submitter_user_id uuid,
    submitter_email text,
    assigned_to uuid,
    assigned_group_id uuid,
    resolution_note text,
    field_values jsonb DEFAULT '{}'::jsonb NOT NULL,
    sla_breach_at timestamp with time zone,
    resolved_at timestamp with time zone,
    closed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: totp_credentials; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.totp_credentials (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    secret_encrypted text NOT NULL,
    enabled_at timestamp with time zone,
    backup_codes jsonb DEFAULT '[]'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: translations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.translations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    locale character varying(10) NOT NULL,
    key character varying(255) NOT NULL,
    value text NOT NULL,
    context character varying(50),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: travel_providers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.travel_providers (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    slug text NOT NULL,
    provider_type text NOT NULL,
    api_config jsonb,
    affiliate_config jsonb,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT travel_providers_provider_type_check CHECK ((provider_type = ANY (ARRAY['flight'::text, 'hotel'::text, 'both'::text])))
);


--
-- Name: trending_snapshots; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.trending_snapshots (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    entity_type text NOT NULL,
    entity_id uuid NOT NULL,
    score double precision DEFAULT 0 NOT NULL,
    "window" text DEFAULT '24h'::text NOT NULL,
    geo_code text,
    computed_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: university_course_catalog; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.university_course_catalog (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    title text NOT NULL,
    slug text NOT NULL,
    description text,
    degree_type text,
    department text,
    field_of_study text,
    language text DEFAULT 'English'::text NOT NULL,
    default_duration_years numeric,
    is_published boolean DEFAULT true NOT NULL,
    deleted_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: university_course_offerings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.university_course_offerings (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    course_id uuid NOT NULL,
    university_id uuid NOT NULL,
    slug text NOT NULL,
    tuition numeric,
    currency text,
    duration_years numeric,
    study_mode text,
    intake_months text[],
    entry_requirements text,
    accreditation text,
    is_published boolean DEFAULT true NOT NULL,
    deleted_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: university_track_categories; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.university_track_categories (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    slug text NOT NULL,
    name text NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: user_affiliate_links; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_affiliate_links (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    product_type text NOT NULL,
    product_id uuid,
    code text NOT NULL,
    description text,
    destination_url text NOT NULL,
    click_count integer DEFAULT 0 NOT NULL,
    conversion_count integer DEFAULT 0 NOT NULL,
    total_earned numeric(12,2) DEFAULT 0 NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT user_affiliate_links_product_type_check CHECK ((product_type = ANY (ARRAY['pro_subscription'::text, 'course'::text, 'general'::text])))
);


--
-- Name: user_badges; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_badges (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    badge_id uuid NOT NULL,
    awarded_at timestamp with time zone DEFAULT now() NOT NULL,
    awarded_by uuid
);


--
-- Name: user_blocks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_blocks (
    blocker_id uuid NOT NULL,
    blocked_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: user_challenge_progress; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_challenge_progress (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    challenge_id uuid NOT NULL,
    current_count integer DEFAULT 0 NOT NULL,
    completed boolean DEFAULT false NOT NULL,
    completed_at timestamp with time zone,
    reward_claimed boolean DEFAULT false NOT NULL
);


--
-- Name: user_content_views; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_content_views (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    content_id uuid NOT NULL,
    user_id uuid,
    session_id text,
    country_code text,
    viewed_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: user_dm_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_dm_settings (
    user_id uuid NOT NULL,
    dm_privacy text DEFAULT 'everyone'::text NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: user_group_memberships; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_group_memberships (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    group_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: user_group_promotions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_group_promotions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    source_group_id uuid NOT NULL,
    target_group_id uuid NOT NULL,
    trigger_type text NOT NULL,
    trigger_value jsonb,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: user_groups; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_groups (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    slug text NOT NULL,
    is_system boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    max_spaces_creatable integer DEFAULT 0 NOT NULL,
    description text,
    auto_withdrawal_limit integer DEFAULT 0,
    requires_withdrawal_approval boolean DEFAULT true NOT NULL,
    tier text,
    allow_search_indexing boolean DEFAULT true NOT NULL,
    CONSTRAINT user_groups_tier_check CHECK ((tier = ANY (ARRAY['regular'::text, 'pro'::text, 'business'::text])))
);


--
-- Name: user_moderation_actions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_moderation_actions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    action text NOT NULL,
    reason text,
    expires_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    violation_type text,
    CONSTRAINT user_moderation_actions_action_check CHECK ((action = ANY (ARRAY['ban'::text, 'unban'::text, 'warning'::text, 'suspension'::text])))
);


--
-- Name: user_mutes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_mutes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    muted_user_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: user_operation_bans; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_operation_bans (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    operation public.ban_operation NOT NULL,
    reason text,
    expires_at timestamp with time zone,
    issued_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);


--
-- Name: user_preferences; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_preferences (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    dark_mode text DEFAULT 'system'::text NOT NULL,
    language text DEFAULT 'en-NG'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: user_privacy_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_privacy_settings (
    user_id uuid NOT NULL,
    dm_privacy text DEFAULT 'everyone'::text NOT NULL,
    profile_visibility text DEFAULT 'public'::text NOT NULL,
    search_indexable boolean DEFAULT true NOT NULL,
    show_online_status boolean DEFAULT true NOT NULL,
    show_activity boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    show_points_publicly boolean DEFAULT false NOT NULL,
    CONSTRAINT user_privacy_settings_dm_privacy_check CHECK ((dm_privacy = ANY (ARRAY['everyone'::text, 'followers_only'::text, 'nobody'::text]))),
    CONSTRAINT user_privacy_settings_profile_visibility_check CHECK ((profile_visibility = ANY (ARRAY['public'::text, 'members'::text, 'followers'::text])))
);


--
-- Name: user_profiles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_profiles (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    display_name text,
    avatar_url text,
    bio text,
    timezone text,
    business_name text,
    registration_number text,
    operating_hours jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    content_country_preference text,
    cover_url text,
    preferred_currency text,
    avatar_media_id uuid,
    header_media_id uuid,
    japa_stage text,
    japa_stage_dest_space_id uuid,
    japa_stage_updated_at timestamp with time zone,
    japa_stage_is_public boolean DEFAULT true NOT NULL,
    japa_stage_history jsonb DEFAULT '[]'::jsonb NOT NULL,
    is_mentor boolean DEFAULT false,
    mentor_capacity smallint DEFAULT 3,
    active_mentee_count smallint DEFAULT 0,
    mentor_areas text[],
    mentor_bio text,
    mentor_since timestamp with time zone,
    mentor_embedding public.vector(1536),
    location text,
    banner_media_id uuid,
    japa_intent text,
    CONSTRAINT user_profiles_intent_stage_chk CHECK (((japa_stage IS NULL) OR (((japa_intent IS NULL) OR (japa_intent = ANY (ARRAY['relocation'::text, 'study'::text, 'work'::text, 'family'::text]))) AND (japa_stage = ANY (ARRAY['considering'::text, 'planning'::text, 'preparing'::text, 'applied'::text, 'approved'::text, 'relocated'::text, 'settled'::text]))) OR ((japa_intent = ANY (ARRAY['visit'::text, 'business'::text])) AND (japa_stage = ANY (ARRAY['considering'::text, 'planning'::text, 'preparing'::text, 'applied'::text, 'approved'::text, 'traveled'::text, 'returned'::text]))))),
    CONSTRAINT user_profiles_japa_intent_chk CHECK ((japa_intent = ANY (ARRAY['relocation'::text, 'study'::text, 'work'::text, 'family'::text, 'visit'::text, 'business'::text]))),
    CONSTRAINT user_profiles_japa_stage_chk CHECK ((japa_stage = ANY (ARRAY['considering'::text, 'planning'::text, 'preparing'::text, 'applied'::text, 'approved'::text, 'relocated'::text, 'settled'::text, 'traveled'::text, 'returned'::text])))
);


--
-- Name: COLUMN user_profiles.content_country_preference; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.user_profiles.content_country_preference IS 'User override for geo content weighting (ISO country code). NULL = use resolved VisitorContext (ADR-064).';


--
-- Name: user_roles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_roles (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    slug text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: user_source_blocks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_source_blocks (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    blocker_id uuid NOT NULL,
    source_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.users (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    email text NOT NULL,
    email_verified_at timestamp with time zone,
    password_hash text,
    username text NOT NULL,
    profile_type text DEFAULT 'regular'::text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    last_login_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    follower_count integer DEFAULT 0 NOT NULL,
    force_password_change boolean DEFAULT false NOT NULL,
    current_geo_space_id uuid,
    username_changed_at timestamp with time zone,
    is_bot boolean DEFAULT false NOT NULL,
    password_changed_at timestamp with time zone,
    ai_exempt boolean DEFAULT false NOT NULL,
    is_synthetic boolean DEFAULT false NOT NULL,
    synthetic_source text,
    source_id uuid,
    CONSTRAINT users_profile_type_check CHECK ((profile_type = ANY (ARRAY['regular'::text, 'pro'::text, 'business'::text])))
);


--
-- Name: video_channel_claims; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.video_channel_claims (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    claimant_user_id uuid NOT NULL,
    video_author_user_id uuid NOT NULL,
    claim_type text NOT NULL,
    evidence_url text,
    notes text,
    status text DEFAULT 'pending'::text NOT NULL,
    reviewed_by uuid,
    resolution_note text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    resolved_at timestamp with time zone,
    video_source_id uuid,
    CONSTRAINT video_channel_claims_claim_type_check CHECK ((claim_type = ANY (ARRAY['claim'::text, 'takedown'::text]))),
    CONSTRAINT video_channel_claims_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'approved'::text, 'rejected'::text, 'withdrawn'::text])))
);


--
-- Name: video_import_jobs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.video_import_jobs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    source_type text NOT NULL,
    source_url text,
    query text,
    target_author_user_id uuid,
    status text DEFAULT 'pending'::text NOT NULL,
    total_items integer DEFAULT 0 NOT NULL,
    imported_count integer DEFAULT 0 NOT NULL,
    failed_count integer DEFAULT 0 NOT NULL,
    error_log jsonb DEFAULT '[]'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    completed_at timestamp with time zone,
    auto_publish boolean DEFAULT false NOT NULL,
    started_at timestamp with time zone,
    CONSTRAINT video_import_jobs_source_type_check CHECK ((source_type = ANY (ARRAY['youtube_channel'::text, 'youtube_playlist'::text, 'youtube_search'::text]))),
    CONSTRAINT video_import_jobs_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'processing'::text, 'completed'::text, 'failed'::text, 'partial'::text])))
);


--
-- Name: video_sources; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.video_sources (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    author_user_id uuid NOT NULL,
    source_type text NOT NULL,
    source_url text NOT NULL,
    external_id text,
    title text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    category text,
    thumbnail_url text,
    thumbnail_media_id uuid,
    duration_secs integer,
    status text DEFAULT 'active'::text NOT NULL,
    field_values jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    source_id uuid,
    slug text NOT NULL,
    CONSTRAINT video_sources_source_type_check CHECK ((source_type = ANY (ARRAY['upload'::text, 'youtube'::text, 'tiktok'::text, 'instagram_reel'::text, 'vimeo'::text, 'other'::text]))),
    CONSTRAINT video_sources_status_check CHECK ((status = ANY (ARRAY['active'::text, 'takedown_requested'::text, 'removed'::text])))
);


--
-- Name: visa_processing_stats; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.visa_processing_stats (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    origin_space_id uuid,
    destination_space_id uuid NOT NULL,
    visa_category text NOT NULL,
    consulate text,
    period_start date NOT NULL,
    sample_count integer DEFAULT 0 NOT NULL,
    avg_days numeric,
    median_days numeric,
    p10_days numeric,
    p90_days numeric,
    refreshed_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: visa_routes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.visa_routes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    destination_space_id uuid NOT NULL,
    visa_category text NOT NULL,
    name text NOT NULL,
    slug text NOT NULL,
    eligibility_summary text,
    requirements_json jsonb,
    typical_fee_usd numeric(8,2),
    path_to_pr text,
    renewal_terms text,
    restrictions text,
    gov_source_url text NOT NULL,
    source_url_last_checked date,
    status text DEFAULT 'published'::text NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    ai_generated boolean DEFAULT false NOT NULL,
    ai_model text,
    ai_prompt_hash text,
    hitl_reviewed_by uuid,
    hitl_reviewed_at timestamp with time zone,
    hitl_review_notes text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    is_japabot_indexed boolean GENERATED ALWAYS AS (((status = 'published'::text) AND (is_active = true))) STORED,
    intel_record_id uuid,
    confidence_overall real DEFAULT 0 NOT NULL,
    verification_status text DEFAULT 'verified'::text NOT NULL,
    freshness_score integer DEFAULT 100 NOT NULL,
    override_source text DEFAULT 'ingestion'::text NOT NULL,
    editorial_overrides jsonb DEFAULT '{}'::jsonb NOT NULL,
    guide_id uuid,
    CONSTRAINT visa_routes_override_source_check CHECK ((override_source = ANY (ARRAY['ingestion'::text, 'editorial'::text, 'mixed'::text]))),
    CONSTRAINT visa_routes_verification_status_check CHECK ((verification_status = ANY (ARRAY['verified'::text, 'needs_review'::text, 'conflict'::text, 'stale'::text])))
);


--
-- Name: visa_timeline_milestones; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.visa_timeline_milestones (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    timeline_id uuid NOT NULL,
    milestone_type text NOT NULL,
    occurred_at date NOT NULL,
    notes text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: visa_timelines; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.visa_timelines (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid,
    origin_space_id uuid,
    destination_space_id uuid NOT NULL,
    visa_category text NOT NULL,
    consulate text,
    application_method text,
    outcome text,
    is_shared boolean DEFAULT false NOT NULL,
    anonymised_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);


--
-- Name: wallet_ledger; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.wallet_ledger (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    ledger_type text NOT NULL,
    operation text NOT NULL,
    amount bigint NOT NULL,
    resulting_balance bigint NOT NULL,
    source text,
    actor_user_id uuid,
    reference text,
    idempotency_key text,
    reason text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT wallet_ledger_ledger_type_check CHECK ((ledger_type = ANY (ARRAY['points'::text, 'credits'::text]))),
    CONSTRAINT wallet_ledger_operation_check CHECK ((operation = ANY (ARRAY['earn'::text, 'spend'::text, 'purchase'::text, 'convert'::text, 'refund'::text, 'clawback'::text, 'admin_adjust'::text, 'withdraw'::text])))
);


--
-- Name: wallet_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.wallet_settings (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    points_to_credits_rate integer DEFAULT 100,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    daily_earning_cap integer DEFAULT 1000 NOT NULL,
    enable_earning boolean DEFAULT true NOT NULL
);


--
-- Name: wallet_withdrawal_fee_tiers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.wallet_withdrawal_fee_tiers (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    min_amount integer DEFAULT 0 NOT NULL,
    max_amount integer,
    fee_type text NOT NULL,
    fee_value integer NOT NULL,
    tier_mode text DEFAULT 'credit'::text NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    "position" integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT wallet_withdrawal_fee_tiers_fee_type_check CHECK ((fee_type = ANY (ARRAY['percentage'::text, 'fixed'::text]))),
    CONSTRAINT wallet_withdrawal_fee_tiers_tier_mode_check CHECK ((tier_mode = ANY (ARRAY['credit'::text, 'fiat'::text])))
);


--
-- Name: wallet_withdrawal_methods; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.wallet_withdrawal_methods (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    provider text NOT NULL,
    label text,
    account text NOT NULL,
    currency_code text,
    is_verified boolean DEFAULT false NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: wallet_withdrawals; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.wallet_withdrawals (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    credit_amount bigint NOT NULL,
    currency_code text NOT NULL,
    credits_per_unit integer NOT NULL,
    fiat_amount bigint NOT NULL,
    fee_amount bigint DEFAULT 0 NOT NULL,
    fee_basis text DEFAULT 'percentage'::text NOT NULL,
    fee_rate numeric(10,6) DEFAULT 0 NOT NULL,
    net_amount bigint NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    payout_method_id uuid,
    payout_provider text NOT NULL,
    provider_ref text,
    admin_note text,
    reviewed_by uuid,
    reviewed_at timestamp with time zone,
    idempotency_key text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT wallet_withdrawals_credit_amount_check CHECK ((credit_amount > 0)),
    CONSTRAINT wallet_withdrawals_fee_basis_check CHECK ((fee_basis = ANY (ARRAY['percentage'::text, 'fixed'::text]))),
    CONSTRAINT wallet_withdrawals_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'pending_approval'::text, 'approved'::text, 'processing'::text, 'completed'::text, 'failed'::text, 'rejected'::text, 'cancelled'::text])))
);


--
-- Name: webhook_deliveries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.webhook_deliveries (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    endpoint_id uuid NOT NULL,
    event_id uuid NOT NULL,
    event_type text NOT NULL,
    payload jsonb NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    last_error text,
    delivered_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    next_attempt_at timestamp with time zone,
    last_response_code integer,
    last_response_body text,
    CONSTRAINT webhook_deliveries_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'delivered'::text, 'failed'::text, 'retrying'::text])))
);


--
-- Name: webhook_endpoints; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.webhook_endpoints (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    url text NOT NULL,
    secret text NOT NULL,
    event_types text[] DEFAULT '{}'::text[] NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    name text NOT NULL,
    is_active boolean DEFAULT false NOT NULL,
    verified_at timestamp with time zone,
    created_by uuid,
    scope text DEFAULT 'platform'::text NOT NULL,
    CONSTRAINT webhook_endpoints_scope_check CHECK ((scope = ANY (ARRAY['platform'::text, 'user'::text])))
);


--
-- Name: widget_configurations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.widget_configurations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    widget_type text NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    cache_ttl_secs integer DEFAULT 300 NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: wiki_pages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.wiki_pages (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    space_id uuid NOT NULL,
    slug text NOT NULL,
    title text NOT NULL,
    category text DEFAULT 'other'::text NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    is_featured boolean DEFAULT false NOT NULL,
    is_locked boolean DEFAULT false NOT NULL,
    created_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    is_japabot_indexed boolean GENERATED ALWAYS AS (((is_active = true) AND (is_locked = false))) STORED,
    intel_record_id uuid,
    guide_id uuid,
    confidence_overall real DEFAULT 0 NOT NULL,
    verification_status text DEFAULT 'verified'::text NOT NULL,
    freshness_score integer DEFAULT 100 NOT NULL,
    override_source text DEFAULT 'ingestion'::text NOT NULL,
    editorial_overrides jsonb DEFAULT '{}'::jsonb NOT NULL,
    data_entity_type text,
    CONSTRAINT wiki_pages_override_source_check CHECK ((override_source = ANY (ARRAY['ingestion'::text, 'editorial'::text, 'mixed'::text]))),
    CONSTRAINT wiki_pages_verification_status_check CHECK ((verification_status = ANY (ARRAY['verified'::text, 'needs_review'::text, 'conflict'::text, 'stale'::text])))
);


--
-- Name: wiki_revisions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.wiki_revisions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    wiki_page_id uuid,
    content jsonb DEFAULT '{}'::jsonb NOT NULL,
    summary text,
    status text DEFAULT 'pending'::text NOT NULL,
    submitted_by uuid,
    reviewed_by uuid,
    reviewed_at timestamp with time zone,
    ai_generated boolean DEFAULT false NOT NULL,
    ai_model text,
    ai_prompt_hash text,
    hitl_reviewed_by uuid,
    hitl_reviewed_at timestamp with time zone,
    hitl_review_notes text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    content_hash text,
    kind text DEFAULT 'content'::text NOT NULL,
    ops jsonb DEFAULT '[]'::jsonb NOT NULL,
    target_entity_type text,
    target_entity_id uuid,
    apply_error text,
    CONSTRAINT wiki_revisions_kind_check CHECK ((kind = ANY (ARRAY['content'::text, 'fields'::text, 'mixed'::text])))
);


--
-- Name: ws_tickets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ws_tickets (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    used boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone NOT NULL
);


--
-- Name: account_deletion_requests account_deletion_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_deletion_requests
    ADD CONSTRAINT account_deletion_requests_pkey PRIMARY KEY (id);


--
-- Name: achievement_meta achievement_meta_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.achievement_meta
    ADD CONSTRAINT achievement_meta_pkey PRIMARY KEY (id);


--
-- Name: activity_events activity_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.activity_events
    ADD CONSTRAINT activity_events_pkey PRIMARY KEY (id);


--
-- Name: activity_point_rules activity_point_rules_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.activity_point_rules
    ADD CONSTRAINT activity_point_rules_pkey PRIMARY KEY (id);


--
-- Name: ad_analytics_daily ad_analytics_daily_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ad_analytics_daily
    ADD CONSTRAINT ad_analytics_daily_pkey PRIMARY KEY (id);


--
-- Name: ad_campaigns ad_campaigns_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ad_campaigns
    ADD CONSTRAINT ad_campaigns_pkey PRIMARY KEY (id);


--
-- Name: ad_events ad_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ad_events
    ADD CONSTRAINT ad_events_pkey PRIMARY KEY (id);


--
-- Name: ad_networks ad_networks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ad_networks
    ADD CONSTRAINT ad_networks_pkey PRIMARY KEY (id);


--
-- Name: ad_slot_types ad_slot_types_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ad_slot_types
    ADD CONSTRAINT ad_slot_types_pkey PRIMARY KEY (id);


--
-- Name: admin_broadcasts admin_broadcasts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_broadcasts
    ADD CONSTRAINT admin_broadcasts_pkey PRIMARY KEY (id);


--
-- Name: ads_campaigns ads_campaigns_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ads_campaigns
    ADD CONSTRAINT ads_campaigns_pkey PRIMARY KEY (id);


--
-- Name: affiliate_content_commissions affiliate_content_commissions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.affiliate_content_commissions
    ADD CONSTRAINT affiliate_content_commissions_pkey PRIMARY KEY (id);


--
-- Name: affiliate_link_events affiliate_link_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.affiliate_link_events
    ADD CONSTRAINT affiliate_link_events_pkey PRIMARY KEY (id);


--
-- Name: affiliate_program_members affiliate_program_members_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.affiliate_program_members
    ADD CONSTRAINT affiliate_program_members_pkey PRIMARY KEY (user_id);


--
-- Name: affiliate_providers affiliate_providers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.affiliate_providers
    ADD CONSTRAINT affiliate_providers_pkey PRIMARY KEY (id);


--
-- Name: ai_agent_sessions ai_agent_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_agent_sessions
    ADD CONSTRAINT ai_agent_sessions_pkey PRIMARY KEY (id);


--
-- Name: ai_agent_tool_calls ai_agent_tool_calls_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_agent_tool_calls
    ADD CONSTRAINT ai_agent_tool_calls_pkey PRIMARY KEY (id);


--
-- Name: ai_hitl_reviews ai_hitl_reviews_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_hitl_reviews
    ADD CONSTRAINT ai_hitl_reviews_pkey PRIMARY KEY (id);


--
-- Name: ai_personas ai_personas_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_personas
    ADD CONSTRAINT ai_personas_pkey PRIMARY KEY (id);


--
-- Name: ai_personas ai_personas_user_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_personas
    ADD CONSTRAINT ai_personas_user_id_key UNIQUE (user_id);


--
-- Name: ai_providers ai_providers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_providers
    ADD CONSTRAINT ai_providers_pkey PRIMARY KEY (id);


--
-- Name: ai_tool_usage ai_tool_usage_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_tool_usage
    ADD CONSTRAINT ai_tool_usage_pkey PRIMARY KEY (id);


--
-- Name: ai_tools ai_tools_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_tools
    ADD CONSTRAINT ai_tools_pkey PRIMARY KEY (id);


--
-- Name: ai_usage_log ai_usage_log_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_usage_log
    ADD CONSTRAINT ai_usage_log_pkey PRIMARY KEY (id);


--
-- Name: api_key_ip_allowlist api_key_ip_allowlist_api_key_id_cidr_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_key_ip_allowlist
    ADD CONSTRAINT api_key_ip_allowlist_api_key_id_cidr_key UNIQUE (api_key_id, cidr);


--
-- Name: api_key_ip_allowlist api_key_ip_allowlist_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_key_ip_allowlist
    ADD CONSTRAINT api_key_ip_allowlist_pkey PRIMARY KEY (id);


--
-- Name: api_keys api_keys_key_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_key_hash_key UNIQUE (key_hash);


--
-- Name: api_keys api_keys_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_pkey PRIMARY KEY (id);


--
-- Name: appointment_alerts appointment_alerts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.appointment_alerts
    ADD CONSTRAINT appointment_alerts_pkey PRIMARY KEY (id);


--
-- Name: arrival_checklist_guides arrival_checklist_guides_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arrival_checklist_guides
    ADD CONSTRAINT arrival_checklist_guides_pkey PRIMARY KEY (id);


--
-- Name: arrival_checklist_items arrival_checklist_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arrival_checklist_items
    ADD CONSTRAINT arrival_checklist_items_pkey PRIMARY KEY (id);


--
-- Name: assessment_results assessment_results_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.assessment_results
    ADD CONSTRAINT assessment_results_pkey PRIMARY KEY (id);


--
-- Name: attestation_community_updates attestation_community_updates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.attestation_community_updates
    ADD CONSTRAINT attestation_community_updates_pkey PRIMARY KEY (id);


--
-- Name: attestation_pathways attestation_pathways_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.attestation_pathways
    ADD CONSTRAINT attestation_pathways_pkey PRIMARY KEY (id);


--
-- Name: audit_logs audit_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_logs
    ADD CONSTRAINT audit_logs_pkey PRIMARY KEY (id);


--
-- Name: badge_definitions badge_definitions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.badge_definitions
    ADD CONSTRAINT badge_definitions_pkey PRIMARY KEY (id);


--
-- Name: blog_series blog_series_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_series
    ADD CONSTRAINT blog_series_pkey PRIMARY KEY (id);


--
-- Name: blog_series_posts blog_series_posts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_series_posts
    ADD CONSTRAINT blog_series_posts_pkey PRIMARY KEY (series_id, content_id);


--
-- Name: booking_slots booking_slots_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.booking_slots
    ADD CONSTRAINT booking_slots_pkey PRIMARY KEY (id);


--
-- Name: bookings bookings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bookings
    ADD CONSTRAINT bookings_pkey PRIMARY KEY (id);


--
-- Name: bookmark_folders bookmark_folders_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bookmark_folders
    ADD CONSTRAINT bookmark_folders_pkey PRIMARY KEY (id);


--
-- Name: bookmark_folders bookmark_folders_user_id_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bookmark_folders
    ADD CONSTRAINT bookmark_folders_user_id_name_key UNIQUE (user_id, name);


--
-- Name: bookmarks bookmarks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bookmarks
    ADD CONSTRAINT bookmarks_pkey PRIMARY KEY (id);


--
-- Name: bookmarks bookmarks_user_id_entity_type_entity_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bookmarks
    ADD CONSTRAINT bookmarks_user_id_entity_type_entity_id_key UNIQUE (user_id, entity_type, entity_id);


--
-- Name: calendar_entries calendar_entries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.calendar_entries
    ADD CONSTRAINT calendar_entries_pkey PRIMARY KEY (id);


--
-- Name: calendar_saves calendar_saves_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.calendar_saves
    ADD CONSTRAINT calendar_saves_pkey PRIMARY KEY (user_id, entry_id);


--
-- Name: censored_words censored_words_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.censored_words
    ADD CONSTRAINT censored_words_pkey PRIMARY KEY (id);


--
-- Name: challenge_definitions challenge_definitions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.challenge_definitions
    ADD CONSTRAINT challenge_definitions_pkey PRIMARY KEY (id);


--
-- Name: checklist_progress checklist_progress_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checklist_progress
    ADD CONSTRAINT checklist_progress_pkey PRIMARY KEY (id);


--
-- Name: comment_views comment_views_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comment_views
    ADD CONSTRAINT comment_views_pkey PRIMARY KEY (comment_id, user_id);


--
-- Name: comment_votes comment_votes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comment_votes
    ADD CONSTRAINT comment_votes_pkey PRIMARY KEY (comment_id, user_id);


--
-- Name: comments comments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comments
    ADD CONSTRAINT comments_pkey PRIMARY KEY (id);


--
-- Name: community_organisations community_organisations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.community_organisations
    ADD CONSTRAINT community_organisations_pkey PRIMARY KEY (id);


--
-- Name: comparison_content_metadata comparison_content_metadata_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_content_metadata
    ADD CONSTRAINT comparison_content_metadata_pkey PRIMARY KEY (content_id);


--
-- Name: comparison_data_ingestion_jobs comparison_data_ingestion_jobs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_data_ingestion_jobs
    ADD CONSTRAINT comparison_data_ingestion_jobs_pkey PRIMARY KEY (id);


--
-- Name: comparison_data_sources comparison_data_sources_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_data_sources
    ADD CONSTRAINT comparison_data_sources_pkey PRIMARY KEY (id);


--
-- Name: comparison_dimensions comparison_dimensions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_dimensions
    ADD CONSTRAINT comparison_dimensions_pkey PRIMARY KEY (id);


--
-- Name: comparison_items comparison_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_items
    ADD CONSTRAINT comparison_items_pkey PRIMARY KEY (id);


--
-- Name: comparison_profiles comparison_profiles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_profiles
    ADD CONSTRAINT comparison_profiles_pkey PRIMARY KEY (id);


--
-- Name: comparison_recommendations comparison_recommendations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_recommendations
    ADD CONSTRAINT comparison_recommendations_pkey PRIMARY KEY (id);


--
-- Name: comparison_score_factors comparison_score_factors_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_score_factors
    ADD CONSTRAINT comparison_score_factors_pkey PRIMARY KEY (id);


--
-- Name: comparison_scores comparison_scores_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_scores
    ADD CONSTRAINT comparison_scores_pkey PRIMARY KEY (id);


--
-- Name: comparison_type_access comparison_type_access_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_type_access
    ADD CONSTRAINT comparison_type_access_pkey PRIMARY KEY (type_id, group_key);


--
-- Name: comparison_type_definitions comparison_type_definitions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_type_definitions
    ADD CONSTRAINT comparison_type_definitions_pkey PRIMARY KEY (id);


--
-- Name: comparison_values comparison_values_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_values
    ADD CONSTRAINT comparison_values_pkey PRIMARY KEY (dimension_id, item_id);


--
-- Name: consent_records consent_records_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.consent_records
    ADD CONSTRAINT consent_records_pkey PRIMARY KEY (id);


--
-- Name: content_affiliate_scan_results content_affiliate_scan_results_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_affiliate_scan_results
    ADD CONSTRAINT content_affiliate_scan_results_pkey PRIMARY KEY (id);


--
-- Name: content_campaign_items content_campaign_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_campaign_items
    ADD CONSTRAINT content_campaign_items_pkey PRIMARY KEY (id);


--
-- Name: content_campaigns content_campaigns_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_campaigns
    ADD CONSTRAINT content_campaigns_pkey PRIMARY KEY (id);


--
-- Name: content_embeddings content_embeddings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_embeddings
    ADD CONSTRAINT content_embeddings_pkey PRIMARY KEY (id);


--
-- Name: content_engagement content_engagement_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_engagement
    ADD CONSTRAINT content_engagement_pkey PRIMARY KEY (content_id);


--
-- Name: content_featured content_featured_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_featured
    ADD CONSTRAINT content_featured_pkey PRIMARY KEY (id);


--
-- Name: content_imports content_imports_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_imports
    ADD CONSTRAINT content_imports_pkey PRIMARY KEY (id);


--
-- Name: content_reports content_reports_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_reports
    ADD CONSTRAINT content_reports_pkey PRIMARY KEY (id);


--
-- Name: content_reviews content_reviews_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_reviews
    ADD CONSTRAINT content_reviews_pkey PRIMARY KEY (id);


--
-- Name: content_revisions content_revisions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_revisions
    ADD CONSTRAINT content_revisions_pkey PRIMARY KEY (id);


--
-- Name: content_scores content_scores_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_scores
    ADD CONSTRAINT content_scores_pkey PRIMARY KEY (content_id);


--
-- Name: content_shares content_shares_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_shares
    ADD CONSTRAINT content_shares_pkey PRIMARY KEY (id);


--
-- Name: content_spaces content_spaces_content_id_space_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_spaces
    ADD CONSTRAINT content_spaces_content_id_space_id_key UNIQUE (content_id, space_id);


--
-- Name: content_spaces content_spaces_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_spaces
    ADD CONSTRAINT content_spaces_pkey PRIMARY KEY (id);


--
-- Name: content_tags content_tags_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_tags
    ADD CONSTRAINT content_tags_pkey PRIMARY KEY (content_id, tag_id);


--
-- Name: content_thread_links content_thread_links_content_id_thread_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_thread_links
    ADD CONSTRAINT content_thread_links_content_id_thread_id_key UNIQUE (content_id, thread_id);


--
-- Name: content_thread_links content_thread_links_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_thread_links
    ADD CONSTRAINT content_thread_links_pkey PRIMARY KEY (id);


--
-- Name: content_type_jsonld_templates content_type_jsonld_templates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_type_jsonld_templates
    ADD CONSTRAINT content_type_jsonld_templates_pkey PRIMARY KEY (id);


--
-- Name: content_views content_views_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_views
    ADD CONSTRAINT content_views_pkey PRIMARY KEY (id);


--
-- Name: contents contents_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contents
    ADD CONSTRAINT contents_pkey PRIMARY KEY (id);


--
-- Name: coupon_events coupon_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.coupon_events
    ADD CONSTRAINT coupon_events_pkey PRIMARY KEY (id);


--
-- Name: coupons coupons_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.coupons
    ADD CONSTRAINT coupons_pkey PRIMARY KEY (id);


--
-- Name: course_access_grants course_access_grants_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.course_access_grants
    ADD CONSTRAINT course_access_grants_pkey PRIMARY KEY (id);


--
-- Name: course_access_grants course_access_grants_user_id_content_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.course_access_grants
    ADD CONSTRAINT course_access_grants_user_id_content_id_key UNIQUE (user_id, content_id);


--
-- Name: credential_community_notes credential_community_notes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.credential_community_notes
    ADD CONSTRAINT credential_community_notes_pkey PRIMARY KEY (id);


--
-- Name: credential_pathways credential_pathways_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.credential_pathways
    ADD CONSTRAINT credential_pathways_pkey PRIMARY KEY (id);


--
-- Name: credential_professions credential_professions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.credential_professions
    ADD CONSTRAINT credential_professions_pkey PRIMARY KEY (id);


--
-- Name: credential_professions credential_professions_slug_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.credential_professions
    ADD CONSTRAINT credential_professions_slug_key UNIQUE (slug);


--
-- Name: credit_packs credit_packs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.credit_packs
    ADD CONSTRAINT credit_packs_pkey PRIMARY KEY (id);


--
-- Name: crs_draws crs_draws_draw_date_category_draw_number_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.crs_draws
    ADD CONSTRAINT crs_draws_draw_date_category_draw_number_key UNIQUE (draw_date, category, draw_number);


--
-- Name: crs_draws crs_draws_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.crs_draws
    ADD CONSTRAINT crs_draws_pkey PRIMARY KEY (id);


--
-- Name: crs_factor_points crs_factor_points_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.crs_factor_points
    ADD CONSTRAINT crs_factor_points_pkey PRIMARY KEY (id);


--
-- Name: crs_factor_points crs_factor_points_ruleset_id_factor_group_factor_key_input__key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.crs_factor_points
    ADD CONSTRAINT crs_factor_points_ruleset_id_factor_group_factor_key_input__key UNIQUE (ruleset_id, factor_group, factor_key, input_descriptor);


--
-- Name: crs_rulesets crs_rulesets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.crs_rulesets
    ADD CONSTRAINT crs_rulesets_pkey PRIMARY KEY (id);


--
-- Name: crs_rulesets crs_rulesets_version_label_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.crs_rulesets
    ADD CONSTRAINT crs_rulesets_version_label_key UNIQUE (version_label);


--
-- Name: crs_score_history crs_score_history_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.crs_score_history
    ADD CONSTRAINT crs_score_history_pkey PRIMARY KEY (id);


--
-- Name: currencies currencies_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.currencies
    ADD CONSTRAINT currencies_pkey PRIMARY KEY (code);


--
-- Name: custom_field_entity_types custom_field_entity_types_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_field_entity_types
    ADD CONSTRAINT custom_field_entity_types_pkey PRIMARY KEY (id);


--
-- Name: custom_field_group_assignments custom_field_group_assignment_group_id_entity_class_entity__key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_field_group_assignments
    ADD CONSTRAINT custom_field_group_assignment_group_id_entity_class_entity__key UNIQUE (group_id, entity_class, entity_id);


--
-- Name: custom_field_group_assignments custom_field_group_assignments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_field_group_assignments
    ADD CONSTRAINT custom_field_group_assignments_pkey PRIMARY KEY (id);


--
-- Name: custom_field_groups custom_field_groups_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_field_groups
    ADD CONSTRAINT custom_field_groups_pkey PRIMARY KEY (id);


--
-- Name: custom_field_permissions custom_field_permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_field_permissions
    ADD CONSTRAINT custom_field_permissions_pkey PRIMARY KEY (field_id, user_group_id);


--
-- Name: custom_field_templates custom_field_templates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_field_templates
    ADD CONSTRAINT custom_field_templates_pkey PRIMARY KEY (id);


--
-- Name: custom_field_values custom_field_values_field_id_entity_type_entity_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_field_values
    ADD CONSTRAINT custom_field_values_field_id_entity_type_entity_id_key UNIQUE (field_id, entity_type, entity_id);


--
-- Name: custom_field_values custom_field_values_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_field_values
    ADD CONSTRAINT custom_field_values_pkey PRIMARY KEY (id);


--
-- Name: custom_fields custom_fields_group_id_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_fields
    ADD CONSTRAINT custom_fields_group_id_key_key UNIQUE (group_id, key);


--
-- Name: custom_fields custom_fields_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_fields
    ADD CONSTRAINT custom_fields_pkey PRIMARY KEY (id);


--
-- Name: cv_format_sessions cv_format_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cv_format_sessions
    ADD CONSTRAINT cv_format_sessions_pkey PRIMARY KEY (id);


--
-- Name: cv_standards cv_standards_destination_space_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cv_standards
    ADD CONSTRAINT cv_standards_destination_space_id_key UNIQUE (destination_space_id);


--
-- Name: cv_standards cv_standards_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cv_standards
    ADD CONSTRAINT cv_standards_pkey PRIMARY KEY (id);


--
-- Name: data_export_requests data_export_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.data_export_requests
    ADD CONSTRAINT data_export_requests_pkey PRIMARY KEY (id);


--
-- Name: device_tokens device_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_tokens
    ADD CONSTRAINT device_tokens_pkey PRIMARY KEY (id);


--
-- Name: digest_editions digest_editions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.digest_editions
    ADD CONSTRAINT digest_editions_pkey PRIMARY KEY (id);


--
-- Name: discount_code_uses discount_code_uses_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.discount_code_uses
    ADD CONSTRAINT discount_code_uses_pkey PRIMARY KEY (code_id, user_id);


--
-- Name: discount_codes discount_codes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.discount_codes
    ADD CONSTRAINT discount_codes_pkey PRIMARY KEY (id);


--
-- Name: dm_conversations dm_conversations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dm_conversations
    ADD CONSTRAINT dm_conversations_pkey PRIMARY KEY (id);


--
-- Name: dm_messages dm_messages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dm_messages
    ADD CONSTRAINT dm_messages_pkey PRIMARY KEY (id);


--
-- Name: dm_requests dm_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dm_requests
    ADD CONSTRAINT dm_requests_pkey PRIMARY KEY (id);


--
-- Name: dm_requests dm_requests_requester_id_recipient_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dm_requests
    ADD CONSTRAINT dm_requests_requester_id_recipient_id_key UNIQUE (requester_id, recipient_id);


--
-- Name: doc_pages doc_pages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.doc_pages
    ADD CONSTRAINT doc_pages_pkey PRIMARY KEY (id);


--
-- Name: eca_community_updates eca_community_updates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.eca_community_updates
    ADD CONSTRAINT eca_community_updates_pkey PRIMARY KEY (id);


--
-- Name: eca_guides eca_guides_destination_space_id_provider_id_purpose_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.eca_guides
    ADD CONSTRAINT eca_guides_destination_space_id_provider_id_purpose_key UNIQUE (destination_space_id, provider_id, purpose);


--
-- Name: eca_guides eca_guides_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.eca_guides
    ADD CONSTRAINT eca_guides_pkey PRIMARY KEY (id);


--
-- Name: eca_providers eca_providers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.eca_providers
    ADD CONSTRAINT eca_providers_pkey PRIMARY KEY (id);


--
-- Name: eca_providers eca_providers_slug_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.eca_providers
    ADD CONSTRAINT eca_providers_slug_key UNIQUE (slug);


--
-- Name: editor_block_permissions editor_block_permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.editor_block_permissions
    ADD CONSTRAINT editor_block_permissions_pkey PRIMARY KEY (id);


--
-- Name: email_templates email_templates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email_templates
    ADD CONSTRAINT email_templates_pkey PRIMARY KEY (id);


--
-- Name: email_verifications email_verifications_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email_verifications
    ADD CONSTRAINT email_verifications_pkey PRIMARY KEY (id);


--
-- Name: enrollments enrollments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.enrollments
    ADD CONSTRAINT enrollments_pkey PRIMARY KEY (id);


--
-- Name: enrollments enrollments_user_id_track_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.enrollments
    ADD CONSTRAINT enrollments_user_id_track_id_key UNIQUE (user_id, track_id);


--
-- Name: external_data_sources external_data_sources_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.external_data_sources
    ADD CONSTRAINT external_data_sources_name_key UNIQUE (name);


--
-- Name: external_data_sources external_data_sources_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.external_data_sources
    ADD CONSTRAINT external_data_sources_pkey PRIMARY KEY (id);


--
-- Name: feature_flags feature_flags_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.feature_flags
    ADD CONSTRAINT feature_flags_pkey PRIMARY KEY (key);


--
-- Name: feeds feeds_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.feeds
    ADD CONSTRAINT feeds_pkey PRIMARY KEY (id);


--
-- Name: feeds feeds_user_id_content_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.feeds
    ADD CONSTRAINT feeds_user_id_content_id_key UNIQUE (user_id, content_id);


--
-- Name: follows follows_follower_id_entity_type_entity_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.follows
    ADD CONSTRAINT follows_follower_id_entity_type_entity_id_key UNIQUE (follower_id, entity_type, entity_id);


--
-- Name: follows follows_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.follows
    ADD CONSTRAINT follows_pkey PRIMARY KEY (id);


--
-- Name: form_steps form_steps_form_id_position_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.form_steps
    ADD CONSTRAINT form_steps_form_id_position_key UNIQUE (form_id, "position");


--
-- Name: form_steps form_steps_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.form_steps
    ADD CONSTRAINT form_steps_pkey PRIMARY KEY (id);


--
-- Name: form_submission_triggers form_submission_triggers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.form_submission_triggers
    ADD CONSTRAINT form_submission_triggers_pkey PRIMARY KEY (id);


--
-- Name: form_submissions form_submissions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.form_submissions
    ADD CONSTRAINT form_submissions_pkey PRIMARY KEY (id);


--
-- Name: forms forms_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forms
    ADD CONSTRAINT forms_pkey PRIMARY KEY (id);


--
-- Name: forum_bookmarks forum_bookmarks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_bookmarks
    ADD CONSTRAINT forum_bookmarks_pkey PRIMARY KEY (id);


--
-- Name: forum_bookmarks forum_bookmarks_user_id_entity_type_entity_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_bookmarks
    ADD CONSTRAINT forum_bookmarks_user_id_entity_type_entity_id_key UNIQUE (user_id, entity_type, entity_id);


--
-- Name: forum_poll_options forum_poll_options_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_poll_options
    ADD CONSTRAINT forum_poll_options_pkey PRIMARY KEY (id);


--
-- Name: forum_poll_votes forum_poll_votes_option_id_user_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_poll_votes
    ADD CONSTRAINT forum_poll_votes_option_id_user_id_key UNIQUE (option_id, user_id);


--
-- Name: forum_poll_votes forum_poll_votes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_poll_votes
    ADD CONSTRAINT forum_poll_votes_pkey PRIMARY KEY (id);


--
-- Name: forum_polls forum_polls_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_polls
    ADD CONSTRAINT forum_polls_pkey PRIMARY KEY (id);


--
-- Name: forum_replies forum_replies_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_replies
    ADD CONSTRAINT forum_replies_pkey PRIMARY KEY (id);


--
-- Name: forum_tags forum_tags_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_tags
    ADD CONSTRAINT forum_tags_pkey PRIMARY KEY (id);


--
-- Name: forum_thread_tags forum_thread_tags_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_thread_tags
    ADD CONSTRAINT forum_thread_tags_pkey PRIMARY KEY (thread_id, tag_id);


--
-- Name: forum_threads forum_threads_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_threads
    ADD CONSTRAINT forum_threads_pkey PRIMARY KEY (id);


--
-- Name: forum_votes forum_votes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_votes
    ADD CONSTRAINT forum_votes_pkey PRIMARY KEY (id);


--
-- Name: forum_votes forum_votes_user_id_entity_type_entity_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_votes
    ADD CONSTRAINT forum_votes_user_id_entity_type_entity_id_key UNIQUE (user_id, entity_type, entity_id);


--
-- Name: group_roles group_roles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.group_roles
    ADD CONSTRAINT group_roles_pkey PRIMARY KEY (group_id, role_id);


--
-- Name: housing_guides housing_guides_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.housing_guides
    ADD CONSTRAINT housing_guides_pkey PRIMARY KEY (id);


--
-- Name: instructor_payouts instructor_payouts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.instructor_payouts
    ADD CONSTRAINT instructor_payouts_pkey PRIMARY KEY (id);


--
-- Name: instructor_profiles instructor_profiles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.instructor_profiles
    ADD CONSTRAINT instructor_profiles_pkey PRIMARY KEY (id);


--
-- Name: intel_inbound_events intel_inbound_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.intel_inbound_events
    ADD CONSTRAINT intel_inbound_events_pkey PRIMARY KEY (id);


--
-- Name: intel_jobs intel_jobs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.intel_jobs
    ADD CONSTRAINT intel_jobs_pkey PRIMARY KEY (id);


--
-- Name: intel_raw_documents intel_raw_documents_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.intel_raw_documents
    ADD CONSTRAINT intel_raw_documents_pkey PRIMARY KEY (id);


--
-- Name: intel_records intel_records_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.intel_records
    ADD CONSTRAINT intel_records_pkey PRIMARY KEY (id);


--
-- Name: intel_review_log intel_review_log_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.intel_review_log
    ADD CONSTRAINT intel_review_log_pkey PRIMARY KEY (id);


--
-- Name: intel_source_channels intel_source_channels_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.intel_source_channels
    ADD CONSTRAINT intel_source_channels_pkey PRIMARY KEY (id);


--
-- Name: intel_source_channels intel_source_channels_source_id_entry_url_entity_types_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.intel_source_channels
    ADD CONSTRAINT intel_source_channels_source_id_entry_url_entity_types_key UNIQUE (source_id, entry_url, entity_types);


--
-- Name: intel_sources intel_sources_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.intel_sources
    ADD CONSTRAINT intel_sources_pkey PRIMARY KEY (id);


--
-- Name: ip_blocks ip_blocks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ip_blocks
    ADD CONSTRAINT ip_blocks_pkey PRIMARY KEY (id);


--
-- Name: job_applications job_applications_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.job_applications
    ADD CONSTRAINT job_applications_pkey PRIMARY KEY (id);


--
-- Name: jwt_signing_keys jwt_signing_keys_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.jwt_signing_keys
    ADD CONSTRAINT jwt_signing_keys_pkey PRIMARY KEY (kid);


--
-- Name: known_bots known_bots_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.known_bots
    ADD CONSTRAINT known_bots_pkey PRIMARY KEY (id);


--
-- Name: language_score_stories language_score_stories_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.language_score_stories
    ADD CONSTRAINT language_score_stories_pkey PRIMARY KEY (id);


--
-- Name: language_test_requirements language_test_requirements_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.language_test_requirements
    ADD CONSTRAINT language_test_requirements_pkey PRIMARY KEY (id);


--
-- Name: language_test_resources language_test_resources_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.language_test_resources
    ADD CONSTRAINT language_test_resources_pkey PRIMARY KEY (id);


--
-- Name: language_tests language_tests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.language_tests
    ADD CONSTRAINT language_tests_pkey PRIMARY KEY (id);


--
-- Name: language_tests language_tests_slug_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.language_tests
    ADD CONSTRAINT language_tests_slug_key UNIQUE (slug);


--
-- Name: learning_certificates learning_certificates_certificate_number_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.learning_certificates
    ADD CONSTRAINT learning_certificates_certificate_number_key UNIQUE (certificate_number);


--
-- Name: learning_certificates learning_certificates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.learning_certificates
    ADD CONSTRAINT learning_certificates_pkey PRIMARY KEY (id);


--
-- Name: learning_lessons learning_lessons_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.learning_lessons
    ADD CONSTRAINT learning_lessons_pkey PRIMARY KEY (id);


--
-- Name: learning_modules learning_modules_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.learning_modules
    ADD CONSTRAINT learning_modules_pkey PRIMARY KEY (id);


--
-- Name: learning_tracks learning_tracks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.learning_tracks
    ADD CONSTRAINT learning_tracks_pkey PRIMARY KEY (id);


--
-- Name: lesson_progress lesson_progress_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.lesson_progress
    ADD CONSTRAINT lesson_progress_pkey PRIMARY KEY (id);


--
-- Name: lesson_progress lesson_progress_user_id_lesson_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.lesson_progress
    ADD CONSTRAINT lesson_progress_user_id_lesson_id_key UNIQUE (user_id, lesson_id);


--
-- Name: lesson_quizzes lesson_quizzes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.lesson_quizzes
    ADD CONSTRAINT lesson_quizzes_pkey PRIMARY KEY (id);


--
-- Name: media media_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.media
    ADD CONSTRAINT media_pkey PRIMARY KEY (id);


--
-- Name: mentor_reviews mentor_reviews_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mentor_reviews
    ADD CONSTRAINT mentor_reviews_pkey PRIMARY KEY (id);


--
-- Name: mentor_reviews mentor_reviews_session_id_reviewer_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mentor_reviews
    ADD CONSTRAINT mentor_reviews_session_id_reviewer_id_key UNIQUE (session_id, reviewer_id);


--
-- Name: mentor_sessions mentor_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mentor_sessions
    ADD CONSTRAINT mentor_sessions_pkey PRIMARY KEY (id);


--
-- Name: mentorship_connections mentorship_connections_mentor_id_mentee_id_status_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mentorship_connections
    ADD CONSTRAINT mentorship_connections_mentor_id_mentee_id_status_key UNIQUE (mentor_id, mentee_id, status);


--
-- Name: mentorship_connections mentorship_connections_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mentorship_connections
    ADD CONSTRAINT mentorship_connections_pkey PRIMARY KEY (id);


--
-- Name: mentorship_matches mentorship_matches_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mentorship_matches
    ADD CONSTRAINT mentorship_matches_pkey PRIMARY KEY (id);


--
-- Name: mentorship_profiles mentorship_profiles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mentorship_profiles
    ADD CONSTRAINT mentorship_profiles_pkey PRIMARY KEY (id);


--
-- Name: mentorship_profiles mentorship_profiles_user_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mentorship_profiles
    ADD CONSTRAINT mentorship_profiles_user_id_key UNIQUE (user_id);


--
-- Name: mentorship_programs mentorship_programs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mentorship_programs
    ADD CONSTRAINT mentorship_programs_pkey PRIMARY KEY (id);


--
-- Name: menu_items menu_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.menu_items
    ADD CONSTRAINT menu_items_pkey PRIMARY KEY (id);


--
-- Name: menus menus_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.menus
    ADD CONSTRAINT menus_pkey PRIMARY KEY (id);


--
-- Name: moderation_alerts moderation_alerts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.moderation_alerts
    ADD CONSTRAINT moderation_alerts_pkey PRIMARY KEY (id);


--
-- Name: moderation_appeals moderation_appeals_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.moderation_appeals
    ADD CONSTRAINT moderation_appeals_pkey PRIMARY KEY (id);


--
-- Name: monitored_embassies monitored_embassies_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.monitored_embassies
    ADD CONSTRAINT monitored_embassies_pkey PRIMARY KEY (id);


--
-- Name: mutual_aid_posts mutual_aid_posts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mutual_aid_posts
    ADD CONSTRAINT mutual_aid_posts_pkey PRIMARY KEY (id);


--
-- Name: newsletter_campaigns newsletter_campaigns_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.newsletter_campaigns
    ADD CONSTRAINT newsletter_campaigns_pkey PRIMARY KEY (id);


--
-- Name: newsletter_list_subscribers newsletter_list_subscribers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.newsletter_list_subscribers
    ADD CONSTRAINT newsletter_list_subscribers_pkey PRIMARY KEY (list_id, subscriber_id);


--
-- Name: newsletter_lists newsletter_lists_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.newsletter_lists
    ADD CONSTRAINT newsletter_lists_name_key UNIQUE (name);


--
-- Name: newsletter_lists newsletter_lists_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.newsletter_lists
    ADD CONSTRAINT newsletter_lists_pkey PRIMARY KEY (id);


--
-- Name: newsletter_send_state newsletter_send_state_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.newsletter_send_state
    ADD CONSTRAINT newsletter_send_state_pkey PRIMARY KEY (campaign_id, recipient);


--
-- Name: newsletter_subscribers newsletter_subscribers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.newsletter_subscribers
    ADD CONSTRAINT newsletter_subscribers_pkey PRIMARY KEY (id);


--
-- Name: newsletter_unsubscribe_tokens newsletter_unsubscribe_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.newsletter_unsubscribe_tokens
    ADD CONSTRAINT newsletter_unsubscribe_tokens_pkey PRIMARY KEY (token);


--
-- Name: notification_batch_windows notification_batch_windows_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notification_batch_windows
    ADD CONSTRAINT notification_batch_windows_pkey PRIMARY KEY (id);


--
-- Name: notification_batch_windows notification_batch_windows_recipient_id_notification_type_s_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notification_batch_windows
    ADD CONSTRAINT notification_batch_windows_recipient_id_notification_type_s_key UNIQUE (recipient_id, notification_type, subject_type, subject_id);


--
-- Name: notification_preferences notification_preferences_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notification_preferences
    ADD CONSTRAINT notification_preferences_pkey PRIMARY KEY (user_id, notification_type);


--
-- Name: notification_types notification_types_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notification_types
    ADD CONSTRAINT notification_types_pkey PRIMARY KEY (key);


--
-- Name: notifications notifications_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notifications
    ADD CONSTRAINT notifications_pkey PRIMARY KEY (id);


--
-- Name: oauth_accounts oauth_accounts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.oauth_accounts
    ADD CONSTRAINT oauth_accounts_pkey PRIMARY KEY (id);


--
-- Name: onboarding_state onboarding_state_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.onboarding_state
    ADD CONSTRAINT onboarding_state_pkey PRIMARY KEY (id);


--
-- Name: onboarding_templates onboarding_templates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.onboarding_templates
    ADD CONSTRAINT onboarding_templates_pkey PRIMARY KEY (id);


--
-- Name: organization_boosts organization_boosts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organization_boosts
    ADD CONSTRAINT organization_boosts_pkey PRIMARY KEY (id);


--
-- Name: organization_invitations organization_invitations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organization_invitations
    ADD CONSTRAINT organization_invitations_pkey PRIMARY KEY (id);


--
-- Name: organization_invitations organization_invitations_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organization_invitations
    ADD CONSTRAINT organization_invitations_token_hash_key UNIQUE (token_hash);


--
-- Name: organization_members organization_members_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organization_members
    ADD CONSTRAINT organization_members_pkey PRIMARY KEY (organization_id, user_id);


--
-- Name: organizations organizations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organizations
    ADD CONSTRAINT organizations_pkey PRIMARY KEY (id);


--
-- Name: ownership_claims ownership_claims_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ownership_claims
    ADD CONSTRAINT ownership_claims_pkey PRIMARY KEY (id);


--
-- Name: page_builder_pages page_builder_pages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_builder_pages
    ADD CONSTRAINT page_builder_pages_pkey PRIMARY KEY (id);


--
-- Name: pagebuilder_blocks pagebuilder_blocks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pagebuilder_blocks
    ADD CONSTRAINT pagebuilder_blocks_pkey PRIMARY KEY (id);


--
-- Name: pagebuilder_templates pagebuilder_templates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pagebuilder_templates
    ADD CONSTRAINT pagebuilder_templates_pkey PRIMARY KEY (id);


--
-- Name: pagebuilder_themes pagebuilder_themes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pagebuilder_themes
    ADD CONSTRAINT pagebuilder_themes_pkey PRIMARY KEY (id);


--
-- Name: pages pages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pages
    ADD CONSTRAINT pages_pkey PRIMARY KEY (id);


--
-- Name: password_history password_history_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.password_history
    ADD CONSTRAINT password_history_pkey PRIMARY KEY (id);


--
-- Name: password_reset_tokens password_reset_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.password_reset_tokens
    ADD CONSTRAINT password_reset_tokens_pkey PRIMARY KEY (id);


--
-- Name: payment_refunds payment_refunds_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payment_refunds
    ADD CONSTRAINT payment_refunds_pkey PRIMARY KEY (id);


--
-- Name: payment_webhook_events payment_webhook_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payment_webhook_events
    ADD CONSTRAINT payment_webhook_events_pkey PRIMARY KEY (id);


--
-- Name: payments payments_idempotency_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payments
    ADD CONSTRAINT payments_idempotency_key_key UNIQUE (idempotency_key);


--
-- Name: payments payments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payments
    ADD CONSTRAINT payments_pkey PRIMARY KEY (id);


--
-- Name: payout_providers payout_providers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payout_providers
    ADD CONSTRAINT payout_providers_pkey PRIMARY KEY (id);


--
-- Name: permissions permissions_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.permissions
    ADD CONSTRAINT permissions_key_key UNIQUE (key);


--
-- Name: permissions permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.permissions
    ADD CONSTRAINT permissions_pkey PRIMARY KEY (id);


--
-- Name: persona_voices persona_voices_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.persona_voices
    ADD CONSTRAINT persona_voices_pkey PRIMARY KEY (id);


--
-- Name: planning_group_checkins planning_group_checkins_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.planning_group_checkins
    ADD CONSTRAINT planning_group_checkins_pkey PRIMARY KEY (id);


--
-- Name: planning_group_members planning_group_members_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.planning_group_members
    ADD CONSTRAINT planning_group_members_pkey PRIMARY KEY (group_id, user_id);


--
-- Name: planning_group_milestones planning_group_milestones_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.planning_group_milestones
    ADD CONSTRAINT planning_group_milestones_pkey PRIMARY KEY (id);


--
-- Name: planning_groups planning_groups_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.planning_groups
    ADD CONSTRAINT planning_groups_pkey PRIMARY KEY (id);


--
-- Name: platform_data_exports platform_data_exports_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.platform_data_exports
    ADD CONSTRAINT platform_data_exports_pkey PRIMARY KEY (id);


--
-- Name: platform_data_imports platform_data_imports_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.platform_data_imports
    ADD CONSTRAINT platform_data_imports_pkey PRIMARY KEY (id);


--
-- Name: platform_events platform_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.platform_events
    ADD CONSTRAINT platform_events_pkey PRIMARY KEY (id);


--
-- Name: pof_requirements pof_requirements_destination_space_id_visa_type_program_lab_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pof_requirements
    ADD CONSTRAINT pof_requirements_destination_space_id_visa_type_program_lab_key UNIQUE (destination_space_id, visa_type, program_label);


--
-- Name: pof_requirements pof_requirements_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pof_requirements
    ADD CONSTRAINT pof_requirements_pkey PRIMARY KEY (id);


--
-- Name: professional_documents professional_documents_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.professional_documents
    ADD CONSTRAINT professional_documents_pkey PRIMARY KEY (id);


--
-- Name: professional_relocations professional_relocations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.professional_relocations
    ADD CONSTRAINT professional_relocations_pkey PRIMARY KEY (id);


--
-- Name: professional_verifiers professional_verifiers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.professional_verifiers
    ADD CONSTRAINT professional_verifiers_pkey PRIMARY KEY (id);


--
-- Name: push_subscriptions push_subscriptions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.push_subscriptions
    ADD CONSTRAINT push_subscriptions_pkey PRIMARY KEY (id);


--
-- Name: push_subscriptions push_subscriptions_user_id_endpoint_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.push_subscriptions
    ADD CONSTRAINT push_subscriptions_user_id_endpoint_key UNIQUE (user_id, endpoint);


--
-- Name: push_tokens push_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.push_tokens
    ADD CONSTRAINT push_tokens_pkey PRIMARY KEY (id);


--
-- Name: push_tokens push_tokens_user_id_token_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.push_tokens
    ADD CONSTRAINT push_tokens_user_id_token_key UNIQUE (user_id, token);


--
-- Name: reaction_types reaction_types_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reaction_types
    ADD CONSTRAINT reaction_types_pkey PRIMARY KEY (key);


--
-- Name: reactions reactions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reactions
    ADD CONSTRAINT reactions_pkey PRIMARY KEY (entity_type, entity_id, user_id, reaction_key);


--
-- Name: redirects redirects_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.redirects
    ADD CONSTRAINT redirects_pkey PRIMARY KEY (id);


--
-- Name: referral_codes referral_codes_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.referral_codes
    ADD CONSTRAINT referral_codes_code_key UNIQUE (code);


--
-- Name: referral_codes referral_codes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.referral_codes
    ADD CONSTRAINT referral_codes_pkey PRIMARY KEY (id);


--
-- Name: referral_codes referral_codes_user_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.referral_codes
    ADD CONSTRAINT referral_codes_user_id_key UNIQUE (user_id);


--
-- Name: referral_programs referral_programs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.referral_programs
    ADD CONSTRAINT referral_programs_pkey PRIMARY KEY (id);


--
-- Name: referrals referrals_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.referrals
    ADD CONSTRAINT referrals_pkey PRIMARY KEY (id);


--
-- Name: refresh_tokens refresh_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.refresh_tokens
    ADD CONSTRAINT refresh_tokens_pkey PRIMARY KEY (id);


--
-- Name: refresh_tokens refresh_tokens_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.refresh_tokens
    ADD CONSTRAINT refresh_tokens_token_hash_key UNIQUE (token_hash);


--
-- Name: remittance_providers remittance_providers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remittance_providers
    ADD CONSTRAINT remittance_providers_pkey PRIMARY KEY (id);


--
-- Name: remittance_rate_alerts remittance_rate_alerts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remittance_rate_alerts
    ADD CONSTRAINT remittance_rate_alerts_pkey PRIMARY KEY (id);


--
-- Name: remittance_rates remittance_rates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remittance_rates
    ADD CONSTRAINT remittance_rates_pkey PRIMARY KEY (id);


--
-- Name: reply_views reply_views_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reply_views
    ADD CONSTRAINT reply_views_pkey PRIMARY KEY (reply_id, user_id);


--
-- Name: revenue_splits revenue_splits_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.revenue_splits
    ADD CONSTRAINT revenue_splits_pkey PRIMARY KEY (id);


--
-- Name: review_aggregates review_aggregates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.review_aggregates
    ADD CONSTRAINT review_aggregates_pkey PRIMARY KEY (entity_type, entity_id);


--
-- Name: reviews reviews_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reviews
    ADD CONSTRAINT reviews_pkey PRIMARY KEY (id);


--
-- Name: role_permissions role_permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT role_permissions_pkey PRIMARY KEY (role_id, permission_id);


--
-- Name: saved_scholarships saved_scholarships_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.saved_scholarships
    ADD CONSTRAINT saved_scholarships_pkey PRIMARY KEY (id);


--
-- Name: saved_scholarships saved_scholarships_user_id_guide_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.saved_scholarships
    ADD CONSTRAINT saved_scholarships_user_id_guide_id_key UNIQUE (user_id, guide_id);


--
-- Name: scam_alert_subscriptions scam_alert_subscriptions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scam_alert_subscriptions
    ADD CONSTRAINT scam_alert_subscriptions_pkey PRIMARY KEY (user_id, destination_space_id);


--
-- Name: scam_reports scam_reports_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scam_reports
    ADD CONSTRAINT scam_reports_pkey PRIMARY KEY (id);
-- Name: scholarships scholarships_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scholarships
    ADD CONSTRAINT scholarships_pkey PRIMARY KEY (id);


--
-- Name: search_analytics search_analytics_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.search_analytics
    ADD CONSTRAINT search_analytics_pkey PRIMARY KEY (id);


--
-- Name: security_scan_results security_scan_results_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.security_scan_results
    ADD CONSTRAINT security_scan_results_pkey PRIMARY KEY (id);


--
-- Name: service_listing_meta service_listing_meta_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.service_listing_meta
    ADD CONSTRAINT service_listing_meta_pkey PRIMARY KEY (id);


--
-- Name: share_platforms share_platforms_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.share_platforms
    ADD CONSTRAINT share_platforms_pkey PRIMARY KEY (id);


--
-- Name: sidebar_widgets sidebar_blocks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sidebar_widgets
    ADD CONSTRAINT sidebar_blocks_pkey PRIMARY KEY (id);


--
-- Name: sidebars sidebars_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sidebars
    ADD CONSTRAINT sidebars_pkey PRIMARY KEY (id);


--
-- Name: slug_history slug_history_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.slug_history
    ADD CONSTRAINT slug_history_pkey PRIMARY KEY (id);


--
-- Name: sop_drafts sop_drafts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sop_drafts
    ADD CONSTRAINT sop_drafts_pkey PRIMARY KEY (id);


--
-- Name: sop_samples sop_samples_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sop_samples
    ADD CONSTRAINT sop_samples_pkey PRIMARY KEY (id);


--
-- Name: sop_templates sop_templates_destination_space_id_purpose_title_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sop_templates
    ADD CONSTRAINT sop_templates_destination_space_id_purpose_title_key UNIQUE (destination_space_id, purpose, title);


--
-- Name: sop_templates sop_templates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sop_templates
    ADD CONSTRAINT sop_templates_pkey PRIMARY KEY (id);


--
-- Name: source_collection_items source_collection_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.source_collection_items
    ADD CONSTRAINT source_collection_items_pkey PRIMARY KEY (collection_id, source_id);


--
-- Name: source_collections source_collections_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.source_collections
    ADD CONSTRAINT source_collections_pkey PRIMARY KEY (id);


--
-- Name: space_members space_members_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_members
    ADD CONSTRAINT space_members_pkey PRIMARY KEY (id);


--
-- Name: space_members space_members_space_id_user_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_members
    ADD CONSTRAINT space_members_space_id_user_id_key UNIQUE (space_id, user_id);


--
-- Name: space_moderators space_moderators_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_moderators
    ADD CONSTRAINT space_moderators_pkey PRIMARY KEY (id);


--
-- Name: space_mutes space_mutes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_mutes
    ADD CONSTRAINT space_mutes_pkey PRIMARY KEY (id);


--
-- Name: space_parent_spaces space_parent_spaces_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_parent_spaces
    ADD CONSTRAINT space_parent_spaces_pkey PRIMARY KEY (space_id, parent_space_id);


--
-- Name: space_permission_presets space_permission_presets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_permission_presets
    ADD CONSTRAINT space_permission_presets_pkey PRIMARY KEY (id);


--
-- Name: space_pinned_posts space_pinned_posts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_pinned_posts
    ADD CONSTRAINT space_pinned_posts_pkey PRIMARY KEY (id);


--
-- Name: space_pinned_posts space_pinned_posts_space_id_content_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_pinned_posts
    ADD CONSTRAINT space_pinned_posts_space_id_content_id_key UNIQUE (space_id, content_id);


--
-- Name: space_rules space_rules_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_rules
    ADD CONSTRAINT space_rules_pkey PRIMARY KEY (id);


--
-- Name: space_types space_types_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_types
    ADD CONSTRAINT space_types_pkey PRIMARY KEY (id);


--
-- Name: spaces spaces_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.spaces
    ADD CONSTRAINT spaces_pkey PRIMARY KEY (id);


--
-- Name: stock_providers stock_providers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_providers
    ADD CONSTRAINT stock_providers_pkey PRIMARY KEY (provider_name);


--
-- Name: story_meta story_meta_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.story_meta
    ADD CONSTRAINT story_meta_pkey PRIMARY KEY (id);


--
-- Name: studio_batches studio_batches_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.studio_batches
    ADD CONSTRAINT studio_batches_pkey PRIMARY KEY (id);


--
-- Name: studio_jobs studio_jobs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.studio_jobs
    ADD CONSTRAINT studio_jobs_pkey PRIMARY KEY (id);


--
-- Name: subscription_plan_tracks subscription_plan_tracks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscription_plan_tracks
    ADD CONSTRAINT subscription_plan_tracks_pkey PRIMARY KEY (plan_id, track_id);


--
-- Name: subscription_plans subscription_plans_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscription_plans
    ADD CONSTRAINT subscription_plans_pkey PRIMARY KEY (id);


--
-- Name: subscriptions subscriptions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscriptions
    ADD CONSTRAINT subscriptions_pkey PRIMARY KEY (id);


--
-- Name: subscriptions subscriptions_provider_sub_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscriptions
    ADD CONSTRAINT subscriptions_provider_sub_id_key UNIQUE (provider_sub_id);


--
-- Name: system_settings system_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.system_settings
    ADD CONSTRAINT system_settings_pkey PRIMARY KEY (id);


--
-- Name: tag_moderators tag_moderators_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tag_moderators
    ADD CONSTRAINT tag_moderators_pkey PRIMARY KEY (id);


--
-- Name: tags tags_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tags
    ADD CONSTRAINT tags_pkey PRIMARY KEY (id);


--
-- Name: tax_obligation_guides tax_obligation_guides_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tax_obligation_guides
    ADD CONSTRAINT tax_obligation_guides_pkey PRIMARY KEY (id);


--
-- Name: tax_treaty_pairs tax_treaty_pairs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tax_treaty_pairs
    ADD CONSTRAINT tax_treaty_pairs_pkey PRIMARY KEY (id);


--
-- Name: template_literals template_literals_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.template_literals
    ADD CONSTRAINT template_literals_pkey PRIMARY KEY (id);


--
-- Name: ticket_attachments ticket_attachments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_attachments
    ADD CONSTRAINT ticket_attachments_pkey PRIMARY KEY (id);


--
-- Name: ticket_comments ticket_comments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_comments
    ADD CONSTRAINT ticket_comments_pkey PRIMARY KEY (id);


--
-- Name: ticket_events ticket_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_events
    ADD CONSTRAINT ticket_events_pkey PRIMARY KEY (id);


--
-- Name: ticket_types ticket_types_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_types
    ADD CONSTRAINT ticket_types_pkey PRIMARY KEY (id);


--
-- Name: tickets tickets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tickets
    ADD CONSTRAINT tickets_pkey PRIMARY KEY (id);


--
-- Name: totp_credentials totp_credentials_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.totp_credentials
    ADD CONSTRAINT totp_credentials_pkey PRIMARY KEY (id);


--
-- Name: translations translations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.translations
    ADD CONSTRAINT translations_pkey PRIMARY KEY (id);


--
-- Name: travel_providers travel_providers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.travel_providers
    ADD CONSTRAINT travel_providers_pkey PRIMARY KEY (id);


--
-- Name: trending_snapshots trending_snapshots_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.trending_snapshots
    ADD CONSTRAINT trending_snapshots_pkey PRIMARY KEY (id);


--
-- Name: university_course_catalog university_course_catalog_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.university_course_catalog
    ADD CONSTRAINT university_course_catalog_pkey PRIMARY KEY (id);


--
-- Name: university_course_offerings university_course_offerings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.university_course_offerings
    ADD CONSTRAINT university_course_offerings_pkey PRIMARY KEY (id);


--
-- Name: university_track_categories university_track_categories_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.university_track_categories
    ADD CONSTRAINT university_track_categories_pkey PRIMARY KEY (id);


--
-- Name: learning_certificates uq_learning_certificates_user_track; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.learning_certificates
    ADD CONSTRAINT uq_learning_certificates_user_track UNIQUE (user_id, track_id);


--
-- Name: user_affiliate_links user_affiliate_links_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_affiliate_links
    ADD CONSTRAINT user_affiliate_links_code_key UNIQUE (code);


--
-- Name: user_affiliate_links user_affiliate_links_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_affiliate_links
    ADD CONSTRAINT user_affiliate_links_pkey PRIMARY KEY (id);


--
-- Name: user_badges user_badges_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_badges
    ADD CONSTRAINT user_badges_pkey PRIMARY KEY (id);


--
-- Name: user_blocks user_blocks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_blocks
    ADD CONSTRAINT user_blocks_pkey PRIMARY KEY (blocker_id, blocked_id);


--
-- Name: user_challenge_progress user_challenge_progress_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_challenge_progress
    ADD CONSTRAINT user_challenge_progress_pkey PRIMARY KEY (id);


--
-- Name: user_content_views user_content_views_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_content_views
    ADD CONSTRAINT user_content_views_pkey PRIMARY KEY (id);


--
-- Name: user_dm_settings user_dm_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_dm_settings
    ADD CONSTRAINT user_dm_settings_pkey PRIMARY KEY (user_id);


--
-- Name: user_group_memberships user_group_memberships_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_group_memberships
    ADD CONSTRAINT user_group_memberships_pkey PRIMARY KEY (id);


--
-- Name: user_group_promotions user_group_promotions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_group_promotions
    ADD CONSTRAINT user_group_promotions_pkey PRIMARY KEY (id);


--
-- Name: user_groups user_groups_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_groups
    ADD CONSTRAINT user_groups_pkey PRIMARY KEY (id);


--
-- Name: user_moderation_actions user_moderation_actions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_moderation_actions
    ADD CONSTRAINT user_moderation_actions_pkey PRIMARY KEY (id);


--
-- Name: user_mutes user_mutes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_mutes
    ADD CONSTRAINT user_mutes_pkey PRIMARY KEY (id);


--
-- Name: user_operation_bans user_operation_bans_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_operation_bans
    ADD CONSTRAINT user_operation_bans_pkey PRIMARY KEY (id);


--
-- Name: user_preferences user_preferences_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_preferences
    ADD CONSTRAINT user_preferences_pkey PRIMARY KEY (id);


--
-- Name: user_privacy_settings user_privacy_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_privacy_settings
    ADD CONSTRAINT user_privacy_settings_pkey PRIMARY KEY (user_id);


--
-- Name: user_profiles user_profiles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_profiles
    ADD CONSTRAINT user_profiles_pkey PRIMARY KEY (id);


--
-- Name: user_roles user_roles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_roles
    ADD CONSTRAINT user_roles_pkey PRIMARY KEY (id);


--
-- Name: user_source_blocks user_source_blocks_blocker_source_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_source_blocks
    ADD CONSTRAINT user_source_blocks_blocker_source_key UNIQUE (blocker_id, source_id);


--
-- Name: user_source_blocks user_source_blocks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_source_blocks
    ADD CONSTRAINT user_source_blocks_pkey PRIMARY KEY (id);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: video_channel_claims video_channel_claims_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.video_channel_claims
    ADD CONSTRAINT video_channel_claims_pkey PRIMARY KEY (id);


--
-- Name: video_import_jobs video_import_jobs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.video_import_jobs
    ADD CONSTRAINT video_import_jobs_pkey PRIMARY KEY (id);


--
-- Name: video_sources video_sources_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.video_sources
    ADD CONSTRAINT video_sources_pkey PRIMARY KEY (id);


--
-- Name: video_sources video_sources_source_url_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.video_sources
    ADD CONSTRAINT video_sources_source_url_key UNIQUE (source_url);


--
-- Name: visa_processing_stats visa_processing_stats_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.visa_processing_stats
    ADD CONSTRAINT visa_processing_stats_pkey PRIMARY KEY (id);


--
-- Name: visa_routes visa_routes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.visa_routes
    ADD CONSTRAINT visa_routes_pkey PRIMARY KEY (id);


--
-- Name: visa_timeline_milestones visa_timeline_milestones_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.visa_timeline_milestones
    ADD CONSTRAINT visa_timeline_milestones_pkey PRIMARY KEY (id);


--
-- Name: visa_timelines visa_timelines_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.visa_timelines
    ADD CONSTRAINT visa_timelines_pkey PRIMARY KEY (id);


--
-- Name: wallet_ledger wallet_ledger_idempotency_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wallet_ledger
    ADD CONSTRAINT wallet_ledger_idempotency_key_key UNIQUE (idempotency_key);


--
-- Name: wallet_ledger wallet_ledger_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wallet_ledger
    ADD CONSTRAINT wallet_ledger_pkey PRIMARY KEY (id);


--
-- Name: wallet_settings wallet_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wallet_settings
    ADD CONSTRAINT wallet_settings_pkey PRIMARY KEY (id);


--
-- Name: wallet_withdrawal_fee_tiers wallet_withdrawal_fee_tiers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wallet_withdrawal_fee_tiers
    ADD CONSTRAINT wallet_withdrawal_fee_tiers_pkey PRIMARY KEY (id);


--
-- Name: wallet_withdrawal_methods wallet_withdrawal_methods_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wallet_withdrawal_methods
    ADD CONSTRAINT wallet_withdrawal_methods_pkey PRIMARY KEY (id);


--
-- Name: wallet_withdrawals wallet_withdrawals_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wallet_withdrawals
    ADD CONSTRAINT wallet_withdrawals_pkey PRIMARY KEY (id);


--
-- Name: webhook_deliveries webhook_deliveries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webhook_deliveries
    ADD CONSTRAINT webhook_deliveries_pkey PRIMARY KEY (id);


--
-- Name: webhook_endpoints webhook_endpoints_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webhook_endpoints
    ADD CONSTRAINT webhook_endpoints_pkey PRIMARY KEY (id);


--
-- Name: widget_configurations widget_configurations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.widget_configurations
    ADD CONSTRAINT widget_configurations_pkey PRIMARY KEY (id);


--
-- Name: wiki_pages wiki_pages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wiki_pages
    ADD CONSTRAINT wiki_pages_pkey PRIMARY KEY (id);


--
-- Name: wiki_revisions wiki_revisions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wiki_revisions
    ADD CONSTRAINT wiki_revisions_pkey PRIMARY KEY (id);


--
-- Name: ws_tickets ws_tickets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ws_tickets
    ADD CONSTRAINT ws_tickets_pkey PRIMARY KEY (id);


--
-- Name: activity_events_tenant_id_activity_key_reference_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX activity_events_tenant_id_activity_key_reference_key ON public.activity_events USING btree (activity_key, reference);


--
-- Name: activity_point_rules_tenant_id_activity_key_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX activity_point_rules_tenant_id_activity_key_key ON public.activity_point_rules USING btree (activity_key);


--
-- Name: ad_analytics_daily_tenant_id_campaign_id_slot_type_id_date_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX ad_analytics_daily_tenant_id_campaign_id_slot_type_id_date_key ON public.ad_analytics_daily USING btree (campaign_id, slot_type_id, date);


--
-- Name: ad_networks_tenant_id_provider_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX ad_networks_tenant_id_provider_key ON public.ad_networks USING btree (provider);


--
-- Name: ad_slot_types_tenant_id_slot_name_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX ad_slot_types_tenant_id_slot_name_key ON public.ad_slot_types USING btree (slot_name);


--
-- Name: affiliate_providers_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX affiliate_providers_tenant_id_slug_key ON public.affiliate_providers USING btree (slug);


--
-- Name: ai_tools_tenant_id_name_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX ai_tools_tenant_id_name_key ON public.ai_tools USING btree (name);


--
-- Name: arrival_checklist_guides_tenant_id_destination_space_id_vis_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX arrival_checklist_guides_tenant_id_destination_space_id_vis_key ON public.arrival_checklist_guides USING btree (destination_space_id, visa_category, applicant_type, version);


--
-- Name: attestation_pathways_tenant_id_document_type_issuing_space__key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX attestation_pathways_tenant_id_document_type_issuing_space__key ON public.attestation_pathways USING btree (document_type, issuing_space_id, destination_space_id);


--
-- Name: badge_definitions_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX badge_definitions_tenant_id_slug_key ON public.badge_definitions USING btree (slug);


--
-- Name: blog_series_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX blog_series_tenant_id_slug_key ON public.blog_series USING btree (slug);


--
-- Name: challenge_definitions_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX challenge_definitions_tenant_id_slug_key ON public.challenge_definitions USING btree (slug);


--
-- Name: checklist_progress_tenant_id_user_id_guide_id_item_key_list_typ; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX checklist_progress_tenant_id_user_id_guide_id_item_key_list_typ ON public.checklist_progress USING btree (user_id, guide_id, item_key, list_type);


--
-- Name: community_organisations_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX community_organisations_tenant_id_slug_key ON public.community_organisations USING btree (slug);


--
-- Name: comparison_score_factors_tenant_id_comparison_type_id_facto_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX comparison_score_factors_tenant_id_comparison_type_id_facto_key ON public.comparison_score_factors USING btree (comparison_type_id, factor_key);


--
-- Name: comparison_scores_tenant_content_item_factor_profile_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX comparison_scores_tenant_content_item_factor_profile_key ON public.comparison_scores USING btree (content_id, item_id, factor_id, profile_id);


--
-- Name: comparison_type_definitions_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX comparison_type_definitions_tenant_id_slug_key ON public.comparison_type_definitions USING btree (slug);


--
-- Name: content_embeddings_tenant_id_entity_type_entity_id_chunk_in_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX content_embeddings_tenant_id_entity_type_entity_id_chunk_in_key ON public.content_embeddings USING btree (entity_type, entity_id, chunk_index);


--
-- Name: content_featured_tenant_id_content_id_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX content_featured_tenant_id_content_id_key ON public.content_featured USING btree (content_id);


--
-- Name: content_type_jsonld_templates_tenant_id_content_type_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX content_type_jsonld_templates_tenant_id_content_type_key ON public.content_type_jsonld_templates USING btree (content_type);


--
-- Name: coupons_tenant_id_code_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX coupons_tenant_id_code_key ON public.coupons USING btree (code);


--
-- Name: credential_pathways_tenant_id_profession_id_issuing_space_i_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX credential_pathways_tenant_id_profession_id_issuing_space_i_key ON public.credential_pathways USING btree (profession_id, issuing_space_id, destination_space_id);


--
-- Name: credit_packs_tenant_id_name_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX credit_packs_tenant_id_name_key ON public.credit_packs USING btree (name);


--
-- Name: custom_field_entity_types_tenant_id_value_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX custom_field_entity_types_tenant_id_value_key ON public.custom_field_entity_types USING btree (value);


--
-- Name: custom_field_groups_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX custom_field_groups_tenant_id_slug_key ON public.custom_field_groups USING btree (slug);


--
-- Name: custom_field_templates_tenant_id_name_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX custom_field_templates_tenant_id_name_key ON public.custom_field_templates USING btree (name);


--
-- Name: device_tokens_tenant_id_user_id_device_id_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX device_tokens_tenant_id_user_id_device_id_key ON public.device_tokens USING btree (user_id, device_id);


--
-- Name: discount_codes_tenant_id_code_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX discount_codes_tenant_id_code_key ON public.discount_codes USING btree (code);


--
-- Name: dm_conversations_tenant_id_participant_1_id_participant_2_i_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX dm_conversations_tenant_id_participant_1_id_participant_2_i_key ON public.dm_conversations USING btree (participant_1_id, participant_2_id);


--
-- Name: doc_pages_tenant_id_audience_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX doc_pages_tenant_id_audience_slug_key ON public.doc_pages USING btree (audience, slug);


--
-- Name: editor_block_permissions_tenant_id_block_type_user_group_id_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX editor_block_permissions_tenant_id_block_type_user_group_id_key ON public.editor_block_permissions USING btree (block_type, user_group_id);


--
-- Name: email_templates_tenant_id_type_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX email_templates_tenant_id_type_key ON public.email_templates USING btree (type);


--
-- Name: email_templates_tenant_slug_uniq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX email_templates_tenant_slug_uniq ON public.email_templates USING btree (slug);


--
-- Name: forms_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX forms_tenant_id_slug_key ON public.forms USING btree (slug);


--
-- Name: forum_tags_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX forum_tags_tenant_id_slug_key ON public.forum_tags USING btree (slug);


--
-- Name: housing_guides_tenant_id_destination_space_id_city_space_id_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX housing_guides_tenant_id_destination_space_id_city_space_id_key ON public.housing_guides USING btree (destination_space_id, city_space_id, accommodation_type);


--
-- Name: idx_account_deletion_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_account_deletion_user ON public.account_deletion_requests USING btree (user_id, status);


--
-- Name: idx_achievement_meta_tenant_dest; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_achievement_meta_tenant_dest ON public.achievement_meta USING btree (destination_space_id);


--
-- Name: idx_achievement_meta_tenant_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_achievement_meta_tenant_type ON public.achievement_meta USING btree (milestone_type);


--
-- Name: idx_activity_events_unprocessed; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_activity_events_unprocessed ON public.activity_events USING btree (processed, created_at) WHERE (processed = false);


--
-- Name: idx_activity_events_user_key; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_activity_events_user_key ON public.activity_events USING btree (user_id, activity_key);


--
-- Name: idx_activity_point_rules_key; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_activity_point_rules_key ON public.activity_point_rules USING btree (activity_key);


--
-- Name: idx_ad_analytics_campaign_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ad_analytics_campaign_date ON public.ad_analytics_daily USING btree (campaign_id, date);


--
-- Name: idx_ad_campaigns_dates; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ad_campaigns_dates ON public.ad_campaigns USING btree (start_date, end_date);


--
-- Name: idx_ad_campaigns_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ad_campaigns_org ON public.ad_campaigns USING btree (organization_id, status) WHERE (organization_id IS NOT NULL);


--
-- Name: idx_ad_campaigns_slot; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ad_campaigns_slot ON public.ad_campaigns USING btree (slot_type_id, status);


--
-- Name: idx_ad_campaigns_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ad_campaigns_status ON public.ad_campaigns USING btree (status);


--
-- Name: idx_ad_events_campaign; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ad_events_campaign ON public.ad_events USING btree (campaign_id, event_type);


--
-- Name: idx_ad_events_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ad_events_user ON public.ad_events USING btree (user_id, event_type);


--
-- Name: idx_ad_events_user_campaign_impression; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ad_events_user_campaign_impression ON public.ad_events USING btree (user_id, campaign_id, occurred_at) WHERE (event_type = 'impression'::text);


--
-- Name: idx_ad_slot_types_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ad_slot_types_name ON public.ad_slot_types USING btree (slot_name);


--
-- Name: idx_ads_campaigns_advertiser; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ads_campaigns_advertiser ON public.ads_campaigns USING btree (advertiser_id);


--
-- Name: idx_ads_campaigns_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ads_campaigns_status ON public.ads_campaigns USING btree (status);


--
-- Name: idx_aff_comm_author; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_aff_comm_author ON public.affiliate_content_commissions USING btree (author_user_id, created_at DESC);


--
-- Name: idx_aff_comm_pending; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_aff_comm_pending ON public.affiliate_content_commissions USING btree (credited, created_at) WHERE (credited = false);


--
-- Name: idx_affiliate_providers_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_affiliate_providers_active ON public.affiliate_providers USING btree (is_active);


--
-- Name: idx_ai_agent_sessions_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_agent_sessions_status ON public.ai_agent_sessions USING btree (status, created_at DESC);


--
-- Name: idx_ai_agent_sessions_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_agent_sessions_user ON public.ai_agent_sessions USING btree (user_id, created_at DESC);


--
-- Name: idx_ai_agent_tool_calls_session; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_agent_tool_calls_session ON public.ai_agent_tool_calls USING btree (session_id, created_at);


--
-- Name: idx_ai_agent_tool_calls_tool; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_agent_tool_calls_tool ON public.ai_agent_tool_calls USING btree (tool_name, status, created_at DESC);


--
-- Name: idx_ai_hitl_reviews_source; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_hitl_reviews_source ON public.ai_hitl_reviews USING btree (source, status, created_at DESC);


--
-- Name: idx_ai_hitl_reviews_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_hitl_reviews_status ON public.ai_hitl_reviews USING btree (status);


--
-- Name: idx_ai_hitl_reviews_target_table; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_hitl_reviews_target_table ON public.ai_hitl_reviews USING btree (target_table, status);


--
-- Name: idx_ai_hitl_reviews_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_hitl_reviews_tenant ON public.ai_hitl_reviews USING btree (created_at DESC);


--
-- Name: idx_ai_personas_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_personas_tenant ON public.ai_personas USING btree (status);


--
-- Name: idx_ai_personas_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_personas_user ON public.ai_personas USING btree (user_id);


--
-- Name: idx_ai_providers_tenant_enabled; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_providers_tenant_enabled ON public.ai_providers USING btree (enabled);


--
-- Name: idx_ai_tool_usage_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_tool_usage_tenant ON public.ai_tool_usage USING btree (created_at DESC);


--
-- Name: idx_ai_tool_usage_tool; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_tool_usage_tool ON public.ai_tool_usage USING btree (tool_id, created_at DESC);


--
-- Name: idx_ai_tools_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_tools_tenant ON public.ai_tools USING btree (category);


--
-- Name: idx_ai_usage_log_tenant_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_usage_log_tenant_created ON public.ai_usage_log USING btree (created_at DESC);


--
-- Name: idx_ai_usage_log_tenant_provider; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_usage_log_tenant_provider ON public.ai_usage_log USING btree (provider, created_at DESC);


--
-- Name: idx_ai_usage_log_tenant_task; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_usage_log_tenant_task ON public.ai_usage_log USING btree (task_type, created_at DESC);


--
-- Name: idx_ale_dedup; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ale_dedup ON public.affiliate_link_events USING btree (link_id, ip_hash, created_at DESC);


--
-- Name: idx_ale_link; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ale_link ON public.affiliate_link_events USING btree (link_id, event_type, created_at DESC);


--
-- Name: idx_api_key_ip_allowlist_key; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_api_key_ip_allowlist_key ON public.api_key_ip_allowlist USING btree (api_key_id);


--
-- Name: idx_api_keys_hash; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_api_keys_hash ON public.api_keys USING btree (key_hash);


--
-- Name: idx_api_keys_prefix; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_api_keys_prefix ON public.api_keys USING btree (key_prefix);


--
-- Name: idx_api_keys_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_api_keys_status ON public.api_keys USING btree (status);


--
-- Name: idx_api_keys_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_api_keys_user ON public.api_keys USING btree (user_id);


--
-- Name: idx_appointment_alerts_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_appointment_alerts_deleted_at ON public.appointment_alerts USING btree (deleted_at);


--
-- Name: idx_appointment_alerts_tenant_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_appointment_alerts_tenant_status ON public.appointment_alerts USING btree (status, embassy_slug);


--
-- Name: idx_appointment_alerts_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_appointment_alerts_user ON public.appointment_alerts USING btree (user_id, status);


--
-- Name: idx_arrival_checklist_guides_tenant_dest; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_arrival_checklist_guides_tenant_dest ON public.arrival_checklist_guides USING btree (destination_space_id, is_active);


--
-- Name: idx_arrival_checklist_items_guide; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_arrival_checklist_items_guide ON public.arrival_checklist_items USING btree (guide_id, sort_order);


--
-- Name: idx_assessment_results_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_assessment_results_tenant ON public.assessment_results USING btree (created_at DESC);


--
-- Name: idx_assessment_results_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_assessment_results_user ON public.assessment_results USING btree (user_id, created_at DESC);


--
-- Name: idx_attestation_pathways_tenant_dest; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_attestation_pathways_tenant_dest ON public.attestation_pathways USING btree (destination_space_id, is_active);


--
-- Name: idx_attestation_pathways_tenant_doc; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_attestation_pathways_tenant_doc ON public.attestation_pathways USING btree (document_type, is_active);


--
-- Name: idx_attestation_updates_pathway; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_attestation_updates_pathway ON public.attestation_community_updates USING btree (pathway_id, status);


--
-- Name: idx_audit_impersonation; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_impersonation ON public.audit_logs USING btree (impersonated_user_id, created_at DESC) WHERE (impersonated_user_id IS NOT NULL);


--
-- Name: idx_audit_logs_actor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_logs_actor ON public.audit_logs USING btree (actor_user_id, created_at DESC);


--
-- Name: idx_audit_logs_target; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_logs_target ON public.audit_logs USING btree (target_type, target_id) WHERE (target_id IS NOT NULL);


--
-- Name: idx_audit_logs_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_logs_tenant ON public.audit_logs USING btree (action_type, created_at DESC);


--
-- Name: idx_badge_definitions_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_badge_definitions_tenant ON public.badge_definitions USING btree (is_active, display_order);


--
-- Name: idx_blog_series_author; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_blog_series_author ON public.blog_series USING btree (author_user_id);


--
-- Name: idx_blog_series_posts_content; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_blog_series_posts_content ON public.blog_series_posts USING btree (content_id);


--
-- Name: idx_blog_series_posts_position; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_blog_series_posts_position ON public.blog_series_posts USING btree (series_id, "position");


--
-- Name: idx_blog_series_posts_series; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_blog_series_posts_series ON public.blog_series_posts USING btree (series_id);


--
-- Name: idx_blog_series_slug; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_blog_series_slug ON public.blog_series USING btree (slug);


--
-- Name: idx_booking_slots_available; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_booking_slots_available ON public.booking_slots USING btree (is_available);


--
-- Name: idx_booking_slots_service; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_booking_slots_service ON public.booking_slots USING btree (service_id);


--
-- Name: idx_bookings_booker; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bookings_booker ON public.bookings USING btree (booker_id);


--
-- Name: idx_bookings_service; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bookings_service ON public.bookings USING btree (service_id);


--
-- Name: idx_bookings_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bookings_status ON public.bookings USING btree (status);


--
-- Name: idx_bookmark_folders_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bookmark_folders_user ON public.bookmark_folders USING btree (user_id);


--
-- Name: idx_bookmarks_entity; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bookmarks_entity ON public.bookmarks USING btree (entity_type, entity_id);


--
-- Name: idx_bookmarks_folder; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bookmarks_folder ON public.bookmarks USING btree (user_id, folder_id);


--
-- Name: idx_bookmarks_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bookmarks_user ON public.bookmarks USING btree (user_id);


--
-- Name: idx_calendar_entries_tenant_dest_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_calendar_entries_tenant_dest_status ON public.calendar_entries USING btree (destination_space_id, status) WHERE (deleted_at IS NULL);


--
-- Name: idx_calendar_entries_tenant_occurs_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_calendar_entries_tenant_occurs_status ON public.calendar_entries USING btree (occurs_at, status) WHERE (deleted_at IS NULL);


--
-- Name: idx_calendar_subscriptions_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_calendar_subscriptions_unique ON public.calendar_subscriptions USING btree (user_id, destination_space_id, category) NULLS NOT DISTINCT;


--
-- Name: idx_cfga_entity; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cfga_entity ON public.custom_field_group_assignments USING btree (entity_class, entity_id);


--
-- Name: idx_cfv_entity; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cfv_entity ON public.custom_field_values USING btree (entity_type, entity_id);


--
-- Name: idx_cfv_filterable; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cfv_filterable ON public.custom_field_values USING btree (field_id, value_text) WHERE (value_text IS NOT NULL);


--
-- Name: idx_cfv_initials_value_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cfv_initials_value_trgm ON public.custom_field_values USING gin (value_text public.gin_trgm_ops) WHERE ((entity_type = 'space'::text) AND (field_id = '00000000-0000-0000-0000-000000000090'::uuid));


--
-- Name: idx_cfv_value_text_lower; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cfv_value_text_lower ON public.custom_field_values USING btree (field_id, lower(value_text)) WHERE (entity_type = 'space'::text);


--
-- Name: idx_challenge_definitions_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_challenge_definitions_tenant ON public.challenge_definitions USING btree (is_active, starts_at DESC);


--
-- Name: idx_challenge_progress_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_challenge_progress_user ON public.user_challenge_progress USING btree (user_id, completed);


--
-- Name: idx_checklist_progress_guide; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_checklist_progress_guide ON public.checklist_progress USING btree (guide_id);


--
-- Name: idx_checklist_progress_list_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_checklist_progress_list_type ON public.checklist_progress USING btree (user_id, list_type);


--
-- Name: idx_checklist_progress_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_checklist_progress_user ON public.checklist_progress USING btree (user_id, guide_id);


--
-- Name: idx_comment_views_comment; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comment_views_comment ON public.comment_views USING btree (comment_id);


--
-- Name: idx_comment_views_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comment_views_user ON public.comment_views USING btree (user_id);


--
-- Name: idx_comment_votes_comment; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comment_votes_comment ON public.comment_votes USING btree (comment_id);


--
-- Name: idx_comment_votes_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comment_votes_user ON public.comment_votes USING btree (user_id);


--
-- Name: idx_comments_author; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comments_author ON public.comments USING btree (author_user_id) WHERE (deleted_at IS NULL);


--
-- Name: idx_comments_content; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comments_content ON public.comments USING btree (content_id) WHERE (deleted_at IS NULL);


--
-- Name: idx_comments_parent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comments_parent ON public.comments USING btree (parent_id) WHERE (deleted_at IS NULL);


--
-- Name: idx_comments_thread; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comments_thread ON public.comments USING btree (content_id, depth, vote_score DESC) WHERE ((status = 'published'::text) AND (deleted_at IS NULL));


--
-- Name: idx_community_organisations_intel_record; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_community_organisations_intel_record ON public.community_organisations USING btree (intel_record_id) WHERE (intel_record_id IS NOT NULL);


--
-- Name: idx_community_orgs_gist_point; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_community_orgs_gist_point ON public.community_organisations USING gist (point((longitude)::double precision, (latitude)::double precision)) WHERE ((latitude IS NOT NULL) AND (longitude IS NOT NULL));


--
-- Name: idx_community_orgs_lat_lng; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_community_orgs_lat_lng ON public.community_organisations USING btree (latitude, longitude) WHERE ((latitude IS NOT NULL) AND (longitude IS NOT NULL));


--
-- Name: idx_community_orgs_tenant_dest_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_community_orgs_tenant_dest_status ON public.community_organisations USING btree (destination_space_id, status);


--
-- Name: idx_community_orgs_tenant_origin; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_community_orgs_tenant_origin ON public.community_organisations USING btree (origin_community, status);


--
-- Name: idx_community_orgs_tenant_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_community_orgs_tenant_type ON public.community_organisations USING btree (org_type, status);


--
-- Name: idx_comp_ingestion_jobs_tenant_content; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comp_ingestion_jobs_tenant_content ON public.comparison_data_ingestion_jobs USING btree (content_id, created_at DESC);


--
-- Name: idx_comparison_content_metadata_promoted_from; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comparison_content_metadata_promoted_from ON public.comparison_content_metadata USING btree (promoted_from_id) WHERE (promoted_from_id IS NOT NULL);


--
-- Name: idx_comparison_content_metadata_tenant_depth; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comparison_content_metadata_tenant_depth ON public.comparison_content_metadata USING btree (depth);


--
-- Name: idx_comparison_data_sources_created_by; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comparison_data_sources_created_by ON public.comparison_data_sources USING btree (created_by_user_id) WHERE (created_by_user_id IS NOT NULL);


--
-- Name: idx_comparison_data_sources_entity; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comparison_data_sources_entity ON public.comparison_data_sources USING btree (entity_type, entity_id);


--
-- Name: idx_comparison_data_sources_source_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comparison_data_sources_source_id ON public.comparison_data_sources USING btree (source_id) WHERE (source_id IS NOT NULL);


--
-- Name: idx_comparison_data_sources_tenant_name; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_comparison_data_sources_tenant_name ON public.comparison_data_sources USING btree (source_name);


--
-- Name: idx_comparison_dimensions_content; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comparison_dimensions_content ON public.comparison_dimensions USING btree (content_id) WHERE (content_id IS NOT NULL);


--
-- Name: idx_comparison_dimensions_not_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comparison_dimensions_not_deleted ON public.comparison_dimensions USING btree (content_id) WHERE (deleted_at IS NULL);


--
-- Name: idx_comparison_dimensions_slug; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comparison_dimensions_slug ON public.comparison_dimensions USING btree (content_id, slug);


--
-- Name: idx_comparison_dimensions_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comparison_dimensions_type ON public.comparison_dimensions USING btree (type_id) WHERE (type_id IS NOT NULL);


--
-- Name: idx_comparison_items_content; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comparison_items_content ON public.comparison_items USING btree (content_id) WHERE (content_id IS NOT NULL);


--
-- Name: idx_comparison_items_intel_record; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comparison_items_intel_record ON public.comparison_items USING btree (intel_record_id) WHERE (intel_record_id IS NOT NULL);


--
-- Name: idx_comparison_items_item_ref; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comparison_items_item_ref ON public.comparison_items USING gin (item_ref) WHERE (item_ref <> '{}'::jsonb);


--
-- Name: idx_comparison_items_not_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comparison_items_not_deleted ON public.comparison_items USING btree (content_id) WHERE (deleted_at IS NULL);


--
-- Name: idx_comparison_items_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comparison_items_type ON public.comparison_items USING btree (type_id) WHERE (type_id IS NOT NULL);


--
-- Name: idx_comparison_profiles_one_default; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_comparison_profiles_one_default ON public.comparison_profiles USING btree (user_id) WHERE ((is_default = true) AND (user_id IS NOT NULL) AND (deleted_at IS NULL));


--
-- Name: idx_comparison_profiles_tenant_templates; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comparison_profiles_tenant_templates ON public.comparison_profiles USING btree (is_template) WHERE ((is_template = true) AND (deleted_at IS NULL));


--
-- Name: idx_comparison_profiles_tenant_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comparison_profiles_tenant_user ON public.comparison_profiles USING btree (user_id) WHERE ((user_id IS NOT NULL) AND (deleted_at IS NULL));


--
-- Name: idx_comparison_recommendations_content; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comparison_recommendations_content ON public.comparison_recommendations USING btree (content_id, profile_id, priority DESC);


--
-- Name: idx_comparison_recommendations_item; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comparison_recommendations_item ON public.comparison_recommendations USING btree (item_id);


--
-- Name: idx_comparison_score_factors_system; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comparison_score_factors_system ON public.comparison_score_factors USING btree (is_system) WHERE (is_system = true);


--
-- Name: idx_comparison_score_factors_tenant_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comparison_score_factors_tenant_type ON public.comparison_score_factors USING btree (comparison_type_id);


--
-- Name: idx_comparison_scores_calculated; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comparison_scores_calculated ON public.comparison_scores USING btree (content_id, calculated_at);


--
-- Name: idx_comparison_scores_content_profile; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comparison_scores_content_profile ON public.comparison_scores USING btree (content_id, profile_id, version);


--
-- Name: idx_comparison_scores_item; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comparison_scores_item ON public.comparison_scores USING btree (item_id);


--
-- Name: idx_comparison_type_access_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comparison_type_access_type ON public.comparison_type_access USING btree (type_id);


--
-- Name: idx_comparison_values_data_source; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comparison_values_data_source ON public.comparison_values USING btree (data_source_id);


--
-- Name: idx_comparison_values_dimension; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comparison_values_dimension ON public.comparison_values USING btree (dimension_id);


--
-- Name: idx_comparison_values_item; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comparison_values_item ON public.comparison_values USING btree (item_id);


--
-- Name: idx_consent_records_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_consent_records_tenant ON public.consent_records USING btree (consent_type);


--
-- Name: idx_consent_records_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_consent_records_user ON public.consent_records USING btree (user_id, consent_type);


--
-- Name: idx_content_campaign_items_campaign; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_content_campaign_items_campaign ON public.content_campaign_items USING btree (campaign_id, status, sort_order);


--
-- Name: idx_content_campaign_items_unique_persona; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_content_campaign_items_unique_persona ON public.content_campaign_items USING btree (campaign_id, persona_id) WHERE (status <> ALL (ARRAY['failed'::text, 'cancelled'::text]));


--
-- Name: idx_content_campaigns_tenant_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_content_campaigns_tenant_status ON public.content_campaigns USING btree (status, created_at DESC);


--
-- Name: idx_content_embeddings_entity; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_content_embeddings_entity ON public.content_embeddings USING btree (entity_type, entity_id);


--
-- Name: idx_content_embeddings_hnsw; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_content_embeddings_hnsw ON public.content_embeddings USING hnsw (embedding public.vector_cosine_ops);


--
-- Name: idx_content_embeddings_scope; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_content_embeddings_scope ON public.content_embeddings USING btree (visibility, space_id);


--
-- Name: idx_content_engagement_velocity; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_content_engagement_velocity ON public.content_engagement USING btree (velocity_score DESC);


--
-- Name: idx_content_featured_order; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_content_featured_order ON public.content_featured USING btree (featured_order);


--
-- Name: idx_content_imports_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_content_imports_tenant ON public.content_imports USING btree (status, created_at DESC);


--
-- Name: idx_content_imports_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_content_imports_user ON public.content_imports USING btree (user_id, status);


--
-- Name: idx_content_reviews_content; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_content_reviews_content ON public.content_reviews USING btree (content_id);


--
-- Name: idx_content_reviews_persona_payload; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_content_reviews_persona_payload ON public.content_reviews USING btree (source, status, created_at DESC) WHERE (source = 'persona_action'::text);


--
-- Name: idx_content_reviews_reviewer; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_content_reviews_reviewer ON public.content_reviews USING btree (reviewer_user_id);


--
-- Name: idx_content_reviews_source; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_content_reviews_source ON public.content_reviews USING btree (source, status, created_at DESC);


--
-- Name: idx_content_reviews_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_content_reviews_status ON public.content_reviews USING btree (status) WHERE (status = 'pending'::text);


--
-- Name: idx_content_revisions_content_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_content_revisions_content_id ON public.content_revisions USING btree (content_id);


--
-- Name: idx_content_scores_velocity; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_content_scores_velocity ON public.content_scores USING btree (velocity_score DESC);


--
-- Name: idx_content_shares_content; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_content_shares_content ON public.content_shares USING btree (content_id);


--
-- Name: idx_content_shares_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_content_shares_user ON public.content_shares USING btree (user_id);


--
-- Name: idx_content_spaces_space; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_content_spaces_space ON public.content_spaces USING btree (space_id);


--
-- Name: idx_content_tags_tag; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_content_tags_tag ON public.content_tags USING btree (tag_id);


--
-- Name: idx_content_thread_links_content; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_content_thread_links_content ON public.content_thread_links USING btree (content_id, created_at DESC);


--
-- Name: idx_content_thread_links_thread; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_content_thread_links_thread ON public.content_thread_links USING btree (thread_id);


--
-- Name: idx_content_views_tenant_content; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_content_views_tenant_content ON public.content_views USING btree (content_id);


--
-- Name: idx_content_views_tenant_space; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_content_views_tenant_space ON public.content_views USING btree (space_id);


--
-- Name: idx_contents_author_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contents_author_id ON public.contents USING btree (author_id);


--
-- Name: idx_contents_auto_hidden; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contents_auto_hidden ON public.contents USING btree (auto_hidden) WHERE (auto_hidden = true);


--
-- Name: idx_contents_content_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contents_content_type ON public.contents USING btree (content_type);


--
-- Name: idx_contents_foryou_window; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contents_foryou_window ON public.contents USING btree (status, visibility, created_at DESC);


--
-- Name: idx_contents_geo_space_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contents_geo_space_id ON public.contents USING btree (geo_space_id);


--
-- Name: idx_contents_guide_category; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contents_guide_category ON public.contents USING btree (guide_category) WHERE (guide_category IS NOT NULL);


--
-- Name: idx_contents_intel_record_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contents_intel_record_id ON public.contents USING btree (intel_record_id) WHERE (intel_record_id IS NOT NULL);


--
-- Name: idx_contents_intel_source_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contents_intel_source_id ON public.contents USING btree (intel_source_id) WHERE (intel_source_id IS NOT NULL);


--
-- Name: idx_contents_journey; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contents_journey ON public.contents USING btree (journey_stage, visa_route) WHERE ((deleted_at IS NULL) AND (status = 'published'::text) AND (visibility = 'public'::text));


--
-- Name: idx_contents_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contents_org ON public.contents USING btree (organization_id, status, published_at DESC) WHERE ((organization_id IS NOT NULL) AND (deleted_at IS NULL));


--
-- Name: idx_contents_publish_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contents_publish_at ON public.contents USING btree (publish_at) WHERE (publish_at IS NOT NULL);


--
-- Name: idx_contents_slug; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contents_slug ON public.contents USING btree (slug);


--
-- Name: idx_contents_slug_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contents_slug_trgm ON public.contents USING gin (slug public.gin_trgm_ops);


--
-- Name: idx_contents_soft_delete; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contents_soft_delete ON public.contents USING btree (id) WHERE (deleted_at IS NULL);


--
-- Name: idx_contents_source_target; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contents_source_target ON public.contents USING btree (source_country, target_country) WHERE ((deleted_at IS NULL) AND (status = 'published'::text) AND (visibility = 'public'::text));


--
-- Name: idx_contents_space_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contents_space_id ON public.contents USING btree (space_id);


--
-- Name: idx_contents_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contents_status ON public.contents USING btree (status);


--
-- Name: idx_contents_target_country; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contents_target_country ON public.contents USING btree (target_country) WHERE ((deleted_at IS NULL) AND (status = 'published'::text) AND (visibility = 'public'::text));


--
-- Name: idx_contents_tenant_content_hash; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_contents_tenant_content_hash ON public.contents USING btree (content_hash) WHERE (content_hash IS NOT NULL);


--
-- Name: idx_contents_tenant_slug; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_contents_tenant_slug ON public.contents USING btree (slug);


--
-- Name: idx_contents_title_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contents_title_trgm ON public.contents USING gin (title public.gin_trgm_ops);


--
-- Name: idx_contents_video_source; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contents_video_source ON public.contents USING btree (video_source_id) WHERE (video_source_id IS NOT NULL);


--
-- Name: idx_coupon_events_coupon; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_coupon_events_coupon ON public.coupon_events USING btree (coupon_id, event_type, created_at DESC);


--
-- Name: idx_coupons_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_coupons_active ON public.coupons USING btree (status, expires_at);


--
-- Name: idx_coupons_provider; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_coupons_provider ON public.coupons USING btree (provider_id, status);


--
-- Name: idx_course_access_grants_content; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_course_access_grants_content ON public.course_access_grants USING btree (content_id);


--
-- Name: idx_course_access_grants_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_course_access_grants_user ON public.course_access_grants USING btree (user_id);


--
-- Name: idx_credential_notes_pathway; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_credential_notes_pathway ON public.credential_community_notes USING btree (pathway_id, status, created_at);


--
-- Name: idx_credential_pathways_intel_record; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_credential_pathways_intel_record ON public.credential_pathways USING btree (intel_record_id) WHERE (intel_record_id IS NOT NULL);


--
-- Name: idx_credential_pathways_tenant_dest; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_credential_pathways_tenant_dest ON public.credential_pathways USING btree (destination_space_id, is_active);


--
-- Name: idx_credential_pathways_tenant_prof; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_credential_pathways_tenant_prof ON public.credential_pathways USING btree (profession_id, is_active);


--
-- Name: idx_credit_packs_tenant_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_credit_packs_tenant_active ON public.credit_packs USING btree (is_active, "position");


--
-- Name: idx_crs_draws_cat; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_crs_draws_cat ON public.crs_draws USING btree (category, draw_date DESC);


--
-- Name: idx_crs_factor_points_ruleset; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_crs_factor_points_ruleset ON public.crs_factor_points USING btree (ruleset_id, factor_group, sort_order);


--
-- Name: idx_crs_score_history_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_crs_score_history_user ON public.crs_score_history USING btree (user_id, created_at DESC) WHERE (deleted_at IS NULL);


--
-- Name: idx_custom_fields_group; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_custom_fields_group ON public.custom_fields USING btree (group_id);


--
-- Name: idx_cv_format_sessions_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cv_format_sessions_user ON public.cv_format_sessions USING btree (user_id, created_at);


--
-- Name: idx_data_export_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_data_export_user ON public.data_export_requests USING btree (user_id, status);


--
-- Name: idx_device_tokens_token; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_device_tokens_token ON public.device_tokens USING btree (token) WHERE (is_active = true);


--
-- Name: idx_device_tokens_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_device_tokens_user ON public.device_tokens USING btree (user_id, is_active);


--
-- Name: idx_digest_editions_tenant_space; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_digest_editions_tenant_space ON public.digest_editions USING btree (space_id, published_at DESC) WHERE (status = 'published'::text);


--
-- Name: idx_digest_editions_tenant_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_digest_editions_tenant_status ON public.digest_editions USING btree (status, published_at DESC);


--
-- Name: idx_discount_code_uses_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_discount_code_uses_code ON public.discount_code_uses USING btree (code_id);


--
-- Name: idx_discount_codes_valid; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_discount_codes_valid ON public.discount_codes USING btree (valid_from, valid_until);


--
-- Name: idx_dm_messages_conversation; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_dm_messages_conversation ON public.dm_messages USING btree (conversation_id, created_at DESC);


--
-- Name: idx_dm_messages_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_dm_messages_deleted ON public.dm_messages USING btree (deleted_at);


--
-- Name: idx_dm_messages_sender; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_dm_messages_sender ON public.dm_messages USING btree (sender_id);


--
-- Name: idx_dm_requests_recipient; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_dm_requests_recipient ON public.dm_requests USING btree (recipient_id, status, created_at DESC);


--
-- Name: idx_dm_requests_stale; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_dm_requests_stale ON public.dm_requests USING btree (status, updated_at);


--
-- Name: idx_doc_pages_audience; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_doc_pages_audience ON public.doc_pages USING btree (audience);


--
-- Name: idx_doc_pages_parent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_doc_pages_parent ON public.doc_pages USING btree (parent_id);


--
-- Name: idx_eca_guides_dest; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_eca_guides_dest ON public.eca_guides USING btree (destination_space_id, is_active);


--
-- Name: idx_eca_guides_provider; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_eca_guides_provider ON public.eca_guides USING btree (provider_id, is_active);


--
-- Name: idx_eca_updates_guide; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_eca_updates_guide ON public.eca_community_updates USING btree (guide_id, status, created_at) WHERE (deleted_at IS NULL);


--
-- Name: idx_email_verifications_hash; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_verifications_hash ON public.email_verifications USING btree (token_hash);


--
-- Name: idx_enrollments_tenant_track; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_enrollments_tenant_track ON public.enrollments USING btree (track_id);


--
-- Name: idx_enrollments_tenant_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_enrollments_tenant_user ON public.enrollments USING btree (user_id);


--
-- Name: idx_external_data_sources_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_external_data_sources_status ON public.external_data_sources USING btree (status);


--
-- Name: idx_external_data_sources_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_external_data_sources_type ON public.external_data_sources USING btree (source_type);


--
-- Name: idx_feeds_space; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_feeds_space ON public.feeds USING btree (space_id, created_at DESC) WHERE (space_id IS NOT NULL);


--
-- Name: idx_feeds_user_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_feeds_user_created ON public.feeds USING btree (user_id, created_at DESC);


--
-- Name: idx_feeds_user_score; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_feeds_user_score ON public.feeds USING btree (user_id, score DESC);


--
-- Name: idx_follows_entity; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_follows_entity ON public.follows USING btree (entity_type, entity_id);


--
-- Name: idx_follows_follower; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_follows_follower ON public.follows USING btree (follower_id, entity_type);


--
-- Name: idx_form_steps_form; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_form_steps_form ON public.form_steps USING btree (form_id);


--
-- Name: idx_form_submissions_form; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_form_submissions_form ON public.form_submissions USING btree (form_id);


--
-- Name: idx_forms_space; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_forms_space ON public.forms USING btree (space_id) WHERE (space_id IS NOT NULL);


--
-- Name: idx_forum_bookmarks_entity; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_forum_bookmarks_entity ON public.forum_bookmarks USING btree (entity_type, entity_id);


--
-- Name: idx_forum_bookmarks_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_forum_bookmarks_user ON public.forum_bookmarks USING btree (user_id);


--
-- Name: idx_forum_poll_options_poll; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_forum_poll_options_poll ON public.forum_poll_options USING btree (poll_id);


--
-- Name: idx_forum_poll_votes_poll; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_forum_poll_votes_poll ON public.forum_poll_votes USING btree (poll_id);


--
-- Name: idx_forum_poll_votes_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_forum_poll_votes_user ON public.forum_poll_votes USING btree (user_id);


--
-- Name: idx_forum_polls_thread; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_forum_polls_thread ON public.forum_polls USING btree (thread_id);


--
-- Name: idx_forum_replies_ama_pending; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_forum_replies_ama_pending ON public.forum_replies USING btree (thread_id) WHERE ((ama_pending = true) AND (deleted_at IS NULL));


--
-- Name: idx_forum_replies_parent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_forum_replies_parent ON public.forum_replies USING btree (parent_reply_id) WHERE (deleted_at IS NULL);


--
-- Name: idx_forum_replies_thread; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_forum_replies_thread ON public.forum_replies USING btree (thread_id) WHERE (deleted_at IS NULL);


--
-- Name: idx_forum_thread_tags_tag; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_forum_thread_tags_tag ON public.forum_thread_tags USING btree (tag_id);


--
-- Name: idx_forum_threads_ama; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_forum_threads_ama ON public.forum_threads USING btree (created_at DESC) WHERE (ama_config IS NOT NULL);


--
-- Name: idx_forum_threads_author; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_forum_threads_author ON public.forum_threads USING btree (author_user_id) WHERE (deleted_at IS NULL);


--
-- Name: idx_forum_threads_slug; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_forum_threads_slug ON public.forum_threads USING btree (slug) WHERE (deleted_at IS NULL);


--
-- Name: idx_forum_threads_space; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_forum_threads_space ON public.forum_threads USING btree (space_id) WHERE (deleted_at IS NULL);


--
-- Name: idx_forum_threads_title_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_forum_threads_title_trgm ON public.forum_threads USING gin (title public.gin_trgm_ops);


--
-- Name: idx_forum_votes_entity; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_forum_votes_entity ON public.forum_votes USING btree (entity_type, entity_id);


--
-- Name: idx_forum_votes_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_forum_votes_user ON public.forum_votes USING btree (user_id);


--
-- Name: idx_fst_form; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fst_form ON public.form_submission_triggers USING btree (form_id);


--
-- Name: idx_housing_guides_intel_record; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_housing_guides_intel_record ON public.housing_guides USING btree (intel_record_id) WHERE (intel_record_id IS NOT NULL);


--
-- Name: idx_housing_guides_tenant_dest; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_housing_guides_tenant_dest ON public.housing_guides USING btree (destination_space_id, is_active);


--
-- Name: idx_instructor_payouts_instructor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_instructor_payouts_instructor ON public.instructor_payouts USING btree (instructor_id, period_start DESC);


--
-- Name: idx_instructor_payouts_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_instructor_payouts_status ON public.instructor_payouts USING btree (status) WHERE (status = ANY (ARRAY['pending'::text, 'processing'::text]));


--
-- Name: idx_instructor_profiles_tenant_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_instructor_profiles_tenant_status ON public.instructor_profiles USING btree (status);


--
-- Name: idx_instructor_profiles_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_instructor_profiles_user ON public.instructor_profiles USING btree (user_id);


--
-- Name: idx_intel_channels_due; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_intel_channels_due ON public.intel_source_channels USING btree (enabled, crawl_frequency_mins) WHERE enabled;


--
-- Name: idx_intel_channels_source; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_intel_channels_source ON public.intel_source_channels USING btree (source_id);


--
-- Name: idx_intel_inbound_channel; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_intel_inbound_channel ON public.intel_inbound_events USING btree (channel_id, received_at DESC);


--
-- Name: idx_intel_jobs_source_url; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_intel_jobs_source_url ON public.intel_jobs USING btree (source_url);


--
-- Name: idx_intel_jobs_tenant_country; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_intel_jobs_tenant_country ON public.intel_jobs USING btree (country);


--
-- Name: idx_intel_jobs_tenant_deadline; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_intel_jobs_tenant_deadline ON public.intel_jobs USING btree (deadline) WHERE (deadline IS NOT NULL);


--
-- Name: idx_intel_jobs_tenant_job_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_intel_jobs_tenant_job_type ON public.intel_jobs USING btree (job_type) WHERE (job_type IS NOT NULL);


--
-- Name: idx_intel_jobs_tenant_space; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_intel_jobs_tenant_space ON public.intel_jobs USING btree (country_space_id) WHERE (country_space_id IS NOT NULL);


--
-- Name: idx_intel_raw_docs_tenant_hash; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_intel_raw_docs_tenant_hash ON public.intel_raw_documents USING btree (source_id, content_hash);


--
-- Name: idx_intel_raw_docs_tenant_source; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_intel_raw_docs_tenant_source ON public.intel_raw_documents USING btree (source_id, retrieved_at DESC);


--
-- Name: idx_intel_raw_documents_parent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_intel_raw_documents_parent ON public.intel_raw_documents USING btree (parent_document_id) WHERE (parent_document_id IS NOT NULL);


--
-- Name: idx_intel_records_tenant_entity_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_intel_records_tenant_entity_status ON public.intel_records USING btree (entity_type, status);


--
-- Name: idx_intel_records_tenant_geo; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_intel_records_tenant_geo ON public.intel_records USING gin (geo_space_ids);


--
-- Name: idx_intel_records_tenant_promoted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_intel_records_tenant_promoted ON public.intel_records USING btree (promoted_record_id) WHERE (promoted_record_id IS NOT NULL);


--
-- Name: idx_intel_review_log_tenant_record; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_intel_review_log_tenant_record ON public.intel_review_log USING btree (intel_record_id, created_at DESC);


--
-- Name: idx_intel_sources_category_kind; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_intel_sources_category_kind ON public.intel_sources USING btree (category, source_kind) WHERE ((enabled = true) AND (status = 'active'::text));


--
-- Name: idx_intel_sources_tenant_category; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_intel_sources_tenant_category ON public.intel_sources USING btree (category, trust_level);


--
-- Name: idx_intel_sources_tenant_enabled_freq; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_intel_sources_tenant_enabled_freq ON public.intel_sources USING btree (enabled, crawl_frequency_mins) WHERE enabled;


--
-- Name: idx_intel_sources_tenant_slug; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_intel_sources_tenant_slug ON public.intel_sources USING btree (slug) WHERE (slug IS NOT NULL);


--
-- Name: idx_intel_sources_tenant_url; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_intel_sources_tenant_url ON public.intel_sources USING btree (source_url);


--
-- Name: idx_ip_blocks_cidr; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ip_blocks_cidr ON public.ip_blocks USING gist (cidr inet_ops);


--
-- Name: idx_job_apps_applicant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_job_apps_applicant ON public.job_applications USING btree (applicant_id);


--
-- Name: idx_job_apps_job; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_job_apps_job ON public.job_applications USING btree (job_id, status);


--
-- Name: idx_job_apps_tenant_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_job_apps_tenant_status ON public.job_applications USING btree (status);


--
-- Name: idx_jwt_signing_keys_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_jwt_signing_keys_active ON public.jwt_signing_keys USING btree (active) WHERE (active = true);


--
-- Name: idx_known_bots_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_known_bots_type ON public.known_bots USING btree (bot_type);


--
-- Name: idx_language_score_stories_test; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_language_score_stories_test ON public.language_score_stories USING btree (test_id, status);


--
-- Name: idx_language_test_req_tenant_dest; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_language_test_req_tenant_dest ON public.language_test_requirements USING btree (destination_space_id, is_active);


--
-- Name: idx_language_test_requirements_intel_record; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_language_test_requirements_intel_record ON public.language_test_requirements USING btree (intel_record_id) WHERE (intel_record_id IS NOT NULL);


--
-- Name: idx_language_test_resources_test; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_language_test_resources_test ON public.language_test_resources USING btree (test_id);


--
-- Name: idx_learning_tracks_geo_space; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_learning_tracks_geo_space ON public.learning_tracks USING btree (geo_space_id) WHERE (geo_space_id IS NOT NULL);


--
-- Name: idx_learning_tracks_instructor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_learning_tracks_instructor ON public.learning_tracks USING btree (instructor_id);


--
-- Name: idx_learning_tracks_submission_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_learning_tracks_submission_status ON public.learning_tracks USING btree (instructor_id, university_submission_status) WHERE (instructor_id IS NOT NULL);


--
-- Name: idx_learning_tracks_university; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_learning_tracks_university ON public.learning_tracks USING btree (is_published_to_university, category) WHERE (is_published_to_university = true);


--
-- Name: idx_lesson_quizzes_tenant_lesson; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lesson_quizzes_tenant_lesson ON public.lesson_quizzes USING btree (lesson_id);


--
-- Name: idx_media_quarantined; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_media_quarantined ON public.media USING btree (quarantined_at DESC) WHERE ((quarantine_reason IS NOT NULL) AND (deleted_at IS NULL));


--
-- Name: idx_media_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_media_tenant ON public.media USING btree (created_at DESC) WHERE (deleted_at IS NULL);


--
-- Name: idx_media_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_media_type ON public.media USING btree (mime_type, confirmed, deleted_at);


--
-- Name: idx_media_uploader; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_media_uploader ON public.media USING btree (uploader_user_id, created_at DESC);


--
-- Name: idx_media_user_group; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_media_user_group ON public.media USING btree (user_group_id) WHERE (deleted_at IS NULL);


--
-- Name: idx_mentor_reviews_reviewer; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mentor_reviews_reviewer ON public.mentor_reviews USING btree (reviewer_id);


--
-- Name: idx_mentor_reviews_session; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mentor_reviews_session ON public.mentor_reviews USING btree (session_id);


--
-- Name: idx_mentor_sessions_mentee; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mentor_sessions_mentee ON public.mentor_sessions USING btree (mentee_id);


--
-- Name: idx_mentor_sessions_mentor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mentor_sessions_mentor ON public.mentor_sessions USING btree (mentor_id);


--
-- Name: idx_mentor_sessions_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mentor_sessions_status ON public.mentor_sessions USING btree (status);


--
-- Name: idx_mentorship_conn_mentee_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mentorship_conn_mentee_status ON public.mentorship_connections USING btree (mentee_id, status);


--
-- Name: idx_mentorship_conn_mentor_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mentorship_conn_mentor_status ON public.mentorship_connections USING btree (mentor_id, status);


--
-- Name: idx_mentorship_conn_tenant_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mentorship_conn_tenant_status ON public.mentorship_connections USING btree (status);


--
-- Name: idx_mentorship_matches_mentee; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mentorship_matches_mentee ON public.mentorship_matches USING btree (mentee_id);


--
-- Name: idx_mentorship_matches_mentor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mentorship_matches_mentor ON public.mentorship_matches USING btree (mentor_id);


--
-- Name: idx_mentorship_matches_program; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mentorship_matches_program ON public.mentorship_matches USING btree (program_id);


--
-- Name: idx_mentorship_matches_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mentorship_matches_tenant ON public.mentorship_matches USING btree (status);


--
-- Name: idx_mentorship_profiles_available; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mentorship_profiles_available ON public.mentorship_profiles USING btree (is_available);


--
-- Name: idx_mentorship_profiles_mentor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mentorship_profiles_mentor ON public.mentorship_profiles USING btree (is_mentor);


--
-- Name: idx_mentorship_profiles_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mentorship_profiles_user ON public.mentorship_profiles USING btree (user_id);


--
-- Name: idx_mentorship_programs_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mentorship_programs_tenant ON public.mentorship_programs USING btree (is_active);


--
-- Name: idx_menu_items_menu; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_menu_items_menu ON public.menu_items USING btree (menu_id, sort_order);


--
-- Name: idx_menu_items_parent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_menu_items_parent ON public.menu_items USING btree (parent_id);


--
-- Name: idx_moderation_alerts_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_moderation_alerts_status ON public.moderation_alerts USING btree (status);


--
-- Name: idx_moderation_appeals_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_moderation_appeals_status ON public.moderation_appeals USING btree (status, created_at DESC);


--
-- Name: idx_monitored_embassies_tenant_dest; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_monitored_embassies_tenant_dest ON public.monitored_embassies USING btree (destination_space_id, is_active);


--
-- Name: idx_mutual_aid_posts_tenant_expires; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mutual_aid_posts_tenant_expires ON public.mutual_aid_posts USING btree (expires_at) WHERE ((status = 'active'::text) AND (deleted_at IS NULL));


--
-- Name: idx_mutual_aid_posts_tenant_status_dest; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mutual_aid_posts_tenant_status_dest ON public.mutual_aid_posts USING btree (status, destination_space_id) WHERE (deleted_at IS NULL);


--
-- Name: idx_mv_daily_content_stats_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_mv_daily_content_stats_unique ON public.mv_daily_content_stats USING btree (day, content_id);


--
-- Name: idx_mv_daily_space_stats_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_mv_daily_space_stats_unique ON public.mv_daily_space_stats USING btree (day, space_id);


--
-- Name: idx_mv_revenue_streams; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_mv_revenue_streams ON public.mv_revenue_streams USING btree (month);


--
-- Name: idx_mv_top_content_30d; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_mv_top_content_30d ON public.mv_top_content_30d USING btree (content_id);


--
-- Name: idx_mv_top_content_7d; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_mv_top_content_7d ON public.mv_top_content_7d USING btree (content_id);


--
-- Name: idx_mv_top_content_90d; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_mv_top_content_90d ON public.mv_top_content_90d USING btree (content_id);


--
-- Name: idx_mv_user_engagement_stats_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_mv_user_engagement_stats_unique ON public.mv_user_engagement_stats USING btree (day, user_id);


--
-- Name: idx_newsletter_campaigns_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_newsletter_campaigns_tenant ON public.newsletter_campaigns USING btree (status);


--
-- Name: idx_newsletter_send_state_campaign; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_newsletter_send_state_campaign ON public.newsletter_send_state USING btree (campaign_id, status);


--
-- Name: idx_newsletter_sub_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_newsletter_sub_status ON public.newsletter_subscribers USING btree (status);


--
-- Name: idx_newsletter_sub_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_newsletter_sub_user ON public.newsletter_subscribers USING btree (user_id) WHERE (user_id IS NOT NULL);


--
-- Name: idx_newsletter_subscribers_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_newsletter_subscribers_tenant ON public.newsletter_subscribers USING btree (status);


--
-- Name: idx_notification_batch_windows_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_notification_batch_windows_tenant ON public.notification_batch_windows USING btree (recipient_id, dispatched, window_close_at);


--
-- Name: idx_notifications_recipient; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_notifications_recipient ON public.notifications USING btree (recipient_id, read_at, created_at DESC);


--
-- Name: idx_notifications_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_notifications_type ON public.notifications USING btree (notification_type);


--
-- Name: idx_oauth_accounts_provider; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_oauth_accounts_provider ON public.oauth_accounts USING btree (provider, provider_user_id);


--
-- Name: idx_oauth_accounts_provider_user_id_global; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_oauth_accounts_provider_user_id_global ON public.oauth_accounts USING btree (provider, provider_user_id);


--
-- Name: idx_oauth_accounts_user_provider_global; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_oauth_accounts_user_provider_global ON public.oauth_accounts USING btree (user_id, provider);


--
-- Name: idx_org_claims_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_org_claims_tenant ON public.ownership_claims USING btree (status, created_at DESC);


--
-- Name: idx_organization_boosts_campaign; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_organization_boosts_campaign ON public.organization_boosts USING btree (campaign_id);


--
-- Name: idx_organization_boosts_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_organization_boosts_org ON public.organization_boosts USING btree (organization_id);


--
-- Name: idx_organizations_name_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_organizations_name_trgm ON public.organizations USING gin (name public.gin_trgm_ops);


--
-- Name: idx_organizations_slug_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_organizations_slug_trgm ON public.organizations USING gin (slug public.gin_trgm_ops);


--
-- Name: idx_organizations_source_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_organizations_source_id ON public.organizations USING btree (source_id) WHERE (source_id IS NOT NULL);


--
-- Name: idx_orgs_geo; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_orgs_geo ON public.organizations USING btree (geo_space_id) WHERE (geo_space_id IS NOT NULL);


--
-- Name: idx_orgs_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_orgs_tenant ON public.organizations USING btree (status, created_at DESC);


--
-- Name: idx_ownership_claims_pending_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_ownership_claims_pending_unique ON public.ownership_claims USING btree (claimant_user_id, organization_id) WHERE (status = 'pending'::text);


--
-- Name: idx_pagebuilder_templates_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pagebuilder_templates_active ON public.pagebuilder_templates USING btree (is_active);


--
-- Name: idx_pages_page_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pages_page_type ON public.pages USING btree (page_type);


--
-- Name: idx_pages_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pages_status ON public.pages USING btree (status);


--
-- Name: idx_password_history_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_password_history_user ON public.password_history USING btree (user_id, created_at DESC);


--
-- Name: idx_password_reset_tokens_hash; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_password_reset_tokens_hash ON public.password_reset_tokens USING btree (token_hash);


--
-- Name: idx_password_reset_tokens_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_password_reset_tokens_user ON public.password_reset_tokens USING btree (user_id);


--
-- Name: idx_payment_refunds_payment; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_refunds_payment ON public.payment_refunds USING btree (payment_id);


--
-- Name: idx_payment_webhook_events_provider; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_webhook_events_provider ON public.payment_webhook_events USING btree (provider, event_id);


--
-- Name: idx_payments_provider_ref; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payments_provider_ref ON public.payments USING btree (provider_ref);


--
-- Name: idx_payments_purpose; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payments_purpose ON public.payments USING btree (purpose_type, purpose_id);


--
-- Name: idx_payments_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payments_status ON public.payments USING btree (status);


--
-- Name: idx_payments_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payments_user ON public.payments USING btree (user_id);


--
-- Name: idx_payout_providers_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payout_providers_active ON public.payout_providers USING btree (is_active);


--
-- Name: idx_pe_entity; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pe_entity ON public.platform_events USING btree (entity_type, entity_id, event_type) WHERE (entity_id IS NOT NULL);


--
-- Name: idx_pe_synthetic; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pe_synthetic ON public.platform_events USING btree (event_type, created_at DESC) WHERE (is_synthetic = true);


--
-- Name: idx_pe_synthetic_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pe_synthetic_user ON public.platform_events USING btree (user_id, created_at DESC) WHERE (is_synthetic = true);


--
-- Name: idx_pe_tenant_type_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pe_tenant_type_created ON public.platform_events USING btree (event_type, created_at DESC);


--
-- Name: idx_pe_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pe_user ON public.platform_events USING btree (user_id, event_type, created_at DESC) WHERE (user_id IS NOT NULL);


--
-- Name: idx_planning_group_checkins_weekly; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_planning_group_checkins_weekly ON public.planning_group_checkins USING btree (group_id, user_id, public.week_start_utc(created_at));


--
-- Name: idx_planning_group_milestones_group_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_planning_group_milestones_group_user ON public.planning_group_milestones USING btree (group_id, user_id);


--
-- Name: idx_planning_groups_tenant_dest; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_planning_groups_tenant_dest ON public.planning_groups USING btree (destination_space_id) WHERE (deleted_at IS NULL);


--
-- Name: idx_platform_data_exports_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_platform_data_exports_status ON public.platform_data_exports USING btree (status);


--
-- Name: idx_platform_data_exports_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_platform_data_exports_tenant ON public.platform_data_exports USING btree (created_at DESC);


--
-- Name: idx_platform_data_imports_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_platform_data_imports_status ON public.platform_data_imports USING btree (status);


--
-- Name: idx_platform_data_imports_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_platform_data_imports_tenant ON public.platform_data_imports USING btree (created_at DESC);


--
-- Name: idx_pof_requirements_dest; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pof_requirements_dest ON public.pof_requirements USING btree (destination_space_id, is_active);


--
-- Name: idx_push_subscriptions_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_push_subscriptions_user ON public.push_subscriptions USING btree (user_id);


--
-- Name: idx_push_tokens_stale; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_push_tokens_stale ON public.push_tokens USING btree (last_used_at, created_at);


--
-- Name: idx_push_tokens_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_push_tokens_user ON public.push_tokens USING btree (user_id);


--
-- Name: idx_rate_alerts_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_rate_alerts_active ON public.remittance_rate_alerts USING btree (send_currency, receive_currency, is_active) WHERE (is_active = true);


--
-- Name: idx_reactions_entity; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reactions_entity ON public.reactions USING btree (entity_type, entity_id);


--
-- Name: idx_reactions_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reactions_user ON public.reactions USING btree (user_id);


--
-- Name: idx_redirects_source; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_redirects_source ON public.redirects USING btree (source_path) WHERE (is_active = true);


--
-- Name: idx_redirects_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_redirects_status ON public.redirects USING btree (is_active);


--
-- Name: idx_referral_codes_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_referral_codes_code ON public.referral_codes USING btree (code);


--
-- Name: idx_referral_codes_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_referral_codes_tenant ON public.referral_codes USING btree (user_id);


--
-- Name: idx_referrals_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_referrals_code ON public.referrals USING btree (referral_code);


--
-- Name: idx_referrals_referrer; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_referrals_referrer ON public.referrals USING btree (referrer_id);


--
-- Name: idx_referrals_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_referrals_status ON public.referrals USING btree (status, created_at DESC);


--
-- Name: idx_referrals_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_referrals_tenant ON public.referrals USING btree (status);


--
-- Name: idx_refresh_tokens_hash; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_refresh_tokens_hash ON public.refresh_tokens USING btree (token_hash);


--
-- Name: idx_refresh_tokens_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_refresh_tokens_user_id ON public.refresh_tokens USING btree (user_id);


--
-- Name: idx_remittance_providers_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_remittance_providers_tenant ON public.remittance_providers USING btree (is_active, display_order);


--
-- Name: idx_remittance_rates_corridor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_remittance_rates_corridor ON public.remittance_rates USING btree (send_currency, receive_currency, provider_id, fetched_at DESC);


--
-- Name: idx_remittance_rates_history; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_remittance_rates_history ON public.remittance_rates USING btree (send_currency, receive_currency, fetched_at DESC);


--
-- Name: idx_remittance_rates_intel_record; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_remittance_rates_intel_record ON public.remittance_rates USING btree (intel_record_id) WHERE (intel_record_id IS NOT NULL);


--
-- Name: idx_reports_entity; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reports_entity ON public.content_reports USING btree (entity_type, entity_id, status);


--
-- Name: idx_reports_reporter; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reports_reporter ON public.content_reports USING btree (reporter_user_id);


--
-- Name: idx_reports_reporter_entity_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_reports_reporter_entity_unique ON public.content_reports USING btree (reporter_user_id, entity_type, entity_id) WHERE (reporter_user_id IS NOT NULL);


--
-- Name: idx_reports_reviewed_by; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reports_reviewed_by ON public.content_reports USING btree (reviewed_by) WHERE (reviewed_by IS NOT NULL);


--
-- Name: idx_reports_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reports_status ON public.content_reports USING btree (status, created_at DESC);


--
-- Name: idx_reports_status_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reports_status_created ON public.content_reports USING btree (status, created_at DESC);


--
-- Name: idx_revenue_splits_instructor_open; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_revenue_splits_instructor_open ON public.revenue_splits USING btree (instructor_id) WHERE (payout_id IS NULL);


--
-- Name: idx_revenue_splits_payout; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_revenue_splits_payout ON public.revenue_splits USING btree (payout_id);


--
-- Name: idx_review_aggregates_entity; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_review_aggregates_entity ON public.review_aggregates USING btree (entity_type, entity_id);


--
-- Name: idx_reviews_entity; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reviews_entity ON public.reviews USING btree (entity_type, entity_id);


--
-- Name: idx_reviews_reviewer; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reviews_reviewer ON public.reviews USING btree (reviewer_user_id);


--
-- Name: idx_reviews_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reviews_status ON public.reviews USING btree (status) WHERE (status = 'flagged'::text);


--
-- Name: idx_saved_scholarships_deadline; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_saved_scholarships_deadline ON public.saved_scholarships USING btree (guide_id);


--
-- Name: idx_saved_scholarships_remind; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_saved_scholarships_remind ON public.saved_scholarships USING btree (last_reminded_at) WHERE (last_reminded_at IS NULL);


--
-- Name: idx_saved_scholarships_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_saved_scholarships_user ON public.saved_scholarships USING btree (user_id, status);


--
-- Name: idx_scam_reports_tenant_dest_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_scam_reports_tenant_dest_status ON public.scam_reports USING btree (destination_space_id, status);


--
-- Name: idx_scam_reports_tenant_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_scam_reports_tenant_status ON public.scam_reports USING btree (status);


--
-- Name: idx_scan_results_content; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_scan_results_content ON public.content_affiliate_scan_results USING btree (content_id);


--
-- Name: idx_scan_results_domain; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_scan_results_domain ON public.content_affiliate_scan_results USING btree (domain);


--
-- Name: idx_scan_results_unreviewed; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_scan_results_unreviewed ON public.content_affiliate_scan_results USING btree (reviewed_at) WHERE (reviewed_at IS NULL);


--
-- Name: idx_scholarships_guide; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_scholarships_guide ON public.scholarships USING btree (guide_id) WHERE (guide_id IS NOT NULL);


--
-- Name: idx_scholarships_tenant_country_deadline; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_scholarships_tenant_country_deadline ON public.scholarships USING btree (country, deadline);


--
-- Name: idx_scholarships_tenant_eligible; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_scholarships_tenant_eligible ON public.scholarships USING gin (eligible_countries);


--
-- Name: idx_scholarships_tenant_fields; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_scholarships_tenant_fields ON public.scholarships USING gin (fields_of_study);


--
-- Name: idx_scholarships_tenant_intel; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_scholarships_tenant_intel ON public.scholarships USING btree (intel_record_id) WHERE (intel_record_id IS NOT NULL);


--
-- Name: idx_scholarships_tenant_space; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_scholarships_tenant_space ON public.scholarships USING btree (country_space_id);


--
-- Name: idx_scholarships_tenant_status_fresh; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_scholarships_tenant_status_fresh ON public.scholarships USING btree (verification_status, freshness_score);


--
-- Name: idx_search_analytics_query; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_search_analytics_query ON public.search_analytics USING btree (query, created_at DESC);


--
-- Name: idx_search_analytics_zero; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_search_analytics_zero ON public.search_analytics USING btree (results_count, created_at DESC) WHERE (results_count = 0);


--
-- Name: idx_security_scan_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_security_scan_tenant ON public.security_scan_results USING btree (severity, created_at DESC);


--
-- Name: idx_service_listing_meta_tenant_dest_cat; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_service_listing_meta_tenant_dest_cat ON public.service_listing_meta USING btree (destination_space_id, service_category);


--
-- Name: idx_service_listing_meta_tenant_verified; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_service_listing_meta_tenant_verified ON public.service_listing_meta USING btree (is_verified) WHERE (is_verified = true);


--
-- Name: idx_share_platforms_tenant_enabled; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_share_platforms_tenant_enabled ON public.share_platforms USING btree (enabled);


--
-- Name: idx_sidebar_widgets_sidebar; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sidebar_widgets_sidebar ON public.sidebar_widgets USING btree (sidebar_id, sort_order);


--
-- Name: idx_sidebars_resolve; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sidebars_resolve ON public.sidebars USING btree (app, "position", status);


--
-- Name: idx_slug_history_entity; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_slug_history_entity ON public.slug_history USING btree (entity_type, entity_id);


--
-- Name: idx_slug_history_lookup; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_slug_history_lookup ON public.slug_history USING btree (entity_type, old_slug);


--
-- Name: idx_sop_drafts_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sop_drafts_user ON public.sop_drafts USING btree (user_id, created_at DESC) WHERE (deleted_at IS NULL);


--
-- Name: idx_sop_samples_dest; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sop_samples_dest ON public.sop_samples USING btree (destination_space_id, status);


--
-- Name: idx_sop_templates_dest; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sop_templates_dest ON public.sop_templates USING btree (destination_space_id, is_active);


--
-- Name: idx_source_collection_items_collection; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_source_collection_items_collection ON public.source_collection_items USING btree (collection_id, "position");


--
-- Name: idx_source_collections_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_source_collections_tenant ON public.source_collections USING btree (collection_type);


--
-- Name: idx_space_members_space; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_space_members_space ON public.space_members USING btree (space_id);


--
-- Name: idx_space_members_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_space_members_user ON public.space_members USING btree (user_id);


--
-- Name: idx_space_moderators_space; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_space_moderators_space ON public.space_moderators USING btree (space_id);


--
-- Name: idx_space_moderators_space_user_global; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_space_moderators_space_user_global ON public.space_moderators USING btree (space_id, user_id);


--
-- Name: idx_space_moderators_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_space_moderators_user ON public.space_moderators USING btree (user_id);


--
-- Name: idx_space_mutes_space; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_space_mutes_space ON public.space_mutes USING btree (space_id);


--
-- Name: idx_space_mutes_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_space_mutes_user ON public.space_mutes USING btree (user_id);


--
-- Name: idx_space_parent_spaces_parent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_space_parent_spaces_parent ON public.space_parent_spaces USING btree (parent_space_id);


--
-- Name: idx_space_permission_presets_name_global; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_space_permission_presets_name_global ON public.space_permission_presets USING btree (name);


--
-- Name: idx_space_pinned_posts_space; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_space_pinned_posts_space ON public.space_pinned_posts USING btree (space_id);


--
-- Name: idx_space_rules_space; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_space_rules_space ON public.space_rules USING btree (space_id);


--
-- Name: idx_space_types_slug_global; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_space_types_slug_global ON public.space_types USING btree (slug);


--
-- Name: idx_spaces_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_spaces_active ON public.spaces USING btree (deleted_at) WHERE (deleted_at IS NULL);


--
-- Name: idx_spaces_creator; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_spaces_creator ON public.spaces USING btree (creator_user_id) WHERE (creator_user_id IS NOT NULL);


--
-- Name: idx_spaces_geo; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_spaces_geo ON public.spaces USING btree (geo_code) WHERE (geo_code IS NOT NULL);


--
-- Name: idx_spaces_keywords; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_spaces_keywords ON public.spaces USING gin (keywords);


--
-- Name: idx_spaces_level; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_spaces_level ON public.spaces USING btree (level);


--
-- Name: idx_spaces_name_lower; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_spaces_name_lower ON public.spaces USING btree (lower(name)) WHERE ((deleted_at IS NULL) AND (visibility = 'public'::text) AND (status = 'active'::text));


--
-- Name: idx_spaces_parent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_spaces_parent ON public.spaces USING btree (parent_id);


--
-- Name: idx_spaces_region; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_spaces_region ON public.spaces USING btree (region) WHERE (level = 'country'::text);


--
-- Name: idx_spaces_slug_global; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_spaces_slug_global ON public.spaces USING btree (slug);


--
-- Name: idx_spaces_source_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_spaces_source_id ON public.spaces USING btree (source_id) WHERE (source_id IS NOT NULL);


--
-- Name: idx_spaces_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_spaces_type ON public.spaces USING btree (space_type_id);


--
-- Name: idx_story_meta_content; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_story_meta_content ON public.story_meta USING btree (content_id);


--
-- Name: idx_story_meta_tenant_dest; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_story_meta_tenant_dest ON public.story_meta USING btree (destination_space_id);


--
-- Name: idx_story_meta_tenant_origin; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_story_meta_tenant_origin ON public.story_meta USING btree (origin_space_id);


--
-- Name: idx_story_meta_tenant_visa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_story_meta_tenant_visa ON public.story_meta USING btree (visa_category) WHERE (visa_category IS NOT NULL);


--
-- Name: idx_story_meta_verified; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_story_meta_verified ON public.story_meta USING btree (verified) WHERE (verified = true);


--
-- Name: idx_studio_jobs_intel_record; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_studio_jobs_intel_record ON public.studio_jobs USING btree (intel_record_id) WHERE (intel_record_id IS NOT NULL);


--
-- Name: idx_studio_jobs_persona; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_studio_jobs_persona ON public.studio_jobs USING btree (persona_id, status);


--
-- Name: idx_studio_jobs_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_studio_jobs_status ON public.studio_jobs USING btree (status, created_at DESC);


--
-- Name: idx_studio_jobs_voice_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_studio_jobs_voice_id ON public.studio_jobs USING btree (voice_id) WHERE (voice_id IS NOT NULL);


--
-- Name: idx_subscription_plan_tracks_plan; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_subscription_plan_tracks_plan ON public.subscription_plan_tracks USING btree (plan_id);


--
-- Name: idx_subscription_plan_tracks_track; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_subscription_plan_tracks_track ON public.subscription_plan_tracks USING btree (track_id);


--
-- Name: idx_subscription_plans_tenant_slug; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_subscription_plans_tenant_slug ON public.subscription_plans USING btree (slug);


--
-- Name: idx_subscription_plans_tenant_sort; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_subscription_plans_tenant_sort ON public.subscription_plans USING btree (sort_order);


--
-- Name: idx_subscription_plans_tenant_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_subscription_plans_tenant_status ON public.subscription_plans USING btree (status);


--
-- Name: idx_subscriptions_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_subscriptions_status ON public.subscriptions USING btree (status);


--
-- Name: idx_subscriptions_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_subscriptions_user ON public.subscriptions USING btree (user_id);


--
-- Name: idx_system_settings_is_public; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_system_settings_is_public ON public.system_settings USING btree (is_public) WHERE (is_public = true);


--
-- Name: idx_system_settings_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_system_settings_key ON public.system_settings USING btree (key);


--
-- Name: idx_system_settings_tenant_key; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_system_settings_tenant_key ON public.system_settings USING btree (key);


--
-- Name: idx_tag_moderators_tag; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tag_moderators_tag ON public.tag_moderators USING btree (tag_id);


--
-- Name: idx_tag_moderators_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tag_moderators_user ON public.tag_moderators USING btree (user_id);


--
-- Name: idx_tags_name_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tags_name_trgm ON public.tags USING gin (name public.gin_trgm_ops);


--
-- Name: idx_tags_slug_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tags_slug_trgm ON public.tags USING gin (slug public.gin_trgm_ops);


--
-- Name: idx_tags_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tags_tenant ON public.tags USING btree (follower_count DESC);


--
-- Name: idx_tax_obligation_guides_intel_record; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tax_obligation_guides_intel_record ON public.tax_obligation_guides USING btree (intel_record_id) WHERE (intel_record_id IS NOT NULL);


--
-- Name: idx_tax_obligation_guides_tenant_dest; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tax_obligation_guides_tenant_dest ON public.tax_obligation_guides USING btree (destination_space_id, is_active);


--
-- Name: idx_template_literals_tenant_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_template_literals_tenant_active ON public.template_literals USING btree (is_active) WHERE (deleted_at IS NULL);


--
-- Name: idx_template_literals_tenant_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_template_literals_tenant_key ON public.template_literals USING btree (key) WHERE (deleted_at IS NULL);


--
-- Name: idx_ticket_comments_ticket; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ticket_comments_ticket ON public.ticket_comments USING btree (ticket_id, created_at);


--
-- Name: idx_ticket_events_ticket; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ticket_events_ticket ON public.ticket_events USING btree (ticket_id, created_at);


--
-- Name: idx_tickets_assigned; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tickets_assigned ON public.tickets USING btree (assigned_to, status);


--
-- Name: idx_tickets_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tickets_status ON public.tickets USING btree (status, created_at DESC);


--
-- Name: idx_tickets_submitter; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tickets_submitter ON public.tickets USING btree (submitter_user_id, status);


--
-- Name: idx_tickets_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tickets_type ON public.tickets USING btree (type_id, status);


--
-- Name: idx_translations_tenant_key; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_translations_tenant_key ON public.translations USING btree (key);


--
-- Name: idx_translations_tenant_locale; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_translations_tenant_locale ON public.translations USING btree (locale);


--
-- Name: idx_travel_providers_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_travel_providers_tenant ON public.travel_providers USING btree (is_active);


--
-- Name: idx_trending_snapshots_entity; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_trending_snapshots_entity ON public.trending_snapshots USING btree (entity_type, entity_id);


--
-- Name: idx_trending_snapshots_expires; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_trending_snapshots_expires ON public.trending_snapshots USING btree (expires_at);


--
-- Name: idx_trending_snapshots_tenant_entity; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_trending_snapshots_tenant_entity ON public.trending_snapshots USING btree (entity_type, entity_id);


--
-- Name: idx_trending_snapshots_trending; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_trending_snapshots_trending ON public.trending_snapshots USING btree (entity_type, "window", geo_code, score DESC);


--
-- Name: idx_ual_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ual_code ON public.user_affiliate_links USING btree (code);


--
-- Name: idx_ual_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ual_user ON public.user_affiliate_links USING btree (user_id, is_active);


--
-- Name: idx_ucv_content; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ucv_content ON public.user_content_views USING btree (content_id, viewed_at DESC);


--
-- Name: idx_ucv_dedup; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_ucv_dedup ON public.user_content_views USING btree (content_id, COALESCE((user_id)::text, session_id), date_trunc('day'::text, (viewed_at AT TIME ZONE 'UTC'::text)));


--
-- Name: idx_uma_violation_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_uma_violation_type ON public.user_moderation_actions USING btree (user_id, violation_type) WHERE ((action = 'warning'::text) AND (deleted_at IS NULL));


--
-- Name: idx_university_course_catalog_tenant_slug; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_university_course_catalog_tenant_slug ON public.university_course_catalog USING btree (slug) WHERE (deleted_at IS NULL);


--
-- Name: idx_university_course_catalog_tenant_title; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_university_course_catalog_tenant_title ON public.university_course_catalog USING btree (title) WHERE (deleted_at IS NULL);


--
-- Name: idx_university_course_offerings_course; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_university_course_offerings_course ON public.university_course_offerings USING btree (course_id) WHERE (deleted_at IS NULL);


--
-- Name: idx_university_course_offerings_university; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_university_course_offerings_university ON public.university_course_offerings USING btree (university_id) WHERE (deleted_at IS NULL);


--
-- Name: idx_unsub_tokens_subscriber; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_unsub_tokens_subscriber ON public.newsletter_unsubscribe_tokens USING btree (subscriber_id);


--
-- Name: idx_uob_active_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_uob_active_unique ON public.user_operation_bans USING btree (user_id, operation) WHERE (deleted_at IS NULL);


--
-- Name: idx_uob_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_uob_tenant ON public.user_operation_bans USING btree (operation);


--
-- Name: idx_uob_user_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_uob_user_active ON public.user_operation_bans USING btree (user_id) WHERE (deleted_at IS NULL);


--
-- Name: idx_user_badges_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_badges_user ON public.user_badges USING btree (user_id, awarded_at DESC);


--
-- Name: idx_user_blocks_blocked; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_blocks_blocked ON public.user_blocks USING btree (blocked_id);


--
-- Name: idx_user_blocks_blocker_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_blocks_blocker_tenant ON public.user_blocks USING btree (blocker_id, created_at DESC);


--
-- Name: idx_user_group_memberships_group; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_group_memberships_group ON public.user_group_memberships USING btree (group_id);


--
-- Name: idx_user_group_memberships_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_group_memberships_user ON public.user_group_memberships USING btree (user_id);


--
-- Name: idx_user_moderation_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_moderation_active ON public.user_moderation_actions USING btree (user_id, action) WHERE (deleted_at IS NULL);


--
-- Name: idx_user_moderation_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_moderation_tenant ON public.user_moderation_actions USING btree (action);


--
-- Name: idx_user_moderation_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_moderation_user ON public.user_moderation_actions USING btree (user_id, action);


--
-- Name: idx_user_mutes_muted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_mutes_muted ON public.user_mutes USING btree (muted_user_id);


--
-- Name: idx_user_mutes_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_mutes_user ON public.user_mutes USING btree (user_id);


--
-- Name: idx_user_preferences_user_id_global; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_user_preferences_user_id_global ON public.user_preferences USING btree (user_id);


--
-- Name: idx_user_privacy_settings_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_privacy_settings_tenant ON public.user_privacy_settings USING btree (user_id);


--
-- Name: idx_user_profiles_avatar_media; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_profiles_avatar_media ON public.user_profiles USING btree (avatar_media_id) WHERE (avatar_media_id IS NOT NULL);


--
-- Name: idx_user_profiles_header_media; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_profiles_header_media ON public.user_profiles USING btree (header_media_id) WHERE (header_media_id IS NOT NULL);


--
-- Name: idx_user_profiles_is_mentor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_profiles_is_mentor ON public.user_profiles USING btree (is_mentor) WHERE (is_mentor = true);


--
-- Name: idx_user_profiles_japa_intent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_profiles_japa_intent ON public.user_profiles USING btree (japa_intent) WHERE ((japa_intent IS NOT NULL) AND (japa_stage_is_public = true));


--
-- Name: idx_user_profiles_japa_stage; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_profiles_japa_stage ON public.user_profiles USING btree (japa_stage) WHERE ((japa_stage IS NOT NULL) AND (japa_stage_is_public = true));


--
-- Name: idx_user_profiles_user_id_global; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_user_profiles_user_id_global ON public.user_profiles USING btree (user_id);


--
-- Name: idx_user_source_blocks_blocker; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_source_blocks_blocker ON public.user_source_blocks USING btree (blocker_id);


--
-- Name: idx_user_source_blocks_source; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_source_blocks_source ON public.user_source_blocks USING btree (source_id);


--
-- Name: idx_users_email; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_email ON public.users USING btree (email);


--
-- Name: idx_users_email_global; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_users_email_global ON public.users USING btree (email);


--
-- Name: idx_users_geo_space; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_geo_space ON public.users USING btree (current_geo_space_id) WHERE (current_geo_space_id IS NOT NULL);


--
-- Name: idx_users_is_bot; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_is_bot ON public.users USING btree (is_bot) WHERE (is_bot = true);


--
-- Name: idx_users_source_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_source_id ON public.users USING btree (source_id) WHERE (source_id IS NOT NULL);


--
-- Name: idx_users_synthetic; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_synthetic ON public.users USING btree (is_synthetic) WHERE (is_synthetic = true);


--
-- Name: idx_users_username_changed_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_username_changed_at ON public.users USING btree (username_changed_at);


--
-- Name: idx_users_username_global; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_users_username_global ON public.users USING btree (username);


--
-- Name: idx_users_username_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_username_trgm ON public.users USING gin (username public.gin_trgm_ops);


--
-- Name: idx_video_channel_claims_pending; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_video_channel_claims_pending ON public.video_channel_claims USING btree (claimant_user_id, video_author_user_id, claim_type, COALESCE(video_source_id, '00000000-0000-0000-0000-000000000000'::uuid)) WHERE (status = 'pending'::text);


--
-- Name: idx_video_channel_claims_source_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_video_channel_claims_source_id ON public.video_channel_claims USING btree (video_source_id) WHERE (video_source_id IS NOT NULL);


--
-- Name: idx_video_import_jobs_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_video_import_jobs_user ON public.video_import_jobs USING btree (user_id, status);


--
-- Name: idx_video_sources_author; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_video_sources_author ON public.video_sources USING btree (author_user_id, status, created_at DESC);


--
-- Name: idx_video_sources_external_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_video_sources_external_id ON public.video_sources USING btree (source_type, external_id) WHERE (external_id IS NOT NULL);


--
-- Name: idx_video_sources_source_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_video_sources_source_id ON public.video_sources USING btree (source_id) WHERE (source_id IS NOT NULL);


--
-- Name: idx_visa_routes_intel_record; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_visa_routes_intel_record ON public.visa_routes USING btree (intel_record_id) WHERE (intel_record_id IS NOT NULL);


--
-- Name: idx_visa_routes_tenant_dest_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_visa_routes_tenant_dest_active ON public.visa_routes USING btree (destination_space_id, is_active);


--
-- Name: idx_visa_routes_tenant_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_visa_routes_tenant_status ON public.visa_routes USING btree (status);


--
-- Name: idx_visa_routes_tenant_visa_cat; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_visa_routes_tenant_visa_cat ON public.visa_routes USING btree (visa_category);


--
-- Name: idx_visa_timeline_milestones_timeline_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_visa_timeline_milestones_timeline_type ON public.visa_timeline_milestones USING btree (timeline_id, milestone_type);


--
-- Name: idx_visa_timelines_user_cat_date; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_visa_timelines_user_cat_date ON public.visa_timelines USING btree (user_id, visa_category, public.date_utc(created_at)) WHERE (deleted_at IS NULL);


--
-- Name: idx_wallet_ledger_idempotency; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_wallet_ledger_idempotency ON public.wallet_ledger USING btree (idempotency_key) WHERE (idempotency_key IS NOT NULL);


--
-- Name: idx_wallet_ledger_tenant_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_wallet_ledger_tenant_user ON public.wallet_ledger USING btree (user_id);


--
-- Name: idx_wallet_ledger_tenant_user_source; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_wallet_ledger_tenant_user_source ON public.wallet_ledger USING btree (user_id, source);


--
-- Name: idx_wallet_ledger_user_type_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_wallet_ledger_user_type_created ON public.wallet_ledger USING btree (user_id, ledger_type, created_at DESC);


--
-- Name: idx_wallet_withdrawal_fee_tiers_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_wallet_withdrawal_fee_tiers_tenant ON public.wallet_withdrawal_fee_tiers USING btree (is_active, "position");


--
-- Name: idx_wallet_withdrawal_methods_default; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_wallet_withdrawal_methods_default ON public.wallet_withdrawal_methods USING btree (user_id, is_default) WHERE (is_default = true);


--
-- Name: idx_wallet_withdrawal_methods_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_wallet_withdrawal_methods_user ON public.wallet_withdrawal_methods USING btree (user_id, is_active);


--
-- Name: idx_wallet_withdrawals_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_wallet_withdrawals_status ON public.wallet_withdrawals USING btree (status, created_at DESC);


--
-- Name: idx_wallet_withdrawals_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_wallet_withdrawals_user ON public.wallet_withdrawals USING btree (user_id, created_at DESC);


--
-- Name: idx_wd_retry; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_wd_retry ON public.webhook_deliveries USING btree (next_attempt_at) WHERE (status = 'retrying'::text);


--
-- Name: idx_webhook_deliveries_endpoint; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_webhook_deliveries_endpoint ON public.webhook_deliveries USING btree (endpoint_id, status);


--
-- Name: idx_webhook_deliveries_pending; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_webhook_deliveries_pending ON public.webhook_deliveries USING btree (status, created_at) WHERE (status = 'pending'::text);


--
-- Name: idx_webhook_endpoints_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_webhook_endpoints_tenant ON public.webhook_endpoints USING btree (status);


--
-- Name: idx_webhook_endpoints_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_webhook_endpoints_user ON public.webhook_endpoints USING btree (created_by) WHERE (scope = 'user'::text);


--
-- Name: idx_widget_configurations_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_widget_configurations_tenant ON public.widget_configurations USING btree (is_active);


--
-- Name: idx_widget_configurations_tenant_name; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_widget_configurations_tenant_name ON public.widget_configurations USING btree (name);


--
-- Name: idx_wiki_pages_category; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_wiki_pages_category ON public.wiki_pages USING btree (space_id, category);


--
-- Name: idx_wiki_pages_featured; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_wiki_pages_featured ON public.wiki_pages USING btree (is_featured) WHERE (is_featured = true);


--
-- Name: idx_wiki_pages_intel_record; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_wiki_pages_intel_record ON public.wiki_pages USING btree (intel_record_id) WHERE (intel_record_id IS NOT NULL);


--
-- Name: idx_wiki_pages_tenant_space; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_wiki_pages_tenant_space ON public.wiki_pages USING btree (space_id);


--
-- Name: idx_wiki_revisions_page_hash; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_wiki_revisions_page_hash ON public.wiki_revisions USING btree (wiki_page_id, content_hash) WHERE (content_hash IS NOT NULL);


--
-- Name: idx_wiki_revisions_page_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_wiki_revisions_page_status ON public.wiki_revisions USING btree (wiki_page_id, status);


--
-- Name: idx_wiki_revisions_status_kind; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_wiki_revisions_status_kind ON public.wiki_revisions USING btree (status, kind);


--
-- Name: idx_wiki_revisions_target; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_wiki_revisions_target ON public.wiki_revisions USING btree (target_entity_type, target_entity_id);


--
-- Name: idx_wiki_revisions_tenant_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_wiki_revisions_tenant_status ON public.wiki_revisions USING btree (status, created_at DESC);


--
-- Name: idx_ws_tickets_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ws_tickets_user ON public.ws_tickets USING btree (user_id, used, expires_at);


--
-- Name: instructor_payouts_tenant_id_instructor_id_period_start_per_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX instructor_payouts_tenant_id_instructor_id_period_start_per_key ON public.instructor_payouts USING btree (instructor_id, period_start, period_end);


--
-- Name: instructor_profiles_tenant_id_user_id_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX instructor_profiles_tenant_id_user_id_key ON public.instructor_profiles USING btree (user_id);


--
-- Name: intel_jobs_tenant_id_intel_record_id_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX intel_jobs_tenant_id_intel_record_id_key ON public.intel_jobs USING btree (intel_record_id);


--
-- Name: intel_records_tenant_source_external_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX intel_records_tenant_source_external_key ON public.intel_records USING btree (source_id, external_key);


--
-- Name: job_applications_tenant_id_job_id_applicant_id_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX job_applications_tenant_id_job_id_applicant_id_key ON public.job_applications USING btree (job_id, applicant_id);


--
-- Name: language_test_requirements_tenant_id_destination_space_id_v_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX language_test_requirements_tenant_id_destination_space_id_v_key ON public.language_test_requirements USING btree (destination_space_id, visa_category, test_id);


--
-- Name: learning_lessons_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX learning_lessons_tenant_id_slug_key ON public.learning_lessons USING btree (slug);


--
-- Name: learning_modules_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX learning_modules_tenant_id_slug_key ON public.learning_modules USING btree (slug);


--
-- Name: learning_tracks_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX learning_tracks_tenant_id_slug_key ON public.learning_tracks USING btree (slug);


--
-- Name: menus_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX menus_tenant_id_slug_key ON public.menus USING btree (slug);


--
-- Name: monitored_embassies_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX monitored_embassies_tenant_id_slug_key ON public.monitored_embassies USING btree (slug);


--
-- Name: newsletter_subscribers_tenant_id_email_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX newsletter_subscribers_tenant_id_email_key ON public.newsletter_subscribers USING btree (email);


--
-- Name: onboarding_state_tenant_id_user_id_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX onboarding_state_tenant_id_user_id_key ON public.onboarding_state USING btree (user_id);


--
-- Name: onboarding_templates_tenant_id_name_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX onboarding_templates_tenant_id_name_key ON public.onboarding_templates USING btree (name);


--
-- Name: organizations_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX organizations_tenant_id_slug_key ON public.organizations USING btree (slug);


--
-- Name: page_builder_pages_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX page_builder_pages_tenant_id_slug_key ON public.page_builder_pages USING btree (slug);


--
-- Name: pagebuilder_blocks_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX pagebuilder_blocks_tenant_id_slug_key ON public.pagebuilder_blocks USING btree (slug);


--
-- Name: pagebuilder_templates_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX pagebuilder_templates_tenant_id_slug_key ON public.pagebuilder_templates USING btree (slug);


--
-- Name: pagebuilder_themes_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX pagebuilder_themes_tenant_id_slug_key ON public.pagebuilder_themes USING btree (slug);


--
-- Name: pages_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX pages_tenant_id_slug_key ON public.pages USING btree (slug);


--
-- Name: payment_webhook_events_tenant_id_provider_event_id_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX payment_webhook_events_tenant_id_provider_event_id_key ON public.payment_webhook_events USING btree (provider, event_id);


--
-- Name: payout_providers_tenant_id_type_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX payout_providers_tenant_id_type_key ON public.payout_providers USING btree (type);


--
-- Name: persona_voices_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX persona_voices_tenant_id_slug_key ON public.persona_voices USING btree (slug);


--
-- Name: redirects_tenant_id_source_path_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX redirects_tenant_id_source_path_key ON public.redirects USING btree (source_path);


--
-- Name: referral_programs_singleton; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX referral_programs_singleton ON public.referral_programs USING btree ((true));


--
-- Name: referrals_referee_id_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX referrals_referee_id_key ON public.referrals USING btree (referee_id);


--
-- Name: remittance_providers_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX remittance_providers_tenant_id_slug_key ON public.remittance_providers USING btree (slug);


--
-- Name: remittance_rate_alerts_tenant_id_user_id_send_currency_rece_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX remittance_rate_alerts_tenant_id_user_id_send_currency_rece_key ON public.remittance_rate_alerts USING btree (user_id, send_currency, receive_currency, direction);


--
-- Name: reviews_tenant_id_entity_type_entity_id_reviewer_user_id_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX reviews_tenant_id_entity_type_entity_id_reviewer_user_id_key ON public.reviews USING btree (entity_type, entity_id, reviewer_user_id);


--
-- Name: share_platforms_tenant_id_key_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX share_platforms_tenant_id_key_key ON public.share_platforms USING btree (key);


--
-- Name: sidebars_tenant_id_app_position_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX sidebars_tenant_id_app_position_slug_key ON public.sidebars USING btree (app, "position", slug);


--
-- Name: source_collections_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX source_collections_tenant_id_slug_key ON public.source_collections USING btree (slug);


--
-- Name: space_mutes_tenant_id_user_id_space_id_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX space_mutes_tenant_id_user_id_space_id_key ON public.space_mutes USING btree (user_id, space_id);


--
-- Name: story_meta_content_tenant_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX story_meta_content_tenant_key ON public.story_meta USING btree (content_id);


--
-- Name: tag_moderators_tenant_id_tag_id_user_id_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX tag_moderators_tenant_id_tag_id_user_id_key ON public.tag_moderators USING btree (tag_id, user_id);


--
-- Name: tags_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX tags_tenant_id_slug_key ON public.tags USING btree (slug);


--
-- Name: tax_obligation_guides_tenant_id_origin_space_id_destination_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX tax_obligation_guides_tenant_id_origin_space_id_destination_key ON public.tax_obligation_guides USING btree (origin_space_id, destination_space_id, employment_status);


--
-- Name: tax_treaty_pairs_tenant_id_origin_space_id_destination_spac_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX tax_treaty_pairs_tenant_id_origin_space_id_destination_spac_key ON public.tax_treaty_pairs USING btree (origin_space_id, destination_space_id);


--
-- Name: ticket_types_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX ticket_types_tenant_id_slug_key ON public.ticket_types USING btree (slug);


--
-- Name: totp_credentials_tenant_id_user_id_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX totp_credentials_tenant_id_user_id_key ON public.totp_credentials USING btree (user_id);


--
-- Name: translations_tenant_id_locale_key_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX translations_tenant_id_locale_key_key ON public.translations USING btree (locale, key);


--
-- Name: travel_providers_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX travel_providers_tenant_id_slug_key ON public.travel_providers USING btree (slug);


--
-- Name: trending_snapshots_entity_window_geo_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX trending_snapshots_entity_window_geo_key ON public.trending_snapshots USING btree (entity_type, entity_id, "window", geo_code) NULLS NOT DISTINCT;


--
-- Name: university_course_catalog_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX university_course_catalog_tenant_id_slug_key ON public.university_course_catalog USING btree (slug);


--
-- Name: university_course_offerings_tenant_id_course_id_university__key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX university_course_offerings_tenant_id_course_id_university__key ON public.university_course_offerings USING btree (course_id, university_id);


--
-- Name: university_course_offerings_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX university_course_offerings_tenant_id_slug_key ON public.university_course_offerings USING btree (slug);


--
-- Name: university_track_categories_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX university_track_categories_tenant_id_slug_key ON public.university_track_categories USING btree (slug);


--
-- Name: user_badges_tenant_id_user_id_badge_id_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX user_badges_tenant_id_user_id_badge_id_key ON public.user_badges USING btree (user_id, badge_id);


--
-- Name: user_challenge_progress_tenant_id_user_id_challenge_id_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX user_challenge_progress_tenant_id_user_id_challenge_id_key ON public.user_challenge_progress USING btree (user_id, challenge_id);


--
-- Name: user_group_memberships_tenant_id_user_id_group_id_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX user_group_memberships_tenant_id_user_id_group_id_key ON public.user_group_memberships USING btree (user_id, group_id);


--
-- Name: user_group_promotions_tenant_id_source_group_id_target_grou_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX user_group_promotions_tenant_id_source_group_id_target_grou_key ON public.user_group_promotions USING btree (source_group_id, target_group_id);


--
-- Name: user_groups_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX user_groups_tenant_id_slug_key ON public.user_groups USING btree (slug);


--
-- Name: user_mutes_tenant_id_user_id_muted_user_id_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX user_mutes_tenant_id_user_id_muted_user_id_key ON public.user_mutes USING btree (user_id, muted_user_id);


--
-- Name: user_roles_tenant_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX user_roles_tenant_id_slug_key ON public.user_roles USING btree (slug);


--
-- Name: video_sources_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX video_sources_slug_key ON public.video_sources USING btree (slug);


--
-- Name: visa_processing_stats_tenant_id_origin_space_id_destination_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX visa_processing_stats_tenant_id_origin_space_id_destination_key ON public.visa_processing_stats USING btree (origin_space_id, destination_space_id, visa_category, consulate, period_start);


--
-- Name: visa_routes_tenant_id_destination_space_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX visa_routes_tenant_id_destination_space_id_slug_key ON public.visa_routes USING btree (destination_space_id, slug);


--
-- Name: wallet_settings_singleton; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX wallet_settings_singleton ON public.wallet_settings USING btree ((true));


--
-- Name: wallet_withdrawals_tenant_id_idempotency_key_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX wallet_withdrawals_tenant_id_idempotency_key_key ON public.wallet_withdrawals USING btree (idempotency_key);


--
-- Name: wiki_pages_tenant_id_space_id_slug_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX wiki_pages_tenant_id_space_id_slug_key ON public.wiki_pages USING btree (space_id, slug);


--
-- Name: forum_threads soft_delete_forum_thread_content; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER soft_delete_forum_thread_content BEFORE DELETE ON public.forum_threads FOR EACH ROW EXECUTE FUNCTION public.trg_soft_delete_forum_thread_content();


--
-- Name: ad_networks trg_ad_networks_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_ad_networks_updated_at BEFORE UPDATE ON public.ad_networks FOR EACH ROW EXECUTE FUNCTION public.update_ad_networks_updated_at();


--
-- Name: comment_views trg_comment_views_after_insert; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_comment_views_after_insert AFTER INSERT ON public.comment_views FOR EACH ROW EXECUTE FUNCTION public.trg_comment_views_increment();


--
-- Name: intel_records trg_intel_records_post_count; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_intel_records_post_count AFTER INSERT OR DELETE OR UPDATE OF status ON public.intel_records FOR EACH ROW EXECUTE FUNCTION public.trg_intel_records_post_count_sync();


--
-- Name: mentorship_connections trg_mentee_count; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_mentee_count AFTER INSERT OR DELETE OR UPDATE OF status ON public.mentorship_connections FOR EACH ROW EXECUTE FUNCTION public.trg_update_active_mentee_count();


--
-- Name: reply_views trg_reply_views_increment; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_reply_views_increment AFTER INSERT ON public.reply_views FOR EACH ROW EXECUTE FUNCTION public.increment_reply_view_count();


--
-- Name: content_tags trg_tags_content_count_delete; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_tags_content_count_delete AFTER DELETE ON public.content_tags FOR EACH ROW EXECUTE FUNCTION public.fn_tags_content_count_sync();


--
-- Name: content_tags trg_tags_content_count_insert; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_tags_content_count_insert AFTER INSERT ON public.content_tags FOR EACH ROW EXECUTE FUNCTION public.fn_tags_content_count_sync();


--
-- Name: account_deletion_requests account_deletion_requests_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_deletion_requests
    ADD CONSTRAINT account_deletion_requests_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: achievement_meta achievement_meta_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.achievement_meta
    ADD CONSTRAINT achievement_meta_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: achievement_meta achievement_meta_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.achievement_meta
    ADD CONSTRAINT achievement_meta_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id);


--
-- Name: achievement_meta achievement_meta_origin_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.achievement_meta
    ADD CONSTRAINT achievement_meta_origin_space_id_fkey FOREIGN KEY (origin_space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: activity_events activity_events_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.activity_events
    ADD CONSTRAINT activity_events_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: ad_analytics_daily ad_analytics_daily_campaign_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ad_analytics_daily
    ADD CONSTRAINT ad_analytics_daily_campaign_id_fkey FOREIGN KEY (campaign_id) REFERENCES public.ad_campaigns(id) ON DELETE CASCADE;


--
-- Name: ad_analytics_daily ad_analytics_daily_slot_type_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ad_analytics_daily
    ADD CONSTRAINT ad_analytics_daily_slot_type_id_fkey FOREIGN KEY (slot_type_id) REFERENCES public.ad_slot_types(id);


--
-- Name: ad_campaigns ad_campaigns_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ad_campaigns
    ADD CONSTRAINT ad_campaigns_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: ad_campaigns ad_campaigns_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ad_campaigns
    ADD CONSTRAINT ad_campaigns_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.organizations(id) ON DELETE SET NULL;


--
-- Name: ad_campaigns ad_campaigns_slot_type_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ad_campaigns
    ADD CONSTRAINT ad_campaigns_slot_type_id_fkey FOREIGN KEY (slot_type_id) REFERENCES public.ad_slot_types(id);


--
-- Name: ad_events ad_events_campaign_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ad_events
    ADD CONSTRAINT ad_events_campaign_id_fkey FOREIGN KEY (campaign_id) REFERENCES public.ad_campaigns(id) ON DELETE CASCADE;


--
-- Name: ad_events ad_events_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ad_events
    ADD CONSTRAINT ad_events_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: admin_broadcasts admin_broadcasts_sent_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_broadcasts
    ADD CONSTRAINT admin_broadcasts_sent_by_fkey FOREIGN KEY (sent_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: ads_campaigns ads_campaigns_advertiser_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ads_campaigns
    ADD CONSTRAINT ads_campaigns_advertiser_id_fkey FOREIGN KEY (advertiser_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: affiliate_content_commissions affiliate_content_commissions_author_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.affiliate_content_commissions
    ADD CONSTRAINT affiliate_content_commissions_author_user_id_fkey FOREIGN KEY (author_user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: affiliate_content_commissions affiliate_content_commissions_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.affiliate_content_commissions
    ADD CONSTRAINT affiliate_content_commissions_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: affiliate_content_commissions affiliate_content_commissions_provider_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.affiliate_content_commissions
    ADD CONSTRAINT affiliate_content_commissions_provider_id_fkey FOREIGN KEY (provider_id) REFERENCES public.affiliate_providers(id) ON DELETE CASCADE;


--
-- Name: affiliate_link_events affiliate_link_events_converted_payment_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.affiliate_link_events
    ADD CONSTRAINT affiliate_link_events_converted_payment_id_fkey FOREIGN KEY (converted_payment_id) REFERENCES public.payments(id) ON DELETE SET NULL;


--
-- Name: affiliate_link_events affiliate_link_events_link_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.affiliate_link_events
    ADD CONSTRAINT affiliate_link_events_link_id_fkey FOREIGN KEY (link_id) REFERENCES public.user_affiliate_links(id) ON DELETE CASCADE;


--
-- Name: affiliate_program_members affiliate_program_members_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.affiliate_program_members
    ADD CONSTRAINT affiliate_program_members_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: affiliate_providers affiliate_providers_logo_media_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.affiliate_providers
    ADD CONSTRAINT affiliate_providers_logo_media_id_fkey FOREIGN KEY (logo_media_id) REFERENCES public.media(id) ON DELETE SET NULL;


--
-- Name: ai_agent_sessions ai_agent_sessions_geo_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_agent_sessions
    ADD CONSTRAINT ai_agent_sessions_geo_space_id_fkey FOREIGN KEY (geo_space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: ai_agent_sessions ai_agent_sessions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_agent_sessions
    ADD CONSTRAINT ai_agent_sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: ai_agent_tool_calls ai_agent_tool_calls_hitl_review_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_agent_tool_calls
    ADD CONSTRAINT ai_agent_tool_calls_hitl_review_id_fkey FOREIGN KEY (hitl_review_id) REFERENCES public.ai_hitl_reviews(id) ON DELETE SET NULL;


--
-- Name: ai_agent_tool_calls ai_agent_tool_calls_session_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_agent_tool_calls
    ADD CONSTRAINT ai_agent_tool_calls_session_id_fkey FOREIGN KEY (session_id) REFERENCES public.ai_agent_sessions(id) ON DELETE CASCADE;


--
-- Name: ai_hitl_reviews ai_hitl_reviews_reviewer_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_hitl_reviews
    ADD CONSTRAINT ai_hitl_reviews_reviewer_id_fkey FOREIGN KEY (reviewer_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: ai_personas ai_personas_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_personas
    ADD CONSTRAINT ai_personas_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: ai_personas ai_personas_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_personas
    ADD CONSTRAINT ai_personas_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: ai_personas ai_personas_voice_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_personas
    ADD CONSTRAINT ai_personas_voice_id_fkey FOREIGN KEY (voice_id) REFERENCES public.persona_voices(id) ON DELETE SET NULL;


--
-- Name: ai_tool_usage ai_tool_usage_tool_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_tool_usage
    ADD CONSTRAINT ai_tool_usage_tool_id_fkey FOREIGN KEY (tool_id) REFERENCES public.ai_tools(id) ON DELETE CASCADE;


--
-- Name: ai_tool_usage ai_tool_usage_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_tool_usage
    ADD CONSTRAINT ai_tool_usage_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: ai_usage_log ai_usage_log_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_usage_log
    ADD CONSTRAINT ai_usage_log_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: api_key_ip_allowlist api_key_ip_allowlist_api_key_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_key_ip_allowlist
    ADD CONSTRAINT api_key_ip_allowlist_api_key_id_fkey FOREIGN KEY (api_key_id) REFERENCES public.api_keys(id) ON DELETE CASCADE;


--
-- Name: api_keys api_keys_revoked_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_revoked_by_fkey FOREIGN KEY (revoked_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: api_keys api_keys_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: appointment_alerts appointment_alerts_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.appointment_alerts
    ADD CONSTRAINT appointment_alerts_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE RESTRICT;


--
-- Name: appointment_alerts appointment_alerts_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.appointment_alerts
    ADD CONSTRAINT appointment_alerts_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: arrival_checklist_guides arrival_checklist_guides_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arrival_checklist_guides
    ADD CONSTRAINT arrival_checklist_guides_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE RESTRICT;


--
-- Name: arrival_checklist_guides arrival_checklist_guides_hitl_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arrival_checklist_guides
    ADD CONSTRAINT arrival_checklist_guides_hitl_reviewed_by_fkey FOREIGN KEY (hitl_reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: arrival_checklist_items arrival_checklist_items_guide_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arrival_checklist_items
    ADD CONSTRAINT arrival_checklist_items_guide_id_fkey FOREIGN KEY (guide_id) REFERENCES public.arrival_checklist_guides(id) ON DELETE CASCADE;


--
-- Name: assessment_results assessment_results_submission_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.assessment_results
    ADD CONSTRAINT assessment_results_submission_id_fkey FOREIGN KEY (submission_id) REFERENCES public.form_submissions(id) ON DELETE CASCADE;


--
-- Name: assessment_results assessment_results_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.assessment_results
    ADD CONSTRAINT assessment_results_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: attestation_community_updates attestation_community_updates_pathway_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.attestation_community_updates
    ADD CONSTRAINT attestation_community_updates_pathway_id_fkey FOREIGN KEY (pathway_id) REFERENCES public.attestation_pathways(id) ON DELETE CASCADE;


--
-- Name: attestation_community_updates attestation_community_updates_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.attestation_community_updates
    ADD CONSTRAINT attestation_community_updates_reviewed_by_fkey FOREIGN KEY (reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: attestation_community_updates attestation_community_updates_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.attestation_community_updates
    ADD CONSTRAINT attestation_community_updates_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: attestation_pathways attestation_pathways_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.attestation_pathways
    ADD CONSTRAINT attestation_pathways_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE RESTRICT;


--
-- Name: attestation_pathways attestation_pathways_hitl_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.attestation_pathways
    ADD CONSTRAINT attestation_pathways_hitl_reviewed_by_fkey FOREIGN KEY (hitl_reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: attestation_pathways attestation_pathways_issuing_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.attestation_pathways
    ADD CONSTRAINT attestation_pathways_issuing_space_id_fkey FOREIGN KEY (issuing_space_id) REFERENCES public.spaces(id) ON DELETE RESTRICT;


--
-- Name: audit_logs audit_logs_actor_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_logs
    ADD CONSTRAINT audit_logs_actor_user_id_fkey FOREIGN KEY (actor_user_id) REFERENCES public.users(id);


--
-- Name: audit_logs audit_logs_impersonated_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_logs
    ADD CONSTRAINT audit_logs_impersonated_user_id_fkey FOREIGN KEY (impersonated_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: badge_definitions badge_definitions_icon_media_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.badge_definitions
    ADD CONSTRAINT badge_definitions_icon_media_id_fkey FOREIGN KEY (icon_media_id) REFERENCES public.media(id);


--
-- Name: blog_series blog_series_author_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_series
    ADD CONSTRAINT blog_series_author_user_id_fkey FOREIGN KEY (author_user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: blog_series blog_series_cover_image_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_series
    ADD CONSTRAINT blog_series_cover_image_id_fkey FOREIGN KEY (cover_image_id) REFERENCES public.media(id) ON DELETE SET NULL;


--
-- Name: blog_series_posts blog_series_posts_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_series_posts
    ADD CONSTRAINT blog_series_posts_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: blog_series_posts blog_series_posts_series_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_series_posts
    ADD CONSTRAINT blog_series_posts_series_id_fkey FOREIGN KEY (series_id) REFERENCES public.blog_series(id) ON DELETE CASCADE;


--
-- Name: bookings bookings_booker_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bookings
    ADD CONSTRAINT bookings_booker_id_fkey FOREIGN KEY (booker_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: bookmark_folders bookmark_folders_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bookmark_folders
    ADD CONSTRAINT bookmark_folders_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: bookmarks bookmarks_folder_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bookmarks
    ADD CONSTRAINT bookmarks_folder_id_fkey FOREIGN KEY (folder_id) REFERENCES public.bookmark_folders(id) ON DELETE SET NULL;


--
-- Name: bookmarks bookmarks_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bookmarks
    ADD CONSTRAINT bookmarks_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: calendar_entries calendar_entries_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.calendar_entries
    ADD CONSTRAINT calendar_entries_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: calendar_entries calendar_entries_hitl_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.calendar_entries
    ADD CONSTRAINT calendar_entries_hitl_reviewed_by_fkey FOREIGN KEY (hitl_reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: calendar_entries calendar_entries_origin_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.calendar_entries
    ADD CONSTRAINT calendar_entries_origin_space_id_fkey FOREIGN KEY (origin_space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: calendar_entries calendar_entries_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.calendar_entries
    ADD CONSTRAINT calendar_entries_reviewed_by_fkey FOREIGN KEY (reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: calendar_entries calendar_entries_submitted_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.calendar_entries
    ADD CONSTRAINT calendar_entries_submitted_by_fkey FOREIGN KEY (submitted_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: calendar_saves calendar_saves_entry_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.calendar_saves
    ADD CONSTRAINT calendar_saves_entry_id_fkey FOREIGN KEY (entry_id) REFERENCES public.calendar_entries(id) ON DELETE CASCADE;


--
-- Name: calendar_saves calendar_saves_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.calendar_saves
    ADD CONSTRAINT calendar_saves_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: calendar_subscriptions calendar_subscriptions_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.calendar_subscriptions
    ADD CONSTRAINT calendar_subscriptions_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE CASCADE;


--
-- Name: calendar_subscriptions calendar_subscriptions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.calendar_subscriptions
    ADD CONSTRAINT calendar_subscriptions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: challenge_definitions challenge_definitions_image_media_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.challenge_definitions
    ADD CONSTRAINT challenge_definitions_image_media_id_fkey FOREIGN KEY (image_media_id) REFERENCES public.media(id);


--
-- Name: challenge_definitions challenge_definitions_reward_badge_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.challenge_definitions
    ADD CONSTRAINT challenge_definitions_reward_badge_id_fkey FOREIGN KEY (reward_badge_id) REFERENCES public.badge_definitions(id);


--
-- Name: checklist_progress checklist_progress_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checklist_progress
    ADD CONSTRAINT checklist_progress_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: comment_views comment_views_comment_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comment_views
    ADD CONSTRAINT comment_views_comment_id_fkey FOREIGN KEY (comment_id) REFERENCES public.comments(id) ON DELETE CASCADE;


--
-- Name: comment_views comment_views_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comment_views
    ADD CONSTRAINT comment_views_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: comment_votes comment_votes_comment_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comment_votes
    ADD CONSTRAINT comment_votes_comment_id_fkey FOREIGN KEY (comment_id) REFERENCES public.comments(id) ON DELETE CASCADE;


--
-- Name: comment_votes comment_votes_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comment_votes
    ADD CONSTRAINT comment_votes_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: comments comments_author_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comments
    ADD CONSTRAINT comments_author_user_id_fkey FOREIGN KEY (author_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: comments comments_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comments
    ADD CONSTRAINT comments_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: comments comments_parent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comments
    ADD CONSTRAINT comments_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES public.comments(id) ON DELETE CASCADE;


--
-- Name: comments comments_removed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comments
    ADD CONSTRAINT comments_removed_by_fkey FOREIGN KEY (removed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: community_organisations community_organisations_city_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.community_organisations
    ADD CONSTRAINT community_organisations_city_space_id_fkey FOREIGN KEY (city_space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: community_organisations community_organisations_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.community_organisations
    ADD CONSTRAINT community_organisations_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE RESTRICT;


--
-- Name: community_organisations community_organisations_guide_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.community_organisations
    ADD CONSTRAINT community_organisations_guide_id_fkey FOREIGN KEY (guide_id) REFERENCES public.contents(id) ON DELETE SET NULL;


--
-- Name: community_organisations community_organisations_hitl_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.community_organisations
    ADD CONSTRAINT community_organisations_hitl_reviewed_by_fkey FOREIGN KEY (hitl_reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: community_organisations community_organisations_intel_record_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.community_organisations
    ADD CONSTRAINT community_organisations_intel_record_id_fkey FOREIGN KEY (intel_record_id) REFERENCES public.intel_records(id) ON DELETE SET NULL;


--
-- Name: community_organisations community_organisations_japaconnect_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.community_organisations
    ADD CONSTRAINT community_organisations_japaconnect_org_id_fkey FOREIGN KEY (japaconnect_org_id) REFERENCES public.organizations(id) ON DELETE SET NULL;


--
-- Name: community_organisations community_organisations_japaconnect_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.community_organisations
    ADD CONSTRAINT community_organisations_japaconnect_space_id_fkey FOREIGN KEY (japaconnect_space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: community_organisations community_organisations_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.community_organisations
    ADD CONSTRAINT community_organisations_reviewed_by_fkey FOREIGN KEY (reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: community_organisations community_organisations_submitted_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.community_organisations
    ADD CONSTRAINT community_organisations_submitted_by_fkey FOREIGN KEY (submitted_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: comparison_content_metadata comparison_content_metadata_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_content_metadata
    ADD CONSTRAINT comparison_content_metadata_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: comparison_content_metadata comparison_content_metadata_promoted_from_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_content_metadata
    ADD CONSTRAINT comparison_content_metadata_promoted_from_id_fkey FOREIGN KEY (promoted_from_id) REFERENCES public.contents(id) ON DELETE SET NULL;


--
-- Name: comparison_data_ingestion_jobs comparison_data_ingestion_jobs_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_data_ingestion_jobs
    ADD CONSTRAINT comparison_data_ingestion_jobs_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: comparison_data_sources comparison_data_sources_source_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_data_sources
    ADD CONSTRAINT comparison_data_sources_source_id_fkey FOREIGN KEY (source_id) REFERENCES public.intel_sources(id) ON DELETE SET NULL;


--
-- Name: comparison_dimensions comparison_dimensions_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_dimensions
    ADD CONSTRAINT comparison_dimensions_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: comparison_dimensions comparison_dimensions_source_field_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_dimensions
    ADD CONSTRAINT comparison_dimensions_source_field_id_fkey FOREIGN KEY (source_field_id) REFERENCES public.custom_fields(id);


--
-- Name: comparison_dimensions comparison_dimensions_type_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_dimensions
    ADD CONSTRAINT comparison_dimensions_type_id_fkey FOREIGN KEY (type_id) REFERENCES public.comparison_type_definitions(id) ON DELETE SET NULL;


--
-- Name: comparison_items comparison_items_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_items
    ADD CONSTRAINT comparison_items_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: comparison_items comparison_items_intel_record_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_items
    ADD CONSTRAINT comparison_items_intel_record_id_fkey FOREIGN KEY (intel_record_id) REFERENCES public.intel_records(id) ON DELETE SET NULL;


--
-- Name: comparison_items comparison_items_type_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_items
    ADD CONSTRAINT comparison_items_type_id_fkey FOREIGN KEY (type_id) REFERENCES public.comparison_type_definitions(id) ON DELETE SET NULL;


--
-- Name: comparison_profiles comparison_profiles_source_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_profiles
    ADD CONSTRAINT comparison_profiles_source_space_id_fkey FOREIGN KEY (source_space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: comparison_profiles comparison_profiles_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_profiles
    ADD CONSTRAINT comparison_profiles_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: comparison_recommendations comparison_recommendations_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_recommendations
    ADD CONSTRAINT comparison_recommendations_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: comparison_recommendations comparison_recommendations_factor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_recommendations
    ADD CONSTRAINT comparison_recommendations_factor_id_fkey FOREIGN KEY (factor_id) REFERENCES public.comparison_score_factors(id) ON DELETE SET NULL;


--
-- Name: comparison_recommendations comparison_recommendations_item_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_recommendations
    ADD CONSTRAINT comparison_recommendations_item_id_fkey FOREIGN KEY (item_id) REFERENCES public.comparison_items(id) ON DELETE CASCADE;


--
-- Name: comparison_recommendations comparison_recommendations_profile_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_recommendations
    ADD CONSTRAINT comparison_recommendations_profile_id_fkey FOREIGN KEY (profile_id) REFERENCES public.comparison_profiles(id) ON DELETE SET NULL;


--
-- Name: comparison_score_factors comparison_score_factors_comparison_type_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_score_factors
    ADD CONSTRAINT comparison_score_factors_comparison_type_id_fkey FOREIGN KEY (comparison_type_id) REFERENCES public.comparison_type_definitions(id) ON DELETE CASCADE;


--
-- Name: comparison_scores comparison_scores_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_scores
    ADD CONSTRAINT comparison_scores_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: comparison_scores comparison_scores_dimension_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_scores
    ADD CONSTRAINT comparison_scores_dimension_id_fkey FOREIGN KEY (dimension_id) REFERENCES public.comparison_dimensions(id) ON DELETE SET NULL;


--
-- Name: comparison_scores comparison_scores_factor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_scores
    ADD CONSTRAINT comparison_scores_factor_id_fkey FOREIGN KEY (factor_id) REFERENCES public.comparison_score_factors(id) ON DELETE CASCADE;


--
-- Name: comparison_scores comparison_scores_item_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_scores
    ADD CONSTRAINT comparison_scores_item_id_fkey FOREIGN KEY (item_id) REFERENCES public.comparison_items(id) ON DELETE CASCADE;


--
-- Name: comparison_scores comparison_scores_profile_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_scores
    ADD CONSTRAINT comparison_scores_profile_id_fkey FOREIGN KEY (profile_id) REFERENCES public.comparison_profiles(id) ON DELETE SET NULL;


--
-- Name: comparison_type_access comparison_type_access_type_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_type_access
    ADD CONSTRAINT comparison_type_access_type_id_fkey FOREIGN KEY (type_id) REFERENCES public.comparison_type_definitions(id) ON DELETE CASCADE;


--
-- Name: comparison_type_access comparison_type_access_user_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_type_access
    ADD CONSTRAINT comparison_type_access_user_group_id_fkey FOREIGN KEY (user_group_id) REFERENCES public.user_groups(id);


--
-- Name: comparison_type_definitions comparison_type_definitions_field_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_type_definitions
    ADD CONSTRAINT comparison_type_definitions_field_group_id_fkey FOREIGN KEY (field_group_id) REFERENCES public.custom_field_groups(id);


--
-- Name: comparison_values comparison_values_data_source_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_values
    ADD CONSTRAINT comparison_values_data_source_id_fkey FOREIGN KEY (data_source_id) REFERENCES public.comparison_data_sources(id) ON DELETE SET NULL;


--
-- Name: comparison_values comparison_values_dimension_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_values
    ADD CONSTRAINT comparison_values_dimension_id_fkey FOREIGN KEY (dimension_id) REFERENCES public.comparison_dimensions(id) ON DELETE CASCADE;


--
-- Name: comparison_values comparison_values_item_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comparison_values
    ADD CONSTRAINT comparison_values_item_id_fkey FOREIGN KEY (item_id) REFERENCES public.comparison_items(id) ON DELETE CASCADE;


--
-- Name: consent_records consent_records_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.consent_records
    ADD CONSTRAINT consent_records_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: content_affiliate_scan_results content_affiliate_scan_results_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_affiliate_scan_results
    ADD CONSTRAINT content_affiliate_scan_results_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: content_affiliate_scan_results content_affiliate_scan_results_matched_provider_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_affiliate_scan_results
    ADD CONSTRAINT content_affiliate_scan_results_matched_provider_id_fkey FOREIGN KEY (matched_provider_id) REFERENCES public.affiliate_providers(id) ON DELETE SET NULL;


--
-- Name: content_affiliate_scan_results content_affiliate_scan_results_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_affiliate_scan_results
    ADD CONSTRAINT content_affiliate_scan_results_reviewed_by_fkey FOREIGN KEY (reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: content_campaign_items content_campaign_items_campaign_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_campaign_items
    ADD CONSTRAINT content_campaign_items_campaign_id_fkey FOREIGN KEY (campaign_id) REFERENCES public.content_campaigns(id) ON DELETE CASCADE;


--
-- Name: content_campaign_items content_campaign_items_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_campaign_items
    ADD CONSTRAINT content_campaign_items_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id);


--
-- Name: content_campaign_items content_campaign_items_persona_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_campaign_items
    ADD CONSTRAINT content_campaign_items_persona_id_fkey FOREIGN KEY (persona_id) REFERENCES public.ai_personas(id);


--
-- Name: content_campaign_items content_campaign_items_review_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_campaign_items
    ADD CONSTRAINT content_campaign_items_review_id_fkey FOREIGN KEY (review_id) REFERENCES public.content_reviews(id);


--
-- Name: content_campaign_items content_campaign_items_studio_job_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_campaign_items
    ADD CONSTRAINT content_campaign_items_studio_job_id_fkey FOREIGN KEY (studio_job_id) REFERENCES public.studio_jobs(id);


--
-- Name: content_campaign_items content_campaign_items_target_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_campaign_items
    ADD CONSTRAINT content_campaign_items_target_space_id_fkey FOREIGN KEY (target_space_id) REFERENCES public.spaces(id);


--
-- Name: content_campaigns content_campaigns_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_campaigns
    ADD CONSTRAINT content_campaigns_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: content_campaigns content_campaigns_target_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_campaigns
    ADD CONSTRAINT content_campaigns_target_space_id_fkey FOREIGN KEY (target_space_id) REFERENCES public.spaces(id);


--
-- Name: content_engagement content_engagement_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_engagement
    ADD CONSTRAINT content_engagement_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: content_featured content_featured_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_featured
    ADD CONSTRAINT content_featured_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: content_imports content_imports_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_imports
    ADD CONSTRAINT content_imports_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: content_reports content_reports_reporter_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_reports
    ADD CONSTRAINT content_reports_reporter_user_id_fkey FOREIGN KEY (reporter_user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: content_reports content_reports_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_reports
    ADD CONSTRAINT content_reports_reviewed_by_fkey FOREIGN KEY (reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: content_reviews content_reviews_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_reviews
    ADD CONSTRAINT content_reviews_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: content_reviews content_reviews_reviewer_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_reviews
    ADD CONSTRAINT content_reviews_reviewer_user_id_fkey FOREIGN KEY (reviewer_user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: content_revisions content_revisions_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_revisions
    ADD CONSTRAINT content_revisions_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: content_revisions content_revisions_edited_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_revisions
    ADD CONSTRAINT content_revisions_edited_by_fkey FOREIGN KEY (edited_by) REFERENCES public.users(id);


--
-- Name: content_scores content_scores_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_scores
    ADD CONSTRAINT content_scores_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: content_shares content_shares_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_shares
    ADD CONSTRAINT content_shares_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: content_shares content_shares_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_shares
    ADD CONSTRAINT content_shares_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: content_spaces content_spaces_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_spaces
    ADD CONSTRAINT content_spaces_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: content_spaces content_spaces_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_spaces
    ADD CONSTRAINT content_spaces_space_id_fkey FOREIGN KEY (space_id) REFERENCES public.spaces(id) ON DELETE CASCADE;


--
-- Name: content_tags content_tags_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_tags
    ADD CONSTRAINT content_tags_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: content_tags content_tags_tag_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_tags
    ADD CONSTRAINT content_tags_tag_id_fkey FOREIGN KEY (tag_id) REFERENCES public.tags(id) ON DELETE CASCADE;


--
-- Name: content_thread_links content_thread_links_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_thread_links
    ADD CONSTRAINT content_thread_links_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: content_thread_links content_thread_links_thread_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_thread_links
    ADD CONSTRAINT content_thread_links_thread_id_fkey FOREIGN KEY (thread_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: content_views content_views_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_views
    ADD CONSTRAINT content_views_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: content_views content_views_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_views
    ADD CONSTRAINT content_views_space_id_fkey FOREIGN KEY (space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: content_views content_views_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_views
    ADD CONSTRAINT content_views_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: contents contents_author_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contents
    ADD CONSTRAINT contents_author_id_fkey FOREIGN KEY (author_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: contents contents_geo_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contents
    ADD CONSTRAINT contents_geo_space_id_fkey FOREIGN KEY (geo_space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: contents contents_hitl_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contents
    ADD CONSTRAINT contents_hitl_reviewed_by_fkey FOREIGN KEY (hitl_reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: contents contents_intel_record_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contents
    ADD CONSTRAINT contents_intel_record_id_fkey FOREIGN KEY (intel_record_id) REFERENCES public.intel_records(id) ON DELETE SET NULL;


--
-- Name: contents contents_intel_source_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contents
    ADD CONSTRAINT contents_intel_source_id_fkey FOREIGN KEY (intel_source_id) REFERENCES public.intel_sources(id) ON DELETE SET NULL;


--
-- Name: contents contents_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contents
    ADD CONSTRAINT contents_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.organizations(id) ON DELETE SET NULL;


--
-- Name: contents contents_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contents
    ADD CONSTRAINT contents_space_id_fkey FOREIGN KEY (space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: contents contents_video_source_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contents
    ADD CONSTRAINT contents_video_source_id_fkey FOREIGN KEY (video_source_id) REFERENCES public.video_sources(id) ON DELETE SET NULL;


--
-- Name: coupon_events coupon_events_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.coupon_events
    ADD CONSTRAINT coupon_events_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE SET NULL;


--
-- Name: coupon_events coupon_events_coupon_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.coupon_events
    ADD CONSTRAINT coupon_events_coupon_id_fkey FOREIGN KEY (coupon_id) REFERENCES public.coupons(id) ON DELETE CASCADE;


--
-- Name: coupon_events coupon_events_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.coupon_events
    ADD CONSTRAINT coupon_events_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: coupons coupons_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.coupons
    ADD CONSTRAINT coupons_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: coupons coupons_provider_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.coupons
    ADD CONSTRAINT coupons_provider_id_fkey FOREIGN KEY (provider_id) REFERENCES public.affiliate_providers(id) ON DELETE SET NULL;


--
-- Name: course_access_grants course_access_grants_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.course_access_grants
    ADD CONSTRAINT course_access_grants_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: course_access_grants course_access_grants_payment_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.course_access_grants
    ADD CONSTRAINT course_access_grants_payment_id_fkey FOREIGN KEY (payment_id) REFERENCES public.payments(id);


--
-- Name: course_access_grants course_access_grants_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.course_access_grants
    ADD CONSTRAINT course_access_grants_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: credential_community_notes credential_community_notes_pathway_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.credential_community_notes
    ADD CONSTRAINT credential_community_notes_pathway_id_fkey FOREIGN KEY (pathway_id) REFERENCES public.credential_pathways(id) ON DELETE CASCADE;


--
-- Name: credential_community_notes credential_community_notes_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.credential_community_notes
    ADD CONSTRAINT credential_community_notes_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: credential_pathways credential_pathways_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.credential_pathways
    ADD CONSTRAINT credential_pathways_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE RESTRICT;


--
-- Name: credential_pathways credential_pathways_guide_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.credential_pathways
    ADD CONSTRAINT credential_pathways_guide_id_fkey FOREIGN KEY (guide_id) REFERENCES public.contents(id) ON DELETE SET NULL;


--
-- Name: credential_pathways credential_pathways_hitl_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.credential_pathways
    ADD CONSTRAINT credential_pathways_hitl_reviewed_by_fkey FOREIGN KEY (hitl_reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: credential_pathways credential_pathways_intel_record_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.credential_pathways
    ADD CONSTRAINT credential_pathways_intel_record_id_fkey FOREIGN KEY (intel_record_id) REFERENCES public.intel_records(id) ON DELETE SET NULL;


--
-- Name: credential_pathways credential_pathways_issuing_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.credential_pathways
    ADD CONSTRAINT credential_pathways_issuing_space_id_fkey FOREIGN KEY (issuing_space_id) REFERENCES public.spaces(id) ON DELETE RESTRICT;


--
-- Name: credential_pathways credential_pathways_profession_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.credential_pathways
    ADD CONSTRAINT credential_pathways_profession_id_fkey FOREIGN KEY (profession_id) REFERENCES public.credential_professions(id) ON DELETE RESTRICT;


--
-- Name: crs_draws crs_draws_hitl_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.crs_draws
    ADD CONSTRAINT crs_draws_hitl_reviewed_by_fkey FOREIGN KEY (hitl_reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: crs_factor_points crs_factor_points_ruleset_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.crs_factor_points
    ADD CONSTRAINT crs_factor_points_ruleset_id_fkey FOREIGN KEY (ruleset_id) REFERENCES public.crs_rulesets(id) ON DELETE CASCADE;


--
-- Name: crs_rulesets crs_rulesets_hitl_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.crs_rulesets
    ADD CONSTRAINT crs_rulesets_hitl_reviewed_by_fkey FOREIGN KEY (hitl_reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: crs_score_history crs_score_history_ruleset_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.crs_score_history
    ADD CONSTRAINT crs_score_history_ruleset_id_fkey FOREIGN KEY (ruleset_id) REFERENCES public.crs_rulesets(id) ON DELETE RESTRICT;


--
-- Name: crs_score_history crs_score_history_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.crs_score_history
    ADD CONSTRAINT crs_score_history_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: custom_field_group_assignments custom_field_group_assignments_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_field_group_assignments
    ADD CONSTRAINT custom_field_group_assignments_group_id_fkey FOREIGN KEY (group_id) REFERENCES public.custom_field_groups(id) ON DELETE CASCADE;


--
-- Name: custom_field_permissions custom_field_permissions_field_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_field_permissions
    ADD CONSTRAINT custom_field_permissions_field_id_fkey FOREIGN KEY (field_id) REFERENCES public.custom_fields(id) ON DELETE CASCADE;


--
-- Name: custom_field_permissions custom_field_permissions_user_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_field_permissions
    ADD CONSTRAINT custom_field_permissions_user_group_id_fkey FOREIGN KEY (user_group_id) REFERENCES public.user_groups(id) ON DELETE CASCADE;


--
-- Name: custom_field_templates custom_field_templates_field_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_field_templates
    ADD CONSTRAINT custom_field_templates_field_group_id_fkey FOREIGN KEY (field_group_id) REFERENCES public.custom_field_groups(id) ON DELETE CASCADE;


--
-- Name: custom_field_values custom_field_values_field_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_field_values
    ADD CONSTRAINT custom_field_values_field_id_fkey FOREIGN KEY (field_id) REFERENCES public.custom_fields(id) ON DELETE CASCADE;


--
-- Name: custom_fields custom_fields_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_fields
    ADD CONSTRAINT custom_fields_group_id_fkey FOREIGN KEY (group_id) REFERENCES public.custom_field_groups(id) ON DELETE CASCADE;


--
-- Name: cv_format_sessions cv_format_sessions_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cv_format_sessions
    ADD CONSTRAINT cv_format_sessions_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE RESTRICT;


--
-- Name: cv_format_sessions cv_format_sessions_hitl_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cv_format_sessions
    ADD CONSTRAINT cv_format_sessions_hitl_reviewed_by_fkey FOREIGN KEY (hitl_reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: cv_format_sessions cv_format_sessions_origin_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cv_format_sessions
    ADD CONSTRAINT cv_format_sessions_origin_space_id_fkey FOREIGN KEY (origin_space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: cv_format_sessions cv_format_sessions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cv_format_sessions
    ADD CONSTRAINT cv_format_sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: cv_standards cv_standards_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cv_standards
    ADD CONSTRAINT cv_standards_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE RESTRICT;


--
-- Name: data_export_requests data_export_requests_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.data_export_requests
    ADD CONSTRAINT data_export_requests_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: device_tokens device_tokens_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_tokens
    ADD CONSTRAINT device_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: digest_editions digest_editions_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.digest_editions
    ADD CONSTRAINT digest_editions_reviewed_by_fkey FOREIGN KEY (reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: digest_editions digest_editions_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.digest_editions
    ADD CONSTRAINT digest_editions_space_id_fkey FOREIGN KEY (space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: discount_code_uses discount_code_uses_code_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.discount_code_uses
    ADD CONSTRAINT discount_code_uses_code_id_fkey FOREIGN KEY (code_id) REFERENCES public.discount_codes(id) ON DELETE CASCADE;


--
-- Name: discount_code_uses discount_code_uses_payment_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.discount_code_uses
    ADD CONSTRAINT discount_code_uses_payment_id_fkey FOREIGN KEY (payment_id) REFERENCES public.payments(id);


--
-- Name: discount_code_uses discount_code_uses_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.discount_code_uses
    ADD CONSTRAINT discount_code_uses_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: discount_codes discount_codes_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.discount_codes
    ADD CONSTRAINT discount_codes_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: dm_conversations dm_conversations_participant_1_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dm_conversations
    ADD CONSTRAINT dm_conversations_participant_1_id_fkey FOREIGN KEY (participant_1_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: dm_conversations dm_conversations_participant_2_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dm_conversations
    ADD CONSTRAINT dm_conversations_participant_2_id_fkey FOREIGN KEY (participant_2_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: dm_messages dm_messages_conversation_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dm_messages
    ADD CONSTRAINT dm_messages_conversation_id_fkey FOREIGN KEY (conversation_id) REFERENCES public.dm_conversations(id) ON DELETE CASCADE;


--
-- Name: dm_messages dm_messages_sender_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dm_messages
    ADD CONSTRAINT dm_messages_sender_id_fkey FOREIGN KEY (sender_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: dm_requests dm_requests_conversation_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dm_requests
    ADD CONSTRAINT dm_requests_conversation_id_fkey FOREIGN KEY (conversation_id) REFERENCES public.dm_conversations(id);


--
-- Name: dm_requests dm_requests_recipient_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dm_requests
    ADD CONSTRAINT dm_requests_recipient_id_fkey FOREIGN KEY (recipient_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: dm_requests dm_requests_requester_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dm_requests
    ADD CONSTRAINT dm_requests_requester_id_fkey FOREIGN KEY (requester_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: doc_pages doc_pages_last_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.doc_pages
    ADD CONSTRAINT doc_pages_last_reviewed_by_fkey FOREIGN KEY (last_reviewed_by) REFERENCES public.users(id);


--
-- Name: doc_pages doc_pages_parent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.doc_pages
    ADD CONSTRAINT doc_pages_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES public.doc_pages(id);


--
-- Name: eca_community_updates eca_community_updates_guide_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.eca_community_updates
    ADD CONSTRAINT eca_community_updates_guide_id_fkey FOREIGN KEY (guide_id) REFERENCES public.eca_guides(id) ON DELETE CASCADE;


--
-- Name: eca_community_updates eca_community_updates_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.eca_community_updates
    ADD CONSTRAINT eca_community_updates_reviewed_by_fkey FOREIGN KEY (reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: eca_community_updates eca_community_updates_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.eca_community_updates
    ADD CONSTRAINT eca_community_updates_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: eca_guides eca_guides_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.eca_guides
    ADD CONSTRAINT eca_guides_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE RESTRICT;


--
-- Name: eca_guides eca_guides_hitl_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.eca_guides
    ADD CONSTRAINT eca_guides_hitl_reviewed_by_fkey FOREIGN KEY (hitl_reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: eca_guides eca_guides_provider_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.eca_guides
    ADD CONSTRAINT eca_guides_provider_id_fkey FOREIGN KEY (provider_id) REFERENCES public.eca_providers(id) ON DELETE RESTRICT;


--
-- Name: editor_block_permissions editor_block_permissions_user_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.editor_block_permissions
    ADD CONSTRAINT editor_block_permissions_user_group_id_fkey FOREIGN KEY (user_group_id) REFERENCES public.user_groups(id) ON DELETE CASCADE;


--
-- Name: email_verifications email_verifications_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email_verifications
    ADD CONSTRAINT email_verifications_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: enrollments enrollments_payment_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.enrollments
    ADD CONSTRAINT enrollments_payment_id_fkey FOREIGN KEY (payment_id) REFERENCES public.payments(id) ON DELETE SET NULL;


--
-- Name: enrollments enrollments_track_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.enrollments
    ADD CONSTRAINT enrollments_track_id_fkey FOREIGN KEY (track_id) REFERENCES public.learning_tracks(id) ON DELETE CASCADE;


--
-- Name: enrollments enrollments_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.enrollments
    ADD CONSTRAINT enrollments_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: feature_flags feature_flags_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.feature_flags
    ADD CONSTRAINT feature_flags_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: feeds feeds_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.feeds
    ADD CONSTRAINT feeds_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: feeds feeds_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.feeds
    ADD CONSTRAINT feeds_space_id_fkey FOREIGN KEY (space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: feeds feeds_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.feeds
    ADD CONSTRAINT feeds_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: forum_threads fk_forum_threads_parent_reply; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_threads
    ADD CONSTRAINT fk_forum_threads_parent_reply FOREIGN KEY (parent_reply_id) REFERENCES public.forum_replies(id) ON DELETE SET NULL;


--
-- Name: follows follows_follower_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.follows
    ADD CONSTRAINT follows_follower_id_fkey FOREIGN KEY (follower_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: form_steps form_steps_form_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.form_steps
    ADD CONSTRAINT form_steps_form_id_fkey FOREIGN KEY (form_id) REFERENCES public.forms(id) ON DELETE CASCADE;


--
-- Name: form_submission_triggers form_submission_triggers_form_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.form_submission_triggers
    ADD CONSTRAINT form_submission_triggers_form_id_fkey FOREIGN KEY (form_id) REFERENCES public.forms(id) ON DELETE CASCADE;


--
-- Name: form_submissions form_submissions_form_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.form_submissions
    ADD CONSTRAINT form_submissions_form_id_fkey FOREIGN KEY (form_id) REFERENCES public.forms(id);


--
-- Name: form_submissions form_submissions_payment_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.form_submissions
    ADD CONSTRAINT form_submissions_payment_id_fkey FOREIGN KEY (payment_id) REFERENCES public.payments(id) ON DELETE SET NULL;


--
-- Name: form_submissions form_submissions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.form_submissions
    ADD CONSTRAINT form_submissions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: forms forms_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forms
    ADD CONSTRAINT forms_space_id_fkey FOREIGN KEY (space_id) REFERENCES public.spaces(id) ON DELETE CASCADE;


--
-- Name: forum_bookmarks forum_bookmarks_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_bookmarks
    ADD CONSTRAINT forum_bookmarks_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: forum_poll_options forum_poll_options_poll_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_poll_options
    ADD CONSTRAINT forum_poll_options_poll_id_fkey FOREIGN KEY (poll_id) REFERENCES public.forum_polls(id) ON DELETE CASCADE;


--
-- Name: forum_poll_votes forum_poll_votes_option_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_poll_votes
    ADD CONSTRAINT forum_poll_votes_option_id_fkey FOREIGN KEY (option_id) REFERENCES public.forum_poll_options(id) ON DELETE CASCADE;


--
-- Name: forum_poll_votes forum_poll_votes_poll_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_poll_votes
    ADD CONSTRAINT forum_poll_votes_poll_id_fkey FOREIGN KEY (poll_id) REFERENCES public.forum_polls(id) ON DELETE CASCADE;


--
-- Name: forum_poll_votes forum_poll_votes_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_poll_votes
    ADD CONSTRAINT forum_poll_votes_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: forum_polls forum_polls_thread_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_polls
    ADD CONSTRAINT forum_polls_thread_id_fkey FOREIGN KEY (thread_id) REFERENCES public.forum_threads(id) ON DELETE CASCADE;


--
-- Name: forum_replies forum_replies_author_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_replies
    ADD CONSTRAINT forum_replies_author_user_id_fkey FOREIGN KEY (author_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: forum_replies forum_replies_parent_reply_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_replies
    ADD CONSTRAINT forum_replies_parent_reply_id_fkey FOREIGN KEY (parent_reply_id) REFERENCES public.forum_replies(id) ON DELETE SET NULL;


--
-- Name: forum_replies forum_replies_thread_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_replies
    ADD CONSTRAINT forum_replies_thread_id_fkey FOREIGN KEY (thread_id) REFERENCES public.forum_threads(id) ON DELETE CASCADE;


--
-- Name: forum_tags forum_tags_cover_image_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_tags
    ADD CONSTRAINT forum_tags_cover_image_id_fkey FOREIGN KEY (cover_image_id) REFERENCES public.media(id) ON DELETE SET NULL;


--
-- Name: forum_tags forum_tags_moderator_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_tags
    ADD CONSTRAINT forum_tags_moderator_user_id_fkey FOREIGN KEY (moderator_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: forum_thread_tags forum_thread_tags_tag_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_thread_tags
    ADD CONSTRAINT forum_thread_tags_tag_id_fkey FOREIGN KEY (tag_id) REFERENCES public.forum_tags(id) ON DELETE CASCADE;


--
-- Name: forum_thread_tags forum_thread_tags_thread_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_thread_tags
    ADD CONSTRAINT forum_thread_tags_thread_id_fkey FOREIGN KEY (thread_id) REFERENCES public.forum_threads(id) ON DELETE CASCADE;


--
-- Name: forum_threads forum_threads_author_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_threads
    ADD CONSTRAINT forum_threads_author_user_id_fkey FOREIGN KEY (author_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: forum_threads forum_threads_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_threads
    ADD CONSTRAINT forum_threads_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE SET NULL;


--
-- Name: forum_threads forum_threads_solved_by_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_threads
    ADD CONSTRAINT forum_threads_solved_by_user_id_fkey FOREIGN KEY (solved_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: forum_threads forum_threads_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_threads
    ADD CONSTRAINT forum_threads_space_id_fkey FOREIGN KEY (space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: forum_votes forum_votes_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_votes
    ADD CONSTRAINT forum_votes_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: group_roles group_roles_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.group_roles
    ADD CONSTRAINT group_roles_group_id_fkey FOREIGN KEY (group_id) REFERENCES public.user_groups(id) ON DELETE CASCADE;


--
-- Name: group_roles group_roles_role_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.group_roles
    ADD CONSTRAINT group_roles_role_id_fkey FOREIGN KEY (role_id) REFERENCES public.user_roles(id) ON DELETE CASCADE;


--
-- Name: housing_guides housing_guides_city_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.housing_guides
    ADD CONSTRAINT housing_guides_city_space_id_fkey FOREIGN KEY (city_space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: housing_guides housing_guides_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.housing_guides
    ADD CONSTRAINT housing_guides_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE RESTRICT;


--
-- Name: housing_guides housing_guides_guide_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.housing_guides
    ADD CONSTRAINT housing_guides_guide_id_fkey FOREIGN KEY (guide_id) REFERENCES public.contents(id) ON DELETE SET NULL;


--
-- Name: housing_guides housing_guides_hitl_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.housing_guides
    ADD CONSTRAINT housing_guides_hitl_reviewed_by_fkey FOREIGN KEY (hitl_reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: housing_guides housing_guides_intel_record_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.housing_guides
    ADD CONSTRAINT housing_guides_intel_record_id_fkey FOREIGN KEY (intel_record_id) REFERENCES public.intel_records(id) ON DELETE SET NULL;


--
-- Name: instructor_payouts instructor_payouts_instructor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.instructor_payouts
    ADD CONSTRAINT instructor_payouts_instructor_id_fkey FOREIGN KEY (instructor_id) REFERENCES public.instructor_profiles(id) ON DELETE SET NULL;


--
-- Name: instructor_profiles instructor_profiles_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.instructor_profiles
    ADD CONSTRAINT instructor_profiles_reviewed_by_fkey FOREIGN KEY (reviewed_by) REFERENCES public.users(id);


--
-- Name: instructor_profiles instructor_profiles_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.instructor_profiles
    ADD CONSTRAINT instructor_profiles_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: intel_inbound_events intel_inbound_events_channel_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.intel_inbound_events
    ADD CONSTRAINT intel_inbound_events_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES public.intel_source_channels(id) ON DELETE CASCADE;


--
-- Name: intel_jobs intel_jobs_country_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.intel_jobs
    ADD CONSTRAINT intel_jobs_country_space_id_fkey FOREIGN KEY (country_space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: intel_jobs intel_jobs_guide_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.intel_jobs
    ADD CONSTRAINT intel_jobs_guide_id_fkey FOREIGN KEY (guide_id) REFERENCES public.contents(id) ON DELETE SET NULL;


--
-- Name: intel_jobs intel_jobs_intel_record_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.intel_jobs
    ADD CONSTRAINT intel_jobs_intel_record_id_fkey FOREIGN KEY (intel_record_id) REFERENCES public.intel_records(id) ON DELETE CASCADE;


--
-- Name: intel_raw_documents intel_raw_documents_channel_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.intel_raw_documents
    ADD CONSTRAINT intel_raw_documents_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES public.intel_source_channels(id) ON DELETE SET NULL;


--
-- Name: intel_raw_documents intel_raw_documents_parent_document_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.intel_raw_documents
    ADD CONSTRAINT intel_raw_documents_parent_document_id_fkey FOREIGN KEY (parent_document_id) REFERENCES public.intel_raw_documents(id) ON DELETE SET NULL;


--
-- Name: intel_raw_documents intel_raw_documents_source_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.intel_raw_documents
    ADD CONSTRAINT intel_raw_documents_source_id_fkey FOREIGN KEY (source_id) REFERENCES public.intel_sources(id) ON DELETE CASCADE;


--
-- Name: intel_records intel_records_raw_document_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.intel_records
    ADD CONSTRAINT intel_records_raw_document_id_fkey FOREIGN KEY (raw_document_id) REFERENCES public.intel_raw_documents(id) ON DELETE CASCADE;


--
-- Name: intel_records intel_records_source_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.intel_records
    ADD CONSTRAINT intel_records_source_id_fkey FOREIGN KEY (source_id) REFERENCES public.intel_sources(id) ON DELETE CASCADE;


--
-- Name: intel_review_log intel_review_log_actor_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.intel_review_log
    ADD CONSTRAINT intel_review_log_actor_user_id_fkey FOREIGN KEY (actor_user_id) REFERENCES public.users(id);


--
-- Name: intel_review_log intel_review_log_intel_record_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.intel_review_log
    ADD CONSTRAINT intel_review_log_intel_record_id_fkey FOREIGN KEY (intel_record_id) REFERENCES public.intel_records(id) ON DELETE CASCADE;


--
-- Name: intel_source_channels intel_source_channels_source_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.intel_source_channels
    ADD CONSTRAINT intel_source_channels_source_id_fkey FOREIGN KEY (source_id) REFERENCES public.intel_sources(id) ON DELETE CASCADE;


--
-- Name: intel_sources intel_sources_approved_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.intel_sources
    ADD CONSTRAINT intel_sources_approved_by_fkey FOREIGN KEY (approved_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: intel_sources intel_sources_persona_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.intel_sources
    ADD CONSTRAINT intel_sources_persona_id_fkey FOREIGN KEY (persona_id) REFERENCES public.ai_personas(id) ON DELETE SET NULL;


--
-- Name: intel_sources intel_sources_proposed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.intel_sources
    ADD CONSTRAINT intel_sources_proposed_by_fkey FOREIGN KEY (proposed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: ip_blocks ip_blocks_blocked_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ip_blocks
    ADD CONSTRAINT ip_blocks_blocked_by_fkey FOREIGN KEY (blocked_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: job_applications job_applications_applicant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.job_applications
    ADD CONSTRAINT job_applications_applicant_id_fkey FOREIGN KEY (applicant_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: job_applications job_applications_job_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.job_applications
    ADD CONSTRAINT job_applications_job_id_fkey FOREIGN KEY (job_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: job_applications job_applications_submission_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.job_applications
    ADD CONSTRAINT job_applications_submission_id_fkey FOREIGN KEY (submission_id) REFERENCES public.form_submissions(id) ON DELETE SET NULL;


--
-- Name: language_score_stories language_score_stories_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.language_score_stories
    ADD CONSTRAINT language_score_stories_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: language_score_stories language_score_stories_test_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.language_score_stories
    ADD CONSTRAINT language_score_stories_test_id_fkey FOREIGN KEY (test_id) REFERENCES public.language_tests(id) ON DELETE RESTRICT;


--
-- Name: language_score_stories language_score_stories_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.language_score_stories
    ADD CONSTRAINT language_score_stories_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: language_test_requirements language_test_requirements_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.language_test_requirements
    ADD CONSTRAINT language_test_requirements_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE RESTRICT;


--
-- Name: language_test_requirements language_test_requirements_guide_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.language_test_requirements
    ADD CONSTRAINT language_test_requirements_guide_id_fkey FOREIGN KEY (guide_id) REFERENCES public.contents(id) ON DELETE SET NULL;


--
-- Name: language_test_requirements language_test_requirements_hitl_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.language_test_requirements
    ADD CONSTRAINT language_test_requirements_hitl_reviewed_by_fkey FOREIGN KEY (hitl_reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: language_test_requirements language_test_requirements_intel_record_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.language_test_requirements
    ADD CONSTRAINT language_test_requirements_intel_record_id_fkey FOREIGN KEY (intel_record_id) REFERENCES public.intel_records(id) ON DELETE SET NULL;


--
-- Name: language_test_requirements language_test_requirements_test_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.language_test_requirements
    ADD CONSTRAINT language_test_requirements_test_id_fkey FOREIGN KEY (test_id) REFERENCES public.language_tests(id) ON DELETE RESTRICT;


--
-- Name: language_test_resources language_test_resources_test_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.language_test_resources
    ADD CONSTRAINT language_test_resources_test_id_fkey FOREIGN KEY (test_id) REFERENCES public.language_tests(id) ON DELETE CASCADE;


--
-- Name: learning_certificates learning_certificates_track_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.learning_certificates
    ADD CONSTRAINT learning_certificates_track_id_fkey FOREIGN KEY (track_id) REFERENCES public.learning_tracks(id);


--
-- Name: learning_certificates learning_certificates_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.learning_certificates
    ADD CONSTRAINT learning_certificates_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: learning_lessons learning_lessons_module_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.learning_lessons
    ADD CONSTRAINT learning_lessons_module_id_fkey FOREIGN KEY (module_id) REFERENCES public.learning_modules(id) ON DELETE CASCADE;


--
-- Name: learning_modules learning_modules_track_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.learning_modules
    ADD CONSTRAINT learning_modules_track_id_fkey FOREIGN KEY (track_id) REFERENCES public.learning_tracks(id) ON DELETE CASCADE;


--
-- Name: learning_tracks learning_tracks_geo_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.learning_tracks
    ADD CONSTRAINT learning_tracks_geo_space_id_fkey FOREIGN KEY (geo_space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: learning_tracks learning_tracks_instructor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.learning_tracks
    ADD CONSTRAINT learning_tracks_instructor_id_fkey FOREIGN KEY (instructor_id) REFERENCES public.instructor_profiles(id) ON DELETE SET NULL;


--
-- Name: lesson_progress lesson_progress_lesson_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.lesson_progress
    ADD CONSTRAINT lesson_progress_lesson_id_fkey FOREIGN KEY (lesson_id) REFERENCES public.learning_lessons(id) ON DELETE CASCADE;


--
-- Name: lesson_progress lesson_progress_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.lesson_progress
    ADD CONSTRAINT lesson_progress_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: lesson_quizzes lesson_quizzes_lesson_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.lesson_quizzes
    ADD CONSTRAINT lesson_quizzes_lesson_id_fkey FOREIGN KEY (lesson_id) REFERENCES public.learning_lessons(id) ON DELETE CASCADE;


--
-- Name: media media_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.media
    ADD CONSTRAINT media_reviewed_by_fkey FOREIGN KEY (reviewed_by) REFERENCES public.users(id);


--
-- Name: media media_uploader_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.media
    ADD CONSTRAINT media_uploader_user_id_fkey FOREIGN KEY (uploader_user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: media media_user_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.media
    ADD CONSTRAINT media_user_group_id_fkey FOREIGN KEY (user_group_id) REFERENCES public.user_groups(id) ON DELETE SET NULL;


--
-- Name: mentor_reviews mentor_reviews_reviewer_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mentor_reviews
    ADD CONSTRAINT mentor_reviews_reviewer_id_fkey FOREIGN KEY (reviewer_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: mentor_reviews mentor_reviews_session_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mentor_reviews
    ADD CONSTRAINT mentor_reviews_session_id_fkey FOREIGN KEY (session_id) REFERENCES public.mentor_sessions(id) ON DELETE CASCADE;


--
-- Name: mentor_sessions mentor_sessions_mentee_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mentor_sessions
    ADD CONSTRAINT mentor_sessions_mentee_id_fkey FOREIGN KEY (mentee_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: mentor_sessions mentor_sessions_mentor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mentor_sessions
    ADD CONSTRAINT mentor_sessions_mentor_id_fkey FOREIGN KEY (mentor_id) REFERENCES public.mentorship_profiles(id) ON DELETE CASCADE;


--
-- Name: mentorship_connections mentorship_connections_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mentorship_connections
    ADD CONSTRAINT mentorship_connections_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: mentorship_connections mentorship_connections_mentee_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mentorship_connections
    ADD CONSTRAINT mentorship_connections_mentee_id_fkey FOREIGN KEY (mentee_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: mentorship_connections mentorship_connections_mentor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mentorship_connections
    ADD CONSTRAINT mentorship_connections_mentor_id_fkey FOREIGN KEY (mentor_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: mentorship_matches mentorship_matches_mentee_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mentorship_matches
    ADD CONSTRAINT mentorship_matches_mentee_id_fkey FOREIGN KEY (mentee_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: mentorship_matches mentorship_matches_mentor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mentorship_matches
    ADD CONSTRAINT mentorship_matches_mentor_id_fkey FOREIGN KEY (mentor_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: mentorship_matches mentorship_matches_program_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mentorship_matches
    ADD CONSTRAINT mentorship_matches_program_id_fkey FOREIGN KEY (program_id) REFERENCES public.mentorship_programs(id) ON DELETE CASCADE;


--
-- Name: mentorship_profiles mentorship_profiles_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mentorship_profiles
    ADD CONSTRAINT mentorship_profiles_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: menu_items menu_items_menu_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.menu_items
    ADD CONSTRAINT menu_items_menu_id_fkey FOREIGN KEY (menu_id) REFERENCES public.menus(id) ON DELETE CASCADE;


--
-- Name: menu_items menu_items_parent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.menu_items
    ADD CONSTRAINT menu_items_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES public.menu_items(id) ON DELETE CASCADE;


--
-- Name: moderation_alerts moderation_alerts_acknowledged_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.moderation_alerts
    ADD CONSTRAINT moderation_alerts_acknowledged_by_fkey FOREIGN KEY (acknowledged_by) REFERENCES public.users(id);


--
-- Name: moderation_alerts moderation_alerts_resolved_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.moderation_alerts
    ADD CONSTRAINT moderation_alerts_resolved_by_fkey FOREIGN KEY (resolved_by) REFERENCES public.users(id);


--
-- Name: moderation_appeals moderation_appeals_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.moderation_appeals
    ADD CONSTRAINT moderation_appeals_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: monitored_embassies monitored_embassies_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.monitored_embassies
    ADD CONSTRAINT monitored_embassies_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE RESTRICT;


--
-- Name: mutual_aid_posts mutual_aid_posts_city_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mutual_aid_posts
    ADD CONSTRAINT mutual_aid_posts_city_space_id_fkey FOREIGN KEY (city_space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: mutual_aid_posts mutual_aid_posts_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mutual_aid_posts
    ADD CONSTRAINT mutual_aid_posts_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE CASCADE;


--
-- Name: mutual_aid_posts mutual_aid_posts_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mutual_aid_posts
    ADD CONSTRAINT mutual_aid_posts_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: newsletter_campaigns newsletter_campaigns_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.newsletter_campaigns
    ADD CONSTRAINT newsletter_campaigns_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: newsletter_list_subscribers newsletter_list_subscribers_list_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.newsletter_list_subscribers
    ADD CONSTRAINT newsletter_list_subscribers_list_id_fkey FOREIGN KEY (list_id) REFERENCES public.newsletter_lists(id) ON DELETE CASCADE;


--
-- Name: newsletter_list_subscribers newsletter_list_subscribers_subscriber_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.newsletter_list_subscribers
    ADD CONSTRAINT newsletter_list_subscribers_subscriber_id_fkey FOREIGN KEY (subscriber_id) REFERENCES public.newsletter_subscribers(id) ON DELETE CASCADE;


--
-- Name: newsletter_send_state newsletter_send_state_campaign_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.newsletter_send_state
    ADD CONSTRAINT newsletter_send_state_campaign_id_fkey FOREIGN KEY (campaign_id) REFERENCES public.newsletter_campaigns(id) ON DELETE CASCADE;


--
-- Name: newsletter_subscribers newsletter_subscribers_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.newsletter_subscribers
    ADD CONSTRAINT newsletter_subscribers_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: newsletter_unsubscribe_tokens newsletter_unsubscribe_tokens_subscriber_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.newsletter_unsubscribe_tokens
    ADD CONSTRAINT newsletter_unsubscribe_tokens_subscriber_id_fkey FOREIGN KEY (subscriber_id) REFERENCES public.newsletter_subscribers(id) ON DELETE CASCADE;


--
-- Name: notification_batch_windows notification_batch_windows_recipient_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notification_batch_windows
    ADD CONSTRAINT notification_batch_windows_recipient_id_fkey FOREIGN KEY (recipient_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: notification_preferences notification_preferences_notification_type_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notification_preferences
    ADD CONSTRAINT notification_preferences_notification_type_fkey FOREIGN KEY (notification_type) REFERENCES public.notification_types(key);


--
-- Name: notification_preferences notification_preferences_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notification_preferences
    ADD CONSTRAINT notification_preferences_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: notifications notifications_notification_type_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notifications
    ADD CONSTRAINT notifications_notification_type_fkey FOREIGN KEY (notification_type) REFERENCES public.notification_types(key);


--
-- Name: notifications notifications_recipient_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notifications
    ADD CONSTRAINT notifications_recipient_id_fkey FOREIGN KEY (recipient_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: oauth_accounts oauth_accounts_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.oauth_accounts
    ADD CONSTRAINT oauth_accounts_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: onboarding_state onboarding_state_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.onboarding_state
    ADD CONSTRAINT onboarding_state_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: organization_boosts organization_boosts_campaign_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organization_boosts
    ADD CONSTRAINT organization_boosts_campaign_id_fkey FOREIGN KEY (campaign_id) REFERENCES public.ad_campaigns(id) ON DELETE CASCADE;


--
-- Name: organization_boosts organization_boosts_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organization_boosts
    ADD CONSTRAINT organization_boosts_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;


--
-- Name: organization_invitations organization_invitations_invited_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organization_invitations
    ADD CONSTRAINT organization_invitations_invited_by_fkey FOREIGN KEY (invited_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: organization_invitations organization_invitations_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organization_invitations
    ADD CONSTRAINT organization_invitations_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;


--
-- Name: organization_members organization_members_invited_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organization_members
    ADD CONSTRAINT organization_members_invited_by_fkey FOREIGN KEY (invited_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: organization_members organization_members_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organization_members
    ADD CONSTRAINT organization_members_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;


--
-- Name: organization_members organization_members_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organization_members
    ADD CONSTRAINT organization_members_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: organizations organizations_geo_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organizations
    ADD CONSTRAINT organizations_geo_space_id_fkey FOREIGN KEY (geo_space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: organizations organizations_header_media_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organizations
    ADD CONSTRAINT organizations_header_media_id_fkey FOREIGN KEY (header_media_id) REFERENCES public.media(id) ON DELETE SET NULL;


--
-- Name: organizations organizations_logo_media_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organizations
    ADD CONSTRAINT organizations_logo_media_id_fkey FOREIGN KEY (logo_media_id) REFERENCES public.media(id) ON DELETE SET NULL;


--
-- Name: organizations organizations_owner_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organizations
    ADD CONSTRAINT organizations_owner_user_id_fkey FOREIGN KEY (owner_user_id) REFERENCES public.users(id);


--
-- Name: organizations organizations_source_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organizations
    ADD CONSTRAINT organizations_source_id_fkey FOREIGN KEY (source_id) REFERENCES public.intel_sources(id) ON DELETE SET NULL;


--
-- Name: ownership_claims ownership_claims_claimant_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ownership_claims
    ADD CONSTRAINT ownership_claims_claimant_user_id_fkey FOREIGN KEY (claimant_user_id) REFERENCES public.users(id);


--
-- Name: ownership_claims ownership_claims_form_submission_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ownership_claims
    ADD CONSTRAINT ownership_claims_form_submission_id_fkey FOREIGN KEY (form_submission_id) REFERENCES public.form_submissions(id) ON DELETE SET NULL;


--
-- Name: ownership_claims ownership_claims_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ownership_claims
    ADD CONSTRAINT ownership_claims_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.organizations(id);


--
-- Name: ownership_claims ownership_claims_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ownership_claims
    ADD CONSTRAINT ownership_claims_reviewed_by_fkey FOREIGN KEY (reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: page_builder_pages page_builder_pages_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_builder_pages
    ADD CONSTRAINT page_builder_pages_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: pagebuilder_templates pagebuilder_templates_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pagebuilder_templates
    ADD CONSTRAINT pagebuilder_templates_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: pagebuilder_templates pagebuilder_templates_thumbnail_media_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pagebuilder_templates
    ADD CONSTRAINT pagebuilder_templates_thumbnail_media_id_fkey FOREIGN KEY (thumbnail_media_id) REFERENCES public.media(id) ON DELETE SET NULL;


--
-- Name: pages pages_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pages
    ADD CONSTRAINT pages_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: pages pages_template_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pages
    ADD CONSTRAINT pages_template_id_fkey FOREIGN KEY (template_id) REFERENCES public.pagebuilder_templates(id);


--
-- Name: pages pages_theme_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pages
    ADD CONSTRAINT pages_theme_id_fkey FOREIGN KEY (theme_id) REFERENCES public.pagebuilder_themes(id);


--
-- Name: password_history password_history_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.password_history
    ADD CONSTRAINT password_history_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: password_reset_tokens password_reset_tokens_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.password_reset_tokens
    ADD CONSTRAINT password_reset_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: payment_refunds payment_refunds_payment_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payment_refunds
    ADD CONSTRAINT payment_refunds_payment_id_fkey FOREIGN KEY (payment_id) REFERENCES public.payments(id) ON DELETE CASCADE;


--
-- Name: payments payments_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payments
    ADD CONSTRAINT payments_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: persona_voices persona_voices_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.persona_voices
    ADD CONSTRAINT persona_voices_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: planning_group_checkins planning_group_checkins_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.planning_group_checkins
    ADD CONSTRAINT planning_group_checkins_group_id_fkey FOREIGN KEY (group_id) REFERENCES public.planning_groups(id) ON DELETE CASCADE;


--
-- Name: planning_group_checkins planning_group_checkins_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.planning_group_checkins
    ADD CONSTRAINT planning_group_checkins_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: planning_group_members planning_group_members_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.planning_group_members
    ADD CONSTRAINT planning_group_members_group_id_fkey FOREIGN KEY (group_id) REFERENCES public.planning_groups(id) ON DELETE CASCADE;


--
-- Name: planning_group_members planning_group_members_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.planning_group_members
    ADD CONSTRAINT planning_group_members_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: planning_group_milestones planning_group_milestones_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.planning_group_milestones
    ADD CONSTRAINT planning_group_milestones_group_id_fkey FOREIGN KEY (group_id) REFERENCES public.planning_groups(id) ON DELETE CASCADE;


--
-- Name: planning_group_milestones planning_group_milestones_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.planning_group_milestones
    ADD CONSTRAINT planning_group_milestones_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: planning_groups planning_groups_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.planning_groups
    ADD CONSTRAINT planning_groups_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE CASCADE;


--
-- Name: planning_groups planning_groups_owner_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.planning_groups
    ADD CONSTRAINT planning_groups_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: platform_data_exports platform_data_exports_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.platform_data_exports
    ADD CONSTRAINT platform_data_exports_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: platform_data_imports platform_data_imports_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.platform_data_imports
    ADD CONSTRAINT platform_data_imports_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: platform_events platform_events_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.platform_events
    ADD CONSTRAINT platform_events_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: pof_requirements pof_requirements_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pof_requirements
    ADD CONSTRAINT pof_requirements_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE RESTRICT;


--
-- Name: pof_requirements pof_requirements_hitl_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pof_requirements
    ADD CONSTRAINT pof_requirements_hitl_reviewed_by_fkey FOREIGN KEY (hitl_reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: professional_verifiers professional_verifiers_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.professional_verifiers
    ADD CONSTRAINT professional_verifiers_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: push_subscriptions push_subscriptions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.push_subscriptions
    ADD CONSTRAINT push_subscriptions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: push_tokens push_tokens_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.push_tokens
    ADD CONSTRAINT push_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: reactions reactions_reaction_key_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reactions
    ADD CONSTRAINT reactions_reaction_key_fkey FOREIGN KEY (reaction_key) REFERENCES public.reaction_types(key) ON DELETE CASCADE;


--
-- Name: reactions reactions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reactions
    ADD CONSTRAINT reactions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: referral_codes referral_codes_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.referral_codes
    ADD CONSTRAINT referral_codes_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: referrals referrals_qualified_payment_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.referrals
    ADD CONSTRAINT referrals_qualified_payment_id_fkey FOREIGN KEY (qualified_payment_id) REFERENCES public.payments(id) ON DELETE SET NULL;


--
-- Name: referrals referrals_referee_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.referrals
    ADD CONSTRAINT referrals_referee_id_fkey FOREIGN KEY (referee_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: referrals referrals_referrer_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.referrals
    ADD CONSTRAINT referrals_referrer_id_fkey FOREIGN KEY (referrer_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: refresh_tokens refresh_tokens_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.refresh_tokens
    ADD CONSTRAINT refresh_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: remittance_providers remittance_providers_logo_media_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remittance_providers
    ADD CONSTRAINT remittance_providers_logo_media_id_fkey FOREIGN KEY (logo_media_id) REFERENCES public.media(id);


--
-- Name: remittance_rate_alerts remittance_rate_alerts_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remittance_rate_alerts
    ADD CONSTRAINT remittance_rate_alerts_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: remittance_rates remittance_rates_intel_record_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remittance_rates
    ADD CONSTRAINT remittance_rates_intel_record_id_fkey FOREIGN KEY (intel_record_id) REFERENCES public.intel_records(id) ON DELETE SET NULL;


--
-- Name: remittance_rates remittance_rates_provider_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remittance_rates
    ADD CONSTRAINT remittance_rates_provider_id_fkey FOREIGN KEY (provider_id) REFERENCES public.remittance_providers(id) ON DELETE CASCADE;


--
-- Name: reply_views reply_views_reply_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reply_views
    ADD CONSTRAINT reply_views_reply_id_fkey FOREIGN KEY (reply_id) REFERENCES public.forum_replies(id) ON DELETE CASCADE;


--
-- Name: reply_views reply_views_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reply_views
    ADD CONSTRAINT reply_views_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: revenue_splits revenue_splits_enrollment_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.revenue_splits
    ADD CONSTRAINT revenue_splits_enrollment_id_fkey FOREIGN KEY (enrollment_id) REFERENCES public.enrollments(id) ON DELETE SET NULL;


--
-- Name: revenue_splits revenue_splits_instructor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.revenue_splits
    ADD CONSTRAINT revenue_splits_instructor_id_fkey FOREIGN KEY (instructor_id) REFERENCES public.instructor_profiles(id) ON DELETE SET NULL;


--
-- Name: revenue_splits revenue_splits_payment_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.revenue_splits
    ADD CONSTRAINT revenue_splits_payment_id_fkey FOREIGN KEY (payment_id) REFERENCES public.payments(id) ON DELETE SET NULL;


--
-- Name: revenue_splits revenue_splits_payout_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.revenue_splits
    ADD CONSTRAINT revenue_splits_payout_id_fkey FOREIGN KEY (payout_id) REFERENCES public.instructor_payouts(id) ON DELETE SET NULL;


--
-- Name: revenue_splits revenue_splits_track_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.revenue_splits
    ADD CONSTRAINT revenue_splits_track_id_fkey FOREIGN KEY (track_id) REFERENCES public.learning_tracks(id) ON DELETE CASCADE;


--
-- Name: reviews reviews_removed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reviews
    ADD CONSTRAINT reviews_removed_by_fkey FOREIGN KEY (removed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: reviews reviews_reviewer_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reviews
    ADD CONSTRAINT reviews_reviewer_user_id_fkey FOREIGN KEY (reviewer_user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: role_permissions role_permissions_permission_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT role_permissions_permission_id_fkey FOREIGN KEY (permission_id) REFERENCES public.permissions(id) ON DELETE CASCADE;


--
-- Name: role_permissions role_permissions_role_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT role_permissions_role_id_fkey FOREIGN KEY (role_id) REFERENCES public.user_roles(id) ON DELETE CASCADE;


--
-- Name: saved_scholarships saved_scholarships_guide_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.saved_scholarships
    ADD CONSTRAINT saved_scholarships_guide_id_fkey FOREIGN KEY (guide_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: saved_scholarships saved_scholarships_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.saved_scholarships
    ADD CONSTRAINT saved_scholarships_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: scam_alert_subscriptions scam_alert_subscriptions_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scam_alert_subscriptions
    ADD CONSTRAINT scam_alert_subscriptions_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE CASCADE;


--
-- Name: scam_alert_subscriptions scam_alert_subscriptions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scam_alert_subscriptions
    ADD CONSTRAINT scam_alert_subscriptions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: scam_reports scam_reports_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scam_reports
    ADD CONSTRAINT scam_reports_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE CASCADE;


--
-- Name: scam_reports scam_reports_hitl_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scam_reports
    ADD CONSTRAINT scam_reports_hitl_reviewed_by_fkey FOREIGN KEY (hitl_reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: scam_reports scam_reports_origin_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scam_reports
    ADD CONSTRAINT scam_reports_origin_space_id_fkey FOREIGN KEY (origin_space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: scam_reports scam_reports_pinned_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scam_reports
    ADD CONSTRAINT scam_reports_pinned_space_id_fkey FOREIGN KEY (pinned_space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: scam_reports scam_reports_reporter_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scam_reports
    ADD CONSTRAINT scam_reports_reporter_id_fkey FOREIGN KEY (reporter_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: scam_reports scam_reports_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scam_reports
    ADD CONSTRAINT scam_reports_reviewed_by_fkey FOREIGN KEY (reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: scholarships scholarships_country_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scholarships
    ADD CONSTRAINT scholarships_country_space_id_fkey FOREIGN KEY (country_space_id) REFERENCES public.spaces(id);


--
-- Name: scholarships scholarships_guide_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scholarships
    ADD CONSTRAINT scholarships_guide_id_fkey FOREIGN KEY (guide_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: scholarships scholarships_intel_record_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scholarships
    ADD CONSTRAINT scholarships_intel_record_id_fkey FOREIGN KEY (intel_record_id) REFERENCES public.intel_records(id) ON DELETE CASCADE;


--
-- Name: search_analytics search_analytics_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.search_analytics
    ADD CONSTRAINT search_analytics_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: service_listing_meta service_listing_meta_city_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.service_listing_meta
    ADD CONSTRAINT service_listing_meta_city_space_id_fkey FOREIGN KEY (city_space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: service_listing_meta service_listing_meta_claimed_by_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.service_listing_meta
    ADD CONSTRAINT service_listing_meta_claimed_by_org_id_fkey FOREIGN KEY (claimed_by_org_id) REFERENCES public.organizations(id) ON DELETE SET NULL;


--
-- Name: service_listing_meta service_listing_meta_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.service_listing_meta
    ADD CONSTRAINT service_listing_meta_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: service_listing_meta service_listing_meta_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.service_listing_meta
    ADD CONSTRAINT service_listing_meta_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: service_listing_meta service_listing_meta_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.service_listing_meta
    ADD CONSTRAINT service_listing_meta_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.organizations(id) ON DELETE SET NULL;


--
-- Name: service_listing_meta service_listing_meta_verified_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.service_listing_meta
    ADD CONSTRAINT service_listing_meta_verified_by_fkey FOREIGN KEY (verified_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: sidebar_widgets sidebar_blocks_sidebar_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sidebar_widgets
    ADD CONSTRAINT sidebar_blocks_sidebar_id_fkey FOREIGN KEY (sidebar_id) REFERENCES public.sidebars(id) ON DELETE CASCADE;


--
-- Name: sop_drafts sop_drafts_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sop_drafts
    ADD CONSTRAINT sop_drafts_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: sop_drafts sop_drafts_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sop_drafts
    ADD CONSTRAINT sop_drafts_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: sop_samples sop_samples_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sop_samples
    ADD CONSTRAINT sop_samples_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE RESTRICT;


--
-- Name: sop_samples sop_samples_hitl_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sop_samples
    ADD CONSTRAINT sop_samples_hitl_reviewed_by_fkey FOREIGN KEY (hitl_reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: sop_templates sop_templates_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sop_templates
    ADD CONSTRAINT sop_templates_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE RESTRICT;


--
-- Name: sop_templates sop_templates_hitl_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sop_templates
    ADD CONSTRAINT sop_templates_hitl_reviewed_by_fkey FOREIGN KEY (hitl_reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: source_collection_items source_collection_items_collection_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.source_collection_items
    ADD CONSTRAINT source_collection_items_collection_id_fkey FOREIGN KEY (collection_id) REFERENCES public.source_collections(id) ON DELETE CASCADE;


--
-- Name: source_collection_items source_collection_items_source_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.source_collection_items
    ADD CONSTRAINT source_collection_items_source_id_fkey FOREIGN KEY (source_id) REFERENCES public.intel_sources(id) ON DELETE CASCADE;


--
-- Name: space_members space_members_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_members
    ADD CONSTRAINT space_members_space_id_fkey FOREIGN KEY (space_id) REFERENCES public.spaces(id) ON DELETE CASCADE;


--
-- Name: space_members space_members_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_members
    ADD CONSTRAINT space_members_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: space_moderators space_moderators_assigned_by_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_moderators
    ADD CONSTRAINT space_moderators_assigned_by_user_id_fkey FOREIGN KEY (assigned_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: space_moderators space_moderators_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_moderators
    ADD CONSTRAINT space_moderators_space_id_fkey FOREIGN KEY (space_id) REFERENCES public.spaces(id) ON DELETE CASCADE;


--
-- Name: space_moderators space_moderators_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_moderators
    ADD CONSTRAINT space_moderators_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: space_mutes space_mutes_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_mutes
    ADD CONSTRAINT space_mutes_space_id_fkey FOREIGN KEY (space_id) REFERENCES public.spaces(id) ON DELETE CASCADE;


--
-- Name: space_mutes space_mutes_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_mutes
    ADD CONSTRAINT space_mutes_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: space_parent_spaces space_parent_spaces_parent_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_parent_spaces
    ADD CONSTRAINT space_parent_spaces_parent_space_id_fkey FOREIGN KEY (parent_space_id) REFERENCES public.spaces(id) ON DELETE CASCADE;


--
-- Name: space_parent_spaces space_parent_spaces_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_parent_spaces
    ADD CONSTRAINT space_parent_spaces_space_id_fkey FOREIGN KEY (space_id) REFERENCES public.spaces(id) ON DELETE CASCADE;


--
-- Name: space_pinned_posts space_pinned_posts_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_pinned_posts
    ADD CONSTRAINT space_pinned_posts_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: space_pinned_posts space_pinned_posts_pinned_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_pinned_posts
    ADD CONSTRAINT space_pinned_posts_pinned_by_fkey FOREIGN KEY (pinned_by) REFERENCES public.users(id);


--
-- Name: space_pinned_posts space_pinned_posts_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_pinned_posts
    ADD CONSTRAINT space_pinned_posts_space_id_fkey FOREIGN KEY (space_id) REFERENCES public.spaces(id) ON DELETE CASCADE;


--
-- Name: space_rules space_rules_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_rules
    ADD CONSTRAINT space_rules_space_id_fkey FOREIGN KEY (space_id) REFERENCES public.spaces(id) ON DELETE CASCADE;


--
-- Name: space_types space_types_field_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_types
    ADD CONSTRAINT space_types_field_group_id_fkey FOREIGN KEY (field_group_id) REFERENCES public.custom_field_groups(id) ON DELETE SET NULL;


--
-- Name: spaces spaces_creator_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.spaces
    ADD CONSTRAINT spaces_creator_user_id_fkey FOREIGN KEY (creator_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: spaces spaces_currency_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.spaces
    ADD CONSTRAINT spaces_currency_code_fkey FOREIGN KEY (currency_code) REFERENCES public.currencies(code) ON DELETE SET NULL;


--
-- Name: spaces spaces_parent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.spaces
    ADD CONSTRAINT spaces_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES public.spaces(id) ON DELETE CASCADE;


--
-- Name: spaces spaces_source_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.spaces
    ADD CONSTRAINT spaces_source_id_fkey FOREIGN KEY (source_id) REFERENCES public.intel_sources(id) ON DELETE SET NULL;


--
-- Name: spaces spaces_space_type_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.spaces
    ADD CONSTRAINT spaces_space_type_id_fkey FOREIGN KEY (space_type_id) REFERENCES public.space_types(id);


--
-- Name: story_meta story_meta_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.story_meta
    ADD CONSTRAINT story_meta_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: story_meta story_meta_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.story_meta
    ADD CONSTRAINT story_meta_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE RESTRICT;


--
-- Name: story_meta story_meta_hitl_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.story_meta
    ADD CONSTRAINT story_meta_hitl_reviewed_by_fkey FOREIGN KEY (hitl_reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: story_meta story_meta_origin_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.story_meta
    ADD CONSTRAINT story_meta_origin_space_id_fkey FOREIGN KEY (origin_space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: studio_batches studio_batches_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.studio_batches
    ADD CONSTRAINT studio_batches_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: studio_jobs studio_jobs_batch_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.studio_jobs
    ADD CONSTRAINT studio_jobs_batch_id_fkey FOREIGN KEY (batch_id) REFERENCES public.studio_batches(id);


--
-- Name: studio_jobs studio_jobs_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.studio_jobs
    ADD CONSTRAINT studio_jobs_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: studio_jobs studio_jobs_hitl_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.studio_jobs
    ADD CONSTRAINT studio_jobs_hitl_reviewed_by_fkey FOREIGN KEY (hitl_reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: studio_jobs studio_jobs_intel_record_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.studio_jobs
    ADD CONSTRAINT studio_jobs_intel_record_id_fkey FOREIGN KEY (intel_record_id) REFERENCES public.intel_records(id) ON DELETE SET NULL;


--
-- Name: studio_jobs studio_jobs_persona_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.studio_jobs
    ADD CONSTRAINT studio_jobs_persona_id_fkey FOREIGN KEY (persona_id) REFERENCES public.ai_personas(id);


--
-- Name: studio_jobs studio_jobs_target_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.studio_jobs
    ADD CONSTRAINT studio_jobs_target_space_id_fkey FOREIGN KEY (target_space_id) REFERENCES public.spaces(id);


--
-- Name: studio_jobs studio_jobs_voice_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.studio_jobs
    ADD CONSTRAINT studio_jobs_voice_id_fkey FOREIGN KEY (voice_id) REFERENCES public.persona_voices(id) ON DELETE SET NULL;


--
-- Name: subscription_plan_tracks subscription_plan_tracks_plan_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscription_plan_tracks
    ADD CONSTRAINT subscription_plan_tracks_plan_id_fkey FOREIGN KEY (plan_id) REFERENCES public.subscription_plans(id) ON DELETE CASCADE;


--
-- Name: subscription_plan_tracks subscription_plan_tracks_track_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscription_plan_tracks
    ADD CONSTRAINT subscription_plan_tracks_track_id_fkey FOREIGN KEY (track_id) REFERENCES public.learning_tracks(id) ON DELETE CASCADE;


--
-- Name: subscriptions subscriptions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscriptions
    ADD CONSTRAINT subscriptions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: tag_moderators tag_moderators_assigned_by_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tag_moderators
    ADD CONSTRAINT tag_moderators_assigned_by_user_id_fkey FOREIGN KEY (assigned_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: tag_moderators tag_moderators_tag_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tag_moderators
    ADD CONSTRAINT tag_moderators_tag_id_fkey FOREIGN KEY (tag_id) REFERENCES public.tags(id) ON DELETE CASCADE;


--
-- Name: tag_moderators tag_moderators_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tag_moderators
    ADD CONSTRAINT tag_moderators_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: tags tags_cover_media_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tags
    ADD CONSTRAINT tags_cover_media_id_fkey FOREIGN KEY (cover_media_id) REFERENCES public.media(id) ON DELETE SET NULL;


--
-- Name: tags tags_logo_media_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tags
    ADD CONSTRAINT tags_logo_media_id_fkey FOREIGN KEY (logo_media_id) REFERENCES public.media(id) ON DELETE SET NULL;


--
-- Name: tax_obligation_guides tax_obligation_guides_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tax_obligation_guides
    ADD CONSTRAINT tax_obligation_guides_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE RESTRICT;


--
-- Name: tax_obligation_guides tax_obligation_guides_guide_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tax_obligation_guides
    ADD CONSTRAINT tax_obligation_guides_guide_id_fkey FOREIGN KEY (guide_id) REFERENCES public.contents(id) ON DELETE SET NULL;


--
-- Name: tax_obligation_guides tax_obligation_guides_hitl_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tax_obligation_guides
    ADD CONSTRAINT tax_obligation_guides_hitl_reviewed_by_fkey FOREIGN KEY (hitl_reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: tax_obligation_guides tax_obligation_guides_intel_record_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tax_obligation_guides
    ADD CONSTRAINT tax_obligation_guides_intel_record_id_fkey FOREIGN KEY (intel_record_id) REFERENCES public.intel_records(id) ON DELETE SET NULL;


--
-- Name: tax_obligation_guides tax_obligation_guides_origin_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tax_obligation_guides
    ADD CONSTRAINT tax_obligation_guides_origin_space_id_fkey FOREIGN KEY (origin_space_id) REFERENCES public.spaces(id) ON DELETE RESTRICT;


--
-- Name: tax_treaty_pairs tax_treaty_pairs_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tax_treaty_pairs
    ADD CONSTRAINT tax_treaty_pairs_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE RESTRICT;


--
-- Name: tax_treaty_pairs tax_treaty_pairs_origin_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tax_treaty_pairs
    ADD CONSTRAINT tax_treaty_pairs_origin_space_id_fkey FOREIGN KEY (origin_space_id) REFERENCES public.spaces(id) ON DELETE RESTRICT;


--
-- Name: ticket_attachments ticket_attachments_comment_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_attachments
    ADD CONSTRAINT ticket_attachments_comment_id_fkey FOREIGN KEY (comment_id) REFERENCES public.ticket_comments(id);


--
-- Name: ticket_attachments ticket_attachments_media_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_attachments
    ADD CONSTRAINT ticket_attachments_media_id_fkey FOREIGN KEY (media_id) REFERENCES public.media(id);


--
-- Name: ticket_attachments ticket_attachments_ticket_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_attachments
    ADD CONSTRAINT ticket_attachments_ticket_id_fkey FOREIGN KEY (ticket_id) REFERENCES public.tickets(id) ON DELETE CASCADE;


--
-- Name: ticket_comments ticket_comments_ticket_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_comments
    ADD CONSTRAINT ticket_comments_ticket_id_fkey FOREIGN KEY (ticket_id) REFERENCES public.tickets(id) ON DELETE CASCADE;


--
-- Name: ticket_comments ticket_comments_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_comments
    ADD CONSTRAINT ticket_comments_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: ticket_events ticket_events_ticket_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_events
    ADD CONSTRAINT ticket_events_ticket_id_fkey FOREIGN KEY (ticket_id) REFERENCES public.tickets(id) ON DELETE CASCADE;


--
-- Name: ticket_events ticket_events_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_events
    ADD CONSTRAINT ticket_events_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: ticket_types ticket_types_auto_assign_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_types
    ADD CONSTRAINT ticket_types_auto_assign_group_id_fkey FOREIGN KEY (auto_assign_group_id) REFERENCES public.user_groups(id);


--
-- Name: ticket_types ticket_types_field_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_types
    ADD CONSTRAINT ticket_types_field_group_id_fkey FOREIGN KEY (field_group_id) REFERENCES public.custom_field_groups(id);


--
-- Name: tickets tickets_assigned_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tickets
    ADD CONSTRAINT tickets_assigned_group_id_fkey FOREIGN KEY (assigned_group_id) REFERENCES public.user_groups(id);


--
-- Name: tickets tickets_assigned_to_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tickets
    ADD CONSTRAINT tickets_assigned_to_fkey FOREIGN KEY (assigned_to) REFERENCES public.users(id);


--
-- Name: tickets tickets_submitter_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tickets
    ADD CONSTRAINT tickets_submitter_user_id_fkey FOREIGN KEY (submitter_user_id) REFERENCES public.users(id);


--
-- Name: tickets tickets_type_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tickets
    ADD CONSTRAINT tickets_type_id_fkey FOREIGN KEY (type_id) REFERENCES public.ticket_types(id);


--
-- Name: totp_credentials totp_credentials_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.totp_credentials
    ADD CONSTRAINT totp_credentials_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: university_course_offerings university_course_offerings_course_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.university_course_offerings
    ADD CONSTRAINT university_course_offerings_course_id_fkey FOREIGN KEY (course_id) REFERENCES public.university_course_catalog(id) ON DELETE CASCADE;


--
-- Name: university_course_offerings university_course_offerings_university_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.university_course_offerings
    ADD CONSTRAINT university_course_offerings_university_id_fkey FOREIGN KEY (university_id) REFERENCES public.spaces(id) ON DELETE CASCADE;


--
-- Name: user_affiliate_links user_affiliate_links_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_affiliate_links
    ADD CONSTRAINT user_affiliate_links_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_badges user_badges_awarded_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_badges
    ADD CONSTRAINT user_badges_awarded_by_fkey FOREIGN KEY (awarded_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: user_badges user_badges_badge_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_badges
    ADD CONSTRAINT user_badges_badge_id_fkey FOREIGN KEY (badge_id) REFERENCES public.badge_definitions(id) ON DELETE CASCADE;


--
-- Name: user_badges user_badges_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_badges
    ADD CONSTRAINT user_badges_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_blocks user_blocks_blocked_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_blocks
    ADD CONSTRAINT user_blocks_blocked_id_fkey FOREIGN KEY (blocked_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_blocks user_blocks_blocker_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_blocks
    ADD CONSTRAINT user_blocks_blocker_id_fkey FOREIGN KEY (blocker_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_challenge_progress user_challenge_progress_challenge_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_challenge_progress
    ADD CONSTRAINT user_challenge_progress_challenge_id_fkey FOREIGN KEY (challenge_id) REFERENCES public.challenge_definitions(id) ON DELETE CASCADE;


--
-- Name: user_challenge_progress user_challenge_progress_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_challenge_progress
    ADD CONSTRAINT user_challenge_progress_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_content_views user_content_views_content_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_content_views
    ADD CONSTRAINT user_content_views_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE CASCADE;


--
-- Name: user_content_views user_content_views_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_content_views
    ADD CONSTRAINT user_content_views_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_dm_settings user_dm_settings_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_dm_settings
    ADD CONSTRAINT user_dm_settings_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_group_memberships user_group_memberships_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_group_memberships
    ADD CONSTRAINT user_group_memberships_group_id_fkey FOREIGN KEY (group_id) REFERENCES public.user_groups(id) ON DELETE CASCADE;


--
-- Name: user_group_memberships user_group_memberships_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_group_memberships
    ADD CONSTRAINT user_group_memberships_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_group_promotions user_group_promotions_source_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_group_promotions
    ADD CONSTRAINT user_group_promotions_source_group_id_fkey FOREIGN KEY (source_group_id) REFERENCES public.user_groups(id) ON DELETE CASCADE;


--
-- Name: user_group_promotions user_group_promotions_target_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_group_promotions
    ADD CONSTRAINT user_group_promotions_target_group_id_fkey FOREIGN KEY (target_group_id) REFERENCES public.user_groups(id) ON DELETE CASCADE;


--
-- Name: user_moderation_actions user_moderation_actions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_moderation_actions
    ADD CONSTRAINT user_moderation_actions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_mutes user_mutes_muted_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_mutes
    ADD CONSTRAINT user_mutes_muted_user_id_fkey FOREIGN KEY (muted_user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_mutes user_mutes_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_mutes
    ADD CONSTRAINT user_mutes_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_operation_bans user_operation_bans_issued_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_operation_bans
    ADD CONSTRAINT user_operation_bans_issued_by_fkey FOREIGN KEY (issued_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: user_operation_bans user_operation_bans_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_operation_bans
    ADD CONSTRAINT user_operation_bans_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_preferences user_preferences_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_preferences
    ADD CONSTRAINT user_preferences_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_privacy_settings user_privacy_settings_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_privacy_settings
    ADD CONSTRAINT user_privacy_settings_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_profiles user_profiles_avatar_media_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_profiles
    ADD CONSTRAINT user_profiles_avatar_media_id_fkey FOREIGN KEY (avatar_media_id) REFERENCES public.media(id) ON DELETE SET NULL;


--
-- Name: user_profiles user_profiles_banner_media_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_profiles
    ADD CONSTRAINT user_profiles_banner_media_id_fkey FOREIGN KEY (banner_media_id) REFERENCES public.media(id) ON DELETE SET NULL;


--
-- Name: user_profiles user_profiles_header_media_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_profiles
    ADD CONSTRAINT user_profiles_header_media_id_fkey FOREIGN KEY (header_media_id) REFERENCES public.media(id) ON DELETE SET NULL;


--
-- Name: user_profiles user_profiles_japa_stage_dest_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_profiles
    ADD CONSTRAINT user_profiles_japa_stage_dest_space_id_fkey FOREIGN KEY (japa_stage_dest_space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: user_profiles user_profiles_preferred_currency_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_profiles
    ADD CONSTRAINT user_profiles_preferred_currency_fkey FOREIGN KEY (preferred_currency) REFERENCES public.currencies(code) ON DELETE SET NULL;


--
-- Name: user_profiles user_profiles_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_profiles
    ADD CONSTRAINT user_profiles_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_source_blocks user_source_blocks_blocker_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_source_blocks
    ADD CONSTRAINT user_source_blocks_blocker_id_fkey FOREIGN KEY (blocker_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_source_blocks user_source_blocks_source_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_source_blocks
    ADD CONSTRAINT user_source_blocks_source_id_fkey FOREIGN KEY (source_id) REFERENCES public.intel_sources(id) ON DELETE CASCADE;


--
-- Name: users users_current_geo_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_current_geo_space_id_fkey FOREIGN KEY (current_geo_space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: users users_source_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_source_id_fkey FOREIGN KEY (source_id) REFERENCES public.intel_sources(id) ON DELETE SET NULL;


--
-- Name: video_channel_claims video_channel_claims_claimant_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.video_channel_claims
    ADD CONSTRAINT video_channel_claims_claimant_user_id_fkey FOREIGN KEY (claimant_user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: video_channel_claims video_channel_claims_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.video_channel_claims
    ADD CONSTRAINT video_channel_claims_reviewed_by_fkey FOREIGN KEY (reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: video_channel_claims video_channel_claims_video_author_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.video_channel_claims
    ADD CONSTRAINT video_channel_claims_video_author_user_id_fkey FOREIGN KEY (video_author_user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: video_channel_claims video_channel_claims_video_source_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.video_channel_claims
    ADD CONSTRAINT video_channel_claims_video_source_id_fkey FOREIGN KEY (video_source_id) REFERENCES public.video_sources(id) ON DELETE SET NULL;


--
-- Name: video_import_jobs video_import_jobs_target_author_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.video_import_jobs
    ADD CONSTRAINT video_import_jobs_target_author_user_id_fkey FOREIGN KEY (target_author_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: video_import_jobs video_import_jobs_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.video_import_jobs
    ADD CONSTRAINT video_import_jobs_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: video_sources video_sources_author_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.video_sources
    ADD CONSTRAINT video_sources_author_user_id_fkey FOREIGN KEY (author_user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: video_sources video_sources_source_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.video_sources
    ADD CONSTRAINT video_sources_source_id_fkey FOREIGN KEY (source_id) REFERENCES public.intel_sources(id) ON DELETE SET NULL;


--
-- Name: video_sources video_sources_thumbnail_media_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.video_sources
    ADD CONSTRAINT video_sources_thumbnail_media_id_fkey FOREIGN KEY (thumbnail_media_id) REFERENCES public.media(id) ON DELETE SET NULL;


--
-- Name: visa_processing_stats visa_processing_stats_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.visa_processing_stats
    ADD CONSTRAINT visa_processing_stats_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE CASCADE;


--
-- Name: visa_processing_stats visa_processing_stats_origin_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.visa_processing_stats
    ADD CONSTRAINT visa_processing_stats_origin_space_id_fkey FOREIGN KEY (origin_space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: visa_routes visa_routes_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.visa_routes
    ADD CONSTRAINT visa_routes_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE RESTRICT;


--
-- Name: visa_routes visa_routes_guide_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.visa_routes
    ADD CONSTRAINT visa_routes_guide_id_fkey FOREIGN KEY (guide_id) REFERENCES public.contents(id) ON DELETE SET NULL;


--
-- Name: visa_routes visa_routes_hitl_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.visa_routes
    ADD CONSTRAINT visa_routes_hitl_reviewed_by_fkey FOREIGN KEY (hitl_reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: visa_routes visa_routes_intel_record_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.visa_routes
    ADD CONSTRAINT visa_routes_intel_record_id_fkey FOREIGN KEY (intel_record_id) REFERENCES public.intel_records(id) ON DELETE SET NULL;


--
-- Name: visa_timeline_milestones visa_timeline_milestones_timeline_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.visa_timeline_milestones
    ADD CONSTRAINT visa_timeline_milestones_timeline_id_fkey FOREIGN KEY (timeline_id) REFERENCES public.visa_timelines(id) ON DELETE CASCADE;


--
-- Name: visa_timelines visa_timelines_destination_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.visa_timelines
    ADD CONSTRAINT visa_timelines_destination_space_id_fkey FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE CASCADE;


--
-- Name: visa_timelines visa_timelines_origin_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.visa_timelines
    ADD CONSTRAINT visa_timelines_origin_space_id_fkey FOREIGN KEY (origin_space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;


--
-- Name: visa_timelines visa_timelines_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.visa_timelines
    ADD CONSTRAINT visa_timelines_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: wallet_ledger wallet_ledger_actor_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wallet_ledger
    ADD CONSTRAINT wallet_ledger_actor_user_id_fkey FOREIGN KEY (actor_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: wallet_ledger wallet_ledger_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wallet_ledger
    ADD CONSTRAINT wallet_ledger_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: wallet_withdrawal_methods wallet_withdrawal_methods_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wallet_withdrawal_methods
    ADD CONSTRAINT wallet_withdrawal_methods_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: wallet_withdrawals wallet_withdrawals_currency_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wallet_withdrawals
    ADD CONSTRAINT wallet_withdrawals_currency_code_fkey FOREIGN KEY (currency_code) REFERENCES public.currencies(code);


--
-- Name: wallet_withdrawals wallet_withdrawals_payout_method_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wallet_withdrawals
    ADD CONSTRAINT wallet_withdrawals_payout_method_id_fkey FOREIGN KEY (payout_method_id) REFERENCES public.wallet_withdrawal_methods(id);


--
-- Name: wallet_withdrawals wallet_withdrawals_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wallet_withdrawals
    ADD CONSTRAINT wallet_withdrawals_reviewed_by_fkey FOREIGN KEY (reviewed_by) REFERENCES public.users(id);


--
-- Name: wallet_withdrawals wallet_withdrawals_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wallet_withdrawals
    ADD CONSTRAINT wallet_withdrawals_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: webhook_deliveries webhook_deliveries_endpoint_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webhook_deliveries
    ADD CONSTRAINT webhook_deliveries_endpoint_id_fkey FOREIGN KEY (endpoint_id) REFERENCES public.webhook_endpoints(id) ON DELETE CASCADE;


--
-- Name: webhook_endpoints webhook_endpoints_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webhook_endpoints
    ADD CONSTRAINT webhook_endpoints_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: wiki_pages wiki_pages_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wiki_pages
    ADD CONSTRAINT wiki_pages_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: wiki_pages wiki_pages_guide_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wiki_pages
    ADD CONSTRAINT wiki_pages_guide_id_fkey FOREIGN KEY (guide_id) REFERENCES public.contents(id) ON DELETE SET NULL;


--
-- Name: wiki_pages wiki_pages_intel_record_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wiki_pages
    ADD CONSTRAINT wiki_pages_intel_record_id_fkey FOREIGN KEY (intel_record_id) REFERENCES public.intel_records(id) ON DELETE SET NULL;


--
-- Name: wiki_pages wiki_pages_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wiki_pages
    ADD CONSTRAINT wiki_pages_space_id_fkey FOREIGN KEY (space_id) REFERENCES public.spaces(id) ON DELETE CASCADE;


--
-- Name: wiki_revisions wiki_revisions_hitl_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wiki_revisions
    ADD CONSTRAINT wiki_revisions_hitl_reviewed_by_fkey FOREIGN KEY (hitl_reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: wiki_revisions wiki_revisions_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wiki_revisions
    ADD CONSTRAINT wiki_revisions_reviewed_by_fkey FOREIGN KEY (reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: wiki_revisions wiki_revisions_submitted_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wiki_revisions
    ADD CONSTRAINT wiki_revisions_submitted_by_fkey FOREIGN KEY (submitted_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: wiki_revisions wiki_revisions_wiki_page_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wiki_revisions
    ADD CONSTRAINT wiki_revisions_wiki_page_id_fkey FOREIGN KEY (wiki_page_id) REFERENCES public.wiki_pages(id) ON DELETE CASCADE;


--
-- Name: ws_tickets ws_tickets_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ws_tickets
    ADD CONSTRAINT ws_tickets_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--



SET search_path = public;
