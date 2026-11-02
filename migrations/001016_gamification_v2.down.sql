-- 001016_gamification_v2 down — reverses the catalogue v2 upgrade.
-- Drops the four new tables, removes the seeded catalogue rows and point
-- rules, reverts the baseline badges' taxonomy columns, then drops the
-- added columns and restores the original trigger_type check.

-- Seeded catalogue rows (inserted by this migration) — deleting them also
-- cascades user_badges / badge_progress / badge_award_events rows that
-- reference them.
DELETE FROM public.badge_definitions
WHERE slug IN (
    'japa_starter','identity_established','perfect_profile','passport_ready',
    'picture_perfect','voice_of_identity','social_links','email_verified',
    'phone_verified','verified_member','early_adopter','japa_pioneer',
    'japa_curious','self_discovery','plan_maker','strategic_planner',
    'goal_setter','destination_decided','route_master','visa_explorer',
    'document_collector','paperwork_pro','application_ready','japa_strategist',
    'mission_accomplished','journey_begins','first_destination',
    'multiple_possibilities','route_selected','on_the_move','waiting_game',
    'approval_alert','ticket_to_freedom','boarding_pass','the_big_move',
    'next_chapter','new_beginnings','settled_abroad','multiple_moves',
    'world_explorer','country_collector','global_explorer','world_citizen',
    'atlas_legend','country_specialist','city_explorer','global_nomad',
    'cultural_enthusiast','immigration_researcher','destination_expert',
    'global_knowledge','scholar_in_the_making','academic_explorer',
    'university_hunter','scholarship_seeker','scholarship_hunter',
    'funding_strategist','fully_funded_dream','application_starter',
    'scholarship_applied','academic_achiever','campus_bound','global_scholar',
    'career_explorer','listing_scout','opportunity_hunter',
    'career_profile_ready','first_application','persistent_applicant',
    'opportunity_magnet','interview_ready','interview_champion',
    'offer_received','visa_sponsored','global_professional',
    'career_trailblazer','first_connection','friendly_face','social_butterfly',
    'network_builder','community_member','community_regular',
    'community_connector','conversation_starter','people_person',
    'crowd_favourite','community_champion','connector_extraordinaire',
    'community_ambassador','storyteller','knowledge_sharer','content_machine',
    'thought_leader','guide_creator','information_architect',
    'experience_matters','japa_storyteller','trusted_contributor',
    'knowledge_authority','community_educator','knowledge_seeker',
    'first_discussion','curious_mind','question_master','helpful_member',
    'helpful_voice','problem_solver','solution_expert','answer_champion',
    'japa_champion','debate_enthusiast','voice_matters','poll_participant',
    'opinion_leader','discussion_champion','forum_legend','guidance_seeker',
    'mentorship_applicant','mentor_mentee','guided_journey','dedicated_learner',
    'mentors_favourite','mentor_in_training','first_time_mentor',
    'knowledge_mentor','master_mentor','transformational_mentor',
    'mentorship_ambassador','service_explorer','service_seeker','first_booking',
    'package_explorer','package_collector','package_member','loyal_customer',
    'service_reviewer','trusted_reviewer','service_champion',
    'partner_professional','service_excellence','financial_explorer',
    'budget_planner','smart_spender','cost_analyst','financial_strategist',
    'currency_watcher','exchange_rate_expert','money_mover','savings_starter',
    'consistent_saver','budget_master','financial_freedom','travel_curious',
    'flight_hunter','travel_planner','hotel_explorer','travel_ready',
    'border_crosser','frequent_traveller','global_traveller','new_home',
    'settling_in','local_explorer','first_alert','alert_contributor',
    'verified_watchdog','scam_investigator','scam_spotter','safety_advocate',
    'digital_guardian','responsible_member','trust_builder','verified_identity',
    'trusted_partner','community_protector','guardian_of_community',
    'daily_visitor','consistent','two_week_warrior','monthly_regular',
    'quarter_champion','half_year_hero','year_of_dedication',
    'challenge_accepted','challenge_hunter','challenge_champion',
    'challenge_legend','mission_impossible','comeback_kid','never_give_up',
    'founding_member','japa_legend','hall_of_fame','outstanding_contributor',
    'community_hero','global_ambassador','trailblazer','change_maker',
    'lifetime_achievement','japa_to_champion','smart_planner',
    'comparison_rookie','decision_maker','destination_analyst','city_analyst',
    'comparison_expert','ultimate_comparator','first_lesson','avid_learner',
    'dedicated_student','quiz_master','perfect_score','learning_champion',
    'academy_graduate','lifelong_learner','group_starter','space_founder',
    'group_organizer','event_participant','active_participant','community_host',
    'space_leader','community_builder','thriving_community','home_seeker',
    'housing_hunter','neighbourhood_explorer','rental_ready','first_home',
    'settlement_specialist','home_sweet_home','document_explorer',
    'first_attestation','document_detective','verification_pro',
    'paperwork_champion','ai_explorer','ai_inquisitive','ai_researcher',
    'ai_smart_planner','ai_collaborator','digital_strategist','first_referral',
    'referral_champion','growth_ambassador','network_champion',
    'community_recruiter','growth_legend'
);

-- Point rules added by this migration.
DELETE FROM public.activity_point_rules
WHERE activity_key IN (
    'forum.thread_created','forum.poll_voted','job.saved','scholarship.saved',
    'service.saved','comparison.completed','wiki.country_viewed',
    'wiki.city_viewed','wiki.page_viewed','content.read',
    'mentorship.requested','mentorship.connected','mentorship.completed',
    'mentorship.mentor_completed','challenge.completed',
    'journey.milestone.application_submitted',
    'journey.milestone.decision_received','journey.milestone.visa_approved',
    'journey.milestone.job_offer','journey.milestone.university_accepted',
    'journey.milestone.arrived','journey.milestone.one_year',
    'journey.milestone.citizenship','profile.photo_added','profile.bio_added',
    'profile.destination_set','email.verified','phone.verified',
    'identity.verified','space.created','scam_report.submitted',
    'checklist.item_checked','service.review_submitted','event.rsvp',
    'ai.conversation_started','referral.created',
    'scholarship.application_started','scholarship.application_submitted',
    'tool.cost_of_living.used','tool.remittance.used','tool.visa_routes.used',
    'tool.scholarships.used','tool.jobs_board.used',
    'tool.services_directory.used','tool.universities.used',
    'tool.housing_guide.used','tool.attestation_guide.used',
    'tool.arrival_checklist.used','tool.travel.used',
    'tool.readiness_assessment.used','tool.processing_tracker.used',
    'visa_timeline.milestone'
);

DROP TABLE IF EXISTS public.badge_award_events;
DROP TABLE IF EXISTS public.badge_tiers;
DROP TABLE IF EXISTS public.badge_progress;

ALTER TABLE public.badge_definitions
    DROP CONSTRAINT IF EXISTS badge_definitions_season_id_fkey,
    DROP CONSTRAINT IF EXISTS badge_definitions_trigger_type_check,
    DROP CONSTRAINT IF EXISTS badge_definitions_tier_check,
    DROP CONSTRAINT IF EXISTS badge_definitions_rarity_check;

DROP INDEX IF EXISTS public.idx_badge_definitions_group;
DROP INDEX IF EXISTS public.idx_badge_definitions_category;

ALTER TABLE public.badge_definitions
    DROP COLUMN IF EXISTS season_id,
    DROP COLUMN IF EXISTS available_until,
    DROP COLUMN IF EXISTS available_from,
    DROP COLUMN IF EXISTS points_reward,
    DROP COLUMN IF EXISTS is_featured,
    DROP COLUMN IF EXISTS is_limited,
    DROP COLUMN IF EXISTS is_secret,
    DROP COLUMN IF EXISTS badge_group,
    DROP COLUMN IF EXISTS tier,
    DROP COLUMN IF EXISTS rarity,
    DROP COLUMN IF EXISTS category;

DROP TABLE IF EXISTS public.badge_seasons;

ALTER TABLE public.badge_definitions
    ADD CONSTRAINT badge_definitions_trigger_type_check
    CHECK (trigger_type = ANY (ARRAY['lifetime_points','activity_count','manual']));
