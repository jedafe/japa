#!/usr/bin/env python3
"""Generate missing API client methods for unconsumed backend routes."""

import re

UNCONSUMED = [
    # control (28)
    ("control", "DELETE", "/control/api-keys/{id}"),
    ("control", "DELETE", "/control/queue/dlq/{task_id}"),
    ("control", "DELETE", "/control/security/ip-blocks/{id}"),
    ("control", "DELETE", "/control/tenants/{id}"),
    ("control", "GET", "/control/api-keys/analytics"),
    ("control", "GET", "/control/api-keys/{id}"),
    ("control", "GET", "/control/cache/stats"),
    ("control", "GET", "/control/deployments"),
    ("control", "GET", "/control/tenants/{id}"),
    ("control", "PATCH", "/control/ai-budget"),
    ("control", "PATCH", "/control/api-keys/{id}"),
    ("control", "PATCH", "/control/feature-flags/{flag_name}"),
    ("control", "PATCH", "/control/system-settings"),
    ("control", "PATCH", "/control/system-settings/{key}"),
    ("control", "PATCH", "/control/tenants/{id}"),
    ("control", "POST", "/control/api-keys"),
    ("control", "POST", "/control/api-keys/{id}/suspend"),
    ("control", "POST", "/control/cache/flush"),
    ("control", "POST", "/control/cron/jobs/{id}/trigger"),
    ("control", "POST", "/control/health/db/vacuum"),
    ("control", "POST", "/control/queue/dlq/discard-all"),
    ("control", "POST", "/control/queue/dlq/{task_id}/retry"),
    ("control", "POST", "/control/security/ip-blocks"),
    ("control", "POST", "/control/security/{id}/resolve"),
    ("control", "POST", "/control/system-settings/{key}/reset"),
    ("control", "POST", "/control/tenants"),
    ("control", "POST", "/control/tenants/{id}/suspend"),
    ("control", "POST", "/control/tenants/{id}/unsuspend"),

    # admin ads
    ("admin", "DELETE", "/admin/ads/campaigns/{id}"),
    ("admin", "GET", "/admin/ads/boosts/active"),
    ("admin", "GET", "/admin/ads/campaigns"),
    ("admin", "GET", "/admin/ads/campaigns/{id}"),
    ("admin", "GET", "/admin/ads/campaigns/{id}/stats"),
    ("admin", "GET", "/admin/ads/slot-types/{id}"),
    ("admin", "PATCH", "/admin/ads/campaigns/{id}"),
    ("admin", "POST", "/admin/ads/boosts/{id}/end"),
    ("admin", "POST", "/admin/ads/campaigns"),
    ("admin", "POST", "/admin/ads/campaigns/{id}/activate"),
    ("admin", "POST", "/admin/ads/campaigns/{id}/end"),
    ("admin", "POST", "/admin/ads/campaigns/{id}/pause"),

    # admin ai
    ("admin", "DELETE", "/admin/ai/context/{user_id}/{entry_id}"),
    ("admin", "GET", "/admin/ai/context/{user_id}"),
    ("admin", "GET", "/admin/ai/tools"),
    ("admin", "GET", "/admin/ai/tools/usage"),
    ("admin", "GET", "/admin/ai/tools/{id}"),
    ("admin", "GET", "/admin/ai/usage"),
    ("admin", "PUT", "/admin/ai/tools/{id}"),

    # admin analytics
    ("admin", "GET", "/admin/analytics/content"),
    ("admin", "GET", "/admin/analytics/events"),
    ("admin", "GET", "/admin/analytics/overview"),
    ("admin", "GET", "/admin/analytics/revenue"),
    ("admin", "GET", "/admin/analytics/search"),
    ("admin", "GET", "/admin/analytics/security"),
    ("admin", "GET", "/admin/analytics/spaces"),
    ("admin", "GET", "/admin/analytics/users"),
    ("admin", "POST", "/admin/analytics/cache/flush"),
    ("admin", "POST", "/admin/analytics/refresh"),
    ("admin", "POST", "/admin/analytics/security/scan"),
    ("admin", "POST", "/admin/analytics/security/{id}/resolve"),

    # admin affiliate
    ("admin", "DELETE", "/admin/affiliate-providers/{id}"),
    ("admin", "GET", "/admin/affiliate-links"),
    ("admin", "GET", "/admin/affiliate-program/commission-rates"),
    ("admin", "GET", "/admin/affiliate-program/members"),
    ("admin", "GET", "/admin/affiliate-providers"),
    ("admin", "GET", "/admin/affiliate-providers/{id}"),
    ("admin", "PATCH", "/admin/affiliate-program/commission-rates"),
    ("admin", "PATCH", "/admin/affiliate-program/members/{user_id}"),
    ("admin", "POST", "/admin/affiliate-providers"),

    # admin appeals
    ("admin", "GET", "/admin/appeals"),
    ("admin", "GET", "/admin/appeals/{id}"),
    ("admin", "PATCH", "/admin/appeals/{id}"),

    # admin blog
    ("admin", "GET", "/admin/blog/review-queue"),
    ("admin", "POST", "/admin/blog/review-queue/{id}/approve"),
    ("admin", "POST", "/admin/blog/review-queue/{id}/reject"),

    # admin bots
    ("admin", "GET", "/admin/bots"),
    ("admin", "GET", "/admin/bots/{id}"),
    ("admin", "POST", "/admin/bots"),
    ("admin", "PUT", "/admin/bots/{id}"),

    # admin broadcasts
    ("admin", "GET", "/admin/broadcasts"),
    ("admin", "GET", "/admin/broadcasts/{id}"),
    ("admin", "POST", "/admin/broadcasts"),
    ("admin", "POST", "/admin/broadcasts/{id}/send"),

    # admin comparisons
    ("admin", "DELETE", "/admin/comparison-types/{id}"),
    ("admin", "GET", "/admin/comparison-types"),
    ("admin", "POST", "/admin/comparison-types"),
    ("admin", "PATCH", "/admin/comparison-types/{id}"),

    # admin content
    ("admin", "DELETE", "/admin/content/{id}/remove"),
    ("admin", "GET", "/admin/content"),
    ("admin", "GET", "/admin/content/{id}/remove"),
    ("admin", "POST", "/admin/content/{id}/remove"),
    ("admin", "POST", "/admin/content/{id}/restore"),

    # admin coupons
    ("admin", "DELETE", "/admin/coupons/{id}"),
    ("admin", "GET", "/admin/coupons"),
    ("admin", "GET", "/admin/coupons/{id}"),
    ("admin", "PATCH", "/admin/coupons/{id}"),
    ("admin", "POST", "/admin/coupons"),

    # admin data-exports
    ("admin", "GET", "/admin/data-exports"),

    # admin deletions
    ("admin", "GET", "/admin/deletions"),

    # admin discount-codes
    ("admin", "DELETE", "/admin/discount-codes/{id}"),
    ("admin", "GET", "/admin/discount-codes"),
    ("admin", "GET", "/admin/discount-codes/{id}/usage"),
    ("admin", "PATCH", "/admin/discount-codes/{id}"),
    ("admin", "POST", "/admin/discount-codes"),

    # admin docs
    ("admin", "DELETE", "/admin/docs/pages/{id}"),
    ("admin", "GET", "/admin/docs/{audience}"),
    ("admin", "GET", "/admin/docs/pages/{id}"),
    ("admin", "PATCH", "/admin/docs/pages/{id}"),
    ("admin", "POST", "/admin/docs/ai-generate"),
    ("admin", "POST", "/admin/docs/pages/{id}/approve"),
    ("admin", "POST", "/admin/docs/pages/{id}/publish"),

    # admin field-groups
    ("admin", "DELETE", "/admin/field-groups/{id}"),
    ("admin", "DELETE", "/admin/field-groups/{id}/fields/{fid}"),
    ("admin", "GET", "/admin/field-groups"),
    ("admin", "PATCH", "/admin/field-groups/{id}"),
    ("admin", "PATCH", "/admin/field-groups/{id}/fields/{fid}"),
    ("admin", "POST", "/admin/field-groups"),
    ("admin", "POST", "/admin/field-groups/{id}/fields"),
    ("admin", "POST", "/admin/field-groups/{id}/fields/reorder"),

    # admin forms
    ("admin", "DELETE", "/admin/forms/{id}/submissions/{sub_id}"),
    ("admin", "GET", "/admin/forms/{id}/submissions/{sub_id}"),
    ("admin", "PATCH", "/admin/forms/{id}/submissions/{sub_id}"),
    ("admin", "POST", "/admin/forms/{id}/submissions"),

    # admin gdpr
    ("admin", "GET", "/admin/gdpr/deletion-requests"),
    ("admin", "GET", "/admin/gdpr/exports"),
    ("admin", "POST", "/admin/gdpr/deletion-requests/{id}/execute"),

    # admin i18n
    ("admin", "GET", "/admin/i18n/languages"),
    ("admin", "GET", "/admin/i18n/strings"),
    ("admin", "POST", "/admin/i18n/strings"),
    ("admin", "POST", "/admin/i18n/strings/ai-translate-batch"),
    ("admin", "POST", "/admin/i18n/strings/batch"),
    ("admin", "POST", "/admin/i18n/strings/{id}/translate"),

    # admin learn
    ("admin", "DELETE", "/admin/learn/lessons/{id}"),
    ("admin", "DELETE", "/admin/learn/modules/{id}"),
    ("admin", "DELETE", "/admin/learn/tracks/{id}"),
    ("admin", "GET", "/admin/learn/tracks"),
    ("admin", "GET", "/admin/learn/tracks/{id}/enrollments"),
    ("admin", "PATCH", "/admin/learn/lessons/{id}"),
    ("admin", "PATCH", "/admin/learn/modules/{id}"),
    ("admin", "PATCH", "/admin/learn/tracks/{id}"),
    ("admin", "POST", "/admin/learn/modules/{moduleId}/lessons"),
    ("admin", "POST", "/admin/learn/tracks"),
    ("admin", "POST", "/admin/learn/tracks/{id}/publish"),
    ("admin", "POST", "/admin/learn/tracks/{trackId}/modules"),

    # admin media
    ("admin", "DELETE", "/admin/media/{id}"),
    ("admin", "GET", "/admin/media"),
    ("admin", "GET", "/admin/media/quarantined"),
    ("admin", "POST", "/admin/media/{id}/approve"),
    ("admin", "POST", "/admin/media/{id}/reject"),

    # admin mentorship
    ("admin", "GET", "/admin/mentorship/matches"),
    ("admin", "GET", "/admin/mentorship/programs"),
    ("admin", "GET", "/admin/mentorship/programs/{id}"),
    ("admin", "POST", "/admin/mentorship/programs"),
    ("admin", "PATCH", "/admin/mentorship/matches/{id}"),
    ("admin", "PATCH", "/admin/mentorship/programs/{id}"),

    # admin moderation
    ("admin", "GET", "/admin/moderation-queue"),
    ("admin", "GET", "/admin/moderation/alerts"),
    ("admin", "PATCH", "/admin/moderation-queue/{id}"),
    ("admin", "PATCH", "/admin/moderation-queue/{id}/assign"),
    ("admin", "POST", "/admin/moderation-queue/bulk"),
    ("admin", "POST", "/admin/moderation-queue/{id}/appeal"),
    ("admin", "POST", "/admin/moderation/alerts/{id}/acknowledge"),
    ("admin", "POST", "/admin/moderation/alerts/{id}/resolve"),

    # admin newsletter
    ("admin", "DELETE", "/admin/newsletter/subscribers/{id}"),
    ("admin", "GET", "/admin/newsletter/campaigns"),
    ("admin", "GET", "/admin/newsletter/campaigns/{id}"),
    ("admin", "GET", "/admin/newsletter/subscribers"),
    ("admin", "GET", "/admin/newsletter/subscribers/{id}"),
    ("admin", "PATCH", "/admin/newsletter/campaigns/{id}"),
    ("admin", "POST", "/admin/newsletter/campaigns"),
    ("admin", "POST", "/admin/newsletter/campaigns/{id}/send"),

    # admin notification-types
    ("admin", "DELETE", "/admin/notification-types/{key}"),
    ("admin", "GET", "/admin/notification-types"),
    ("admin", "PATCH", "/admin/notification-types/{key}"),
    ("admin", "POST", "/admin/notification-types"),

    # admin page-builder
    ("admin", "DELETE", "/admin/pagebuilder/blocks/{id}"),
    ("admin", "DELETE", "/admin/pagebuilder/themes/{id}"),
    ("admin", "GET", "/admin/pagebuilder/blocks"),
    ("admin", "GET", "/admin/pagebuilder/blocks/{id}"),
    ("admin", "GET", "/admin/pagebuilder/themes"),
    ("admin", "GET", "/admin/pagebuilder/themes/{id}"),
    ("admin", "PATCH", "/admin/pagebuilder/blocks/{id}"),
    ("admin", "PATCH", "/admin/pagebuilder/themes/{id}"),
    ("admin", "POST", "/admin/pagebuilder/blocks"),
    ("admin", "POST", "/admin/pagebuilder/themes"),
    ("admin", "POST", "/admin/pagebuilder/themes/{id}/default"),

    # admin page-templates
    ("admin", "DELETE", "/admin/page-templates/{id}"),
    ("admin", "GET", "/admin/page-templates"),
    ("admin", "GET", "/admin/page-templates/{id}"),
    ("admin", "PATCH", "/admin/page-templates/{id}"),
    ("admin", "POST", "/admin/page-templates"),

    # admin profiles
    ("admin", "POST", "/admin/profiles/{id}/verify"),

    # admin referral
    ("admin", "GET", "/admin/referral/program"),
    ("admin", "GET", "/admin/referral/referrals"),
    ("admin", "GET", "/admin/referral/referrals/{id}"),
    ("admin", "PUT", "/admin/referral/program"),

    # admin reports
    ("admin", "GET", "/admin/reports"),
    ("admin", "GET", "/admin/reports/{id}"),
    ("admin", "PATCH", "/admin/reports/{id}"),

    # admin reviews
    ("admin", "GET", "/admin/reviews"),
    ("admin", "PATCH", "/admin/reviews/{id}"),

    # admin roles
    ("admin", "DELETE", "/admin/roles/{id}"),
    ("admin", "GET", "/admin/roles/{id}/permissions"),
    ("admin", "PATCH", "/admin/roles/{id}"),
    ("admin", "PATCH", "/admin/roles/{id}/permissions"),
    ("admin", "POST", "/admin/roles"),
    ("admin", "POST", "/admin/roles/{id}/permissions"),

    # admin search
    ("admin", "GET", "/admin/search/analytics"),

    # admin seo
    ("admin", "GET", "/admin/seo/validate/{id}"),

    # admin settings
    ("admin", "GET", "/admin/settings"),
    ("admin", "PUT", "/admin/settings/{category}"),

    # admin spaces
    ("admin", "GET", "/admin/spaces/{id}/join-requests"),
    ("admin", "PATCH", "/admin/spaces/{id}/join-requests/{requestId}"),

    # admin tags
    ("admin", "DELETE", "/admin/tags/{id}/moderators/{userId}"),
    ("admin", "POST", "/admin/tags/{id}/moderators"),

    # admin tickets
    ("admin", "DELETE", "/admin/tickets/{id}"),
    ("admin", "GET", "/admin/tickets"),
    ("admin", "GET", "/admin/tickets/{id}"),
    ("admin", "GET", "/admin/tickets/{id}/events"),
    ("admin", "PATCH", "/admin/tickets/{id}"),
    ("admin", "POST", "/admin/tickets/{id}/comments"),
    ("admin", "POST", "/admin/tickets/{id}/notes"),
    ("admin", "POST", "/admin/tickets/{id}/resolve"),

    # admin user-groups
    ("admin", "GET", "/admin/user-groups/{id}/members"),
    ("admin", "GET", "/admin/user-groups/{id}/roles"),
    ("admin", "POST", "/admin/user-groups/{id}/members"),
    ("admin", "POST", "/admin/user-groups/{id}/roles"),

    # admin users
    ("admin", "DELETE", "/admin/users/{id}"),
    ("admin", "GET", "/admin/users"),
    ("admin", "GET", "/admin/users/{id}"),
    ("admin", "GET", "/admin/users/{id}/activity"),
    ("admin", "GET", "/admin/users/{id}/ai-context"),
    ("admin", "GET", "/admin/users/{id}/bans"),
    ("admin", "GET", "/admin/users/{id}/sessions"),
    ("admin", "GET", "/admin/users/{id}/warnings"),
    ("admin", "PATCH", "/admin/users/{id}/group"),
    ("admin", "PATCH", "/admin/users/{id}/status"),
    ("admin", "POST", "/admin/users/{id}/ban"),
    ("admin", "POST", "/admin/users/{id}/warn"),

    # admin webhooks
    ("admin", "DELETE", "/admin/webhooks/{id}"),
    ("admin", "GET", "/admin/webhooks/{id}"),
    ("admin", "PATCH", "/admin/webhooks/{id}"),
    ("admin", "POST", "/admin/webhooks/{id}/deliveries/{delivery_id}/retry"),
    ("admin", "POST", "/admin/webhooks/{id}/retry/{delivery_id}"),
    ("admin", "POST", "/admin/webhooks/{id}/test"),
    ("admin", "POST", "/admin/webhooks/{id}/verify"),
    ("admin", "PUT", "/admin/webhooks/{id}"),

    # admin widgets
    ("admin", "DELETE", "/admin/widgets/{id}"),
    ("admin", "GET", "/admin/widgets"),
    ("admin", "PATCH", "/admin/widgets/{id}"),
    ("admin", "POST", "/admin/widgets"),

    # admin wallet
    ("admin", "POST", "/admin/wallet/award"),

    # main routes (selected high-priority)
    ("main", "GET", "/me/analytics"),
    ("main", "GET", "/me/api-keys/{id}"),
    ("main", "GET", "/me/api-keys/{id}/ip-allowlist"),
    ("main", "GET", "/me/blocked-users"),
    ("main", "GET", "/me/connected-accounts"),
    ("main", "GET", "/me/content"),
    ("main", "GET", "/me/data-export/download"),
    ("main", "GET", "/me/data-export/status"),
    ("main", "GET", "/me/followed-tags"),
    ("main", "GET", "/me/following"),
    ("main", "GET", "/me/media"),
    ("main", "GET", "/me/muted-spaces"),
    ("main", "GET", "/me/muted-users"),
    ("main", "GET", "/me/preferences"),
    ("main", "GET", "/me/privacy"),
    ("main", "GET", "/me/reports"),
    ("main", "GET", "/me/sessions"),
    ("main", "GET", "/me/spaces"),
    ("main", "GET", "/me/spaces/moderated"),
    ("main", "GET", "/me/spaces/owned"),
    ("main", "GET", "/me/tickets/{id}/attachments"),
    ("main", "GET", "/me/totp/status"),
    ("main", "GET", "/me/wallet/credits"),
    ("main", "GET", "/me/wallet/points"),
    ("main", "DELETE", "/me/api-keys/{id}/ip-allowlist/{ipId}"),
    ("main", "DELETE", "/me/connected-accounts/{provider}"),
    ("main", "DELETE", "/me/notifications/{id}"),
    ("main", "DELETE", "/me/push-tokens"),
    ("main", "DELETE", "/me/push-tokens/{id}"),
    ("main", "DELETE", "/me/sessions"),
    ("main", "DELETE", "/me/sessions/{id}"),
    ("main", "POST", "/me/api-keys/{id}/ip-allowlist"),
    ("main", "POST", "/me/api-keys/{id}/rotate"),
    ("main", "POST", "/me/change-password"),
    ("main", "POST", "/me/deactivate"),
    ("main", "POST", "/me/subscription/reactivate"),
    ("main", "POST", "/me/tickets/{id}/attachments"),
    ("main", "PATCH", "/me/account"),
    ("main", "PATCH", "/me/affiliate-links/{id}"),
    ("main", "PATCH", "/me/notification-preferences"),
    ("main", "PATCH", "/me/privacy"),
    ("main", "PUT", "/me/preferences"),
    ("main", "GET", "/affiliate-program"),
    ("main", "GET", "/ai/conversations"),
    ("main", "GET", "/ai/conversations/{id}"),
    ("main", "GET", "/blog"),
    ("main", "GET", "/blog/reviews"),
    ("main", "GET", "/blog/{slug}"),
    ("main", "GET", "/claim-ownership"),
    ("main", "GET", "/comparisons/{id}"),
    ("main", "GET", "/content"),
    ("main", "GET", "/content/{id}/analytics"),
    ("main", "GET", "/content/{id}/bookmark"),
    ("main", "GET", "/content/{id}/comments"),
    ("main", "GET", "/content/{id}/discussions"),
    ("main", "GET", "/content/{id}/related"),
    ("main", "GET", "/content/{id}/share-count"),
    ("main", "GET", "/content/{id}/tags"),
    ("main", "GET", "/content/{slug}"),
    ("main", "GET", "/coupons"),
    ("main", "GET", "/custom-field-groups/{id}"),
    ("main", "GET", "/custom-field-values/{entity_type}/{entity_id}"),
    ("main", "GET", "/developer/swagger"),
    ("main", "GET", "/dm/requests"),
    ("main", "GET", "/forms"),
    ("main", "GET", "/forms/{id}"),
    ("main", "GET", "/forms/{id}/submissions"),
    ("main", "GET", "/forum/tags/{slug}"),
    ("main", "GET", "/geo/countries/{code}/states"),
    ("main", "GET", "/geo/detect"),
    ("main", "GET", "/geo/states/{id}/cities"),
    ("main", "GET", "/i18n/packs"),
    ("main", "GET", "/learn/lessons/{id}"),
    ("main", "GET", "/learn/quizzes/{id}"),
    ("main", "GET", "/profiles/types/{slug}"),
    ("main", "GET", "/profiles/{id}/author-context"),
    ("main", "GET", "/profiles/{id}/content"),
    ("main", "GET", "/reviews"),
    ("main", "GET", "/reviews/{id}"),
    ("main", "GET", "/search/autocomplete"),
    ("main", "GET", "/spaces/search"),
    ("main", "GET", "/spaces/suggest-parents"),
    ("main", "GET", "/spaces/{id}/analytics"),
    ("main", "GET", "/spaces/{id}/search"),
    ("main", "GET", "/spaces/{slug}/content"),
    ("main", "GET", "/spaces/{slug}/members"),
    ("main", "GET", "/spaces/{slug}/moderators"),
    ("main", "GET", "/spaces/{slug}/mute-status"),
    ("main", "GET", "/spaces/{slug}/parents"),
    ("main", "GET", "/spaces/{slug}/pinned"),
    ("main", "GET", "/spaces/{slug}/rules"),
    ("main", "GET", "/subscriptions"),
    ("main", "GET", "/suggestions/content"),
    ("main", "GET", "/tags/trending"),
    ("main", "GET", "/tags/{slug}"),
    ("main", "GET", "/tags/{slug}/content"),
    ("main", "GET", "/tags/{slug}/search"),
    ("main", "GET", "/universities/{id}/compare"),
    ("main", "GET", "/universities/{slug}"),
    ("main", "GET", "/users/{id}/analytics"),
    ("main", "GET", "/users/{id}/content"),
    ("main", "GET", "/users/{user_id}/follow-status"),
    ("main", "GET", "/widgets/{id}"),
    ("main", "POST", "/ai/chat"),
    ("main", "POST", "/blog"),
    ("main", "POST", "/blog/featured"),
    ("main", "POST", "/blog/reviews"),
    ("main", "POST", "/blog/series"),
    ("main", "POST", "/blog/series/{id}/posts"),
    ("main", "POST", "/blog/{id}/feature"),
    ("main", "POST", "/blog/{id}/submit-for-review"),
    ("main", "POST", "/content/{id}/archive"),
    ("main", "POST", "/content/{id}/bookmark"),
    ("main", "POST", "/content/{id}/comments"),
    ("main", "POST", "/content/{id}/discussions"),
    ("main", "POST", "/content/{id}/publish"),
    ("main", "POST", "/content/{id}/react"),
    ("main", "POST", "/content/{id}/report"),
    ("main", "POST", "/content/{id}/schedule"),
    ("main", "POST", "/content/{id}/share"),
    ("main", "POST", "/content/{id}/tags"),
    ("main", "POST", "/content/{id}/view"),
    ("main", "POST", "/custom-field-groups"),
    ("main", "POST", "/custom-field-groups/{id}/fields"),
    ("main", "POST", "/dm/conversations"),
    ("main", "POST", "/forum/{id}/poll"),
    ("main", "POST", "/forum/{id}/replies"),
    ("main", "POST", "/forum/{id}/tags"),
    ("main", "POST", "/forum/{id}/tags/{tagId}"),
    ("main", "POST", "/learn/lessons/{id}/complete"),
    ("main", "POST", "/learn/lessons/{id}/progress"),
    ("main", "POST", "/learn/tracks/{trackId}/enroll"),
    ("main", "POST", "/media/bulk-delete"),
    ("main", "POST", "/media/confirm/{id}"),
    ("main", "POST", "/profiles/invitations/accept"),
    ("main", "POST", "/profiles/invitations/{token}/accept"),
    ("main", "POST", "/profiles/{id}/follow"),
    ("main", "POST", "/profiles/{id}/members/invite"),
    ("main", "POST", "/spaces/{id}/report"),
    ("main", "POST", "/spaces/{slug}/follow"),
    ("main", "POST", "/spaces/{slug}/moderators"),
    ("main", "POST", "/spaces/{slug}/mute"),
    ("main", "POST", "/spaces/{slug}/parents"),
    ("main", "POST", "/spaces/{slug}/pinned"),
    ("main", "POST", "/spaces/{slug}/rules"),
    ("main", "POST", "/tags/{slug}/follow"),
    ("main", "POST", "/users/{id}/report"),
    ("main", "POST", "/users/{user_id}/block"),
    ("main", "POST", "/users/{user_id}/mute"),
    ("main", "PATCH", "/blog/featured/reorder"),
    ("main", "PATCH", "/blog/reviews/{id}"),
    ("main", "PATCH", "/blog/series/{id}"),
    ("main", "PATCH", "/blog/series/{id}/posts/reorder"),
    ("main", "PATCH", "/blog/{id}"),
    ("main", "PATCH", "/content/{id}/spaces"),
    ("main", "PATCH", "/custom-field-groups/{id}"),
    ("main", "PATCH", "/custom-fields/{id}"),
    ("main", "PATCH", "/forum/tags/{id}"),
    ("main", "PATCH", "/me/affiliate-links/{id}"),
    ("main", "PATCH", "/media/{id}"),
    ("main", "PATCH", "/pages/{id}/blocks"),
    ("main", "PATCH", "/profiles/documents/{documentId}"),
    ("main", "PATCH", "/profiles/relocations/{relocationId}"),
    ("main", "PATCH", "/spaces/{id}"),
    ("main", "PATCH", "/spaces/{id}/pinned/reorder"),
    ("main", "PATCH", "/spaces/{slug}/rules/{ruleId}"),
    ("main", "PUT", "/custom-fields/{id}/permissions"),
    ("main", "PUT", "/space-permission-presets/{name}"),
    ("main", "DELETE", "/blog/featured/{contentId}"),
    ("main", "DELETE", "/blog/series/{id}"),
    ("main", "DELETE", "/blog/series/{id}/posts/{postId}"),
    ("main", "DELETE", "/blog/{id}"),
    ("main", "DELETE", "/comparisons/dimensions/{id}"),
    ("main", "DELETE", "/comparisons/items/{id}"),
    ("main", "DELETE", "/comparisons/{id}"),
    ("main", "DELETE", "/comparisons/{id}/items/{itemId}"),
    ("main", "DELETE", "/content/{id}/bookmark"),
    ("main", "DELETE", "/content/{id}/react"),
    ("main", "DELETE", "/content/{id}/tags/{tagId}"),
    ("main", "DELETE", "/custom-field-groups/{id}"),
    ("main", "DELETE", "/custom-fields/{id}"),
    ("main", "DELETE", "/dm/conversations/{id}/messages/{mid}"),
    ("main", "DELETE", "/forms/{id}"),
    ("main", "DELETE", "/forum/tags/{id}"),
    ("main", "DELETE", "/forum/{id}/solve"),
    ("main", "DELETE", "/forum/{id}/tags/{tagId}"),
    ("main", "DELETE", "/forum/{id}/vote"),
    ("main", "DELETE", "/poll-options/{id}/vote"),
    ("main", "DELETE", "/profiles/documents/{documentId}"),
    ("main", "DELETE", "/profiles/relocations/{relocationId}"),
    ("main", "DELETE", "/profiles/verifiers/{verifierId}"),
    ("main", "DELETE", "/profiles/{id}/follow"),
    ("main", "DELETE", "/spaces/{id}"),
    ("main", "DELETE", "/spaces/{slug}/follow"),
    ("main", "DELETE", "/spaces/{slug}/leave"),
    ("main", "DELETE", "/spaces/{slug}/moderators/{userId}"),
    ("main", "DELETE", "/spaces/{slug}/mute"),
    ("main", "DELETE", "/spaces/{slug}/parents/{parentId}"),
    ("main", "DELETE", "/spaces/{slug}/pinned/{content_id}"),
    ("main", "DELETE", "/spaces/{slug}/rules/{ruleId}"),
    ("main", "DELETE", "/subscriptions/{id}"),
    ("main", "DELETE", "/tags/{slug}/follow"),
    ("main", "DELETE", "/users/{id}/follow"),
    ("main", "DELETE", "/users/{user_id}/block"),
    ("main", "DELETE", "/users/{user_id}/mute"),
    ("main", "POST", "/comments/{id}/report"),
    ("main", "POST", "/comparisons/{id}/dimensions"),
    ("main", "POST", "/comparisons/{id}/items"),
    ("main", "POST", "/comparisons/{id}/items/reorder"),
    ("main", "POST", "/content/{id}/tags"),
    ("main", "POST", "/dm/conversations/{id}/messages/{mid}/report"),
    ("main", "POST", "/dm/requests/{id}/block"),
    ("main", "POST", "/forum/{id}/vote"),
    ("main", "POST", "/me/api-keys/{id}/ip-allowlist"),
    ("main", "POST", "/poll-options/{id}/vote"),
    ("main", "POST", "/spaces/{id}/pinned/reorder"),
    ("main", "PATCH", "/comparisons/dimensions/{id}"),
    ("main", "PATCH", "/comparisons/items/{id}"),
    ("main", "PATCH", "/comparisons/{id}"),
    ("main", "PATCH", "/comparisons/{id}/items/{itemId}"),
    ("main", "PATCH", "/dm/conversations/{id}/messages/{mid}"),
    ("main", "POST", "/payments/webhook/nowpayments"),
    ("main", "POST", "/payments/webhook/paystack"),
    ("main", "POST", "/payments/webhook/stripe"),
]


def path_to_domain(path, app):
    if app == "admin":
        p = path.replace("/admin/", "")
        parts = p.split("/")
        domain = parts[0] if parts[0] else "root"
        mapping = {
            "ads": "ads", "ai": "ai", "analytics": "analytics",
            "affiliate": "affiliate", "appeals": "appeals", "blog": "blog",
            "bots": "bots", "broadcasts": "broadcasts", "comparison": "comparisons",
            "content": "content", "coupons": "coupons", "data-exports": "dataExports",
            "deletions": "deletions", "discount": "discounts", "docs": "docs",
            "field": "fieldGroups", "forms": "forms", "gdpr": "gdpr",
            "i18n": "i18n", "learn": "learn", "media": "media",
            "mentorship": "mentorship", "moderation": "moderation", "newsletter": "newsletter",
            "notification": "notifications", "page-templates": "pageTemplates",
            "pagebuilder": "pageBuilder", "profiles": "profiles",
            "referral": "referral", "reports": "reports", "reviews": "reviews",
            "roles": "roles", "search": "search", "seo": "seo",
            "settings": "settings", "spaces": "spaces", "tags": "tags",
            "tickets": "tickets", "user-groups": "userGroups", "users": "users",
            "webhooks": "webhooks", "widgets": "widgets", "wallet": "wallet",
        }
        for prefix, d in mapping.items():
            if p.startswith(prefix):
                return d
        return domain
    elif app == "control":
        parts = path.replace("/control/", "").split("/")
        return parts[0] if parts[0] else "root"
    elif app == "main":
        if path.startswith("/me/"):
            return "me"
        parts = path.strip("/").split("/")
        return parts[0] if parts[0] else "root"
    return "root"


LIST_ENDPOINTS = {
    "/admin/analytics/overview", "/admin/analytics/users", "/admin/analytics/spaces",
    "/admin/analytics/content", "/admin/analytics/events", "/admin/analytics/revenue",
    "/admin/analytics/search", "/admin/analytics/security", "/admin/reports",
    "/admin/data-exports", "/admin/deletions", "/admin/mentorship/matches",
    "/admin/mentorship/programs", "/admin/moderation-queue", "/admin/moderation/alerts",
    "/admin/newsletter/campaigns", "/admin/newsletter/subscribers",
    "/admin/notification-types", "/admin/pagebuilder/blocks", "/admin/pagebuilder/themes",
    "/admin/page-templates", "/admin/widgets", "/admin/spaces", "/admin/users",
    "/admin/field-groups", "/admin/forms", "/admin/i18n/languages", "/admin/i18n/strings",
    "/admin/media", "/admin/media/quarantined", "/admin/bots", "/admin/broadcasts",
    "/admin/comparison-types", "/admin/content", "/admin/coupons", "/admin/discount-codes",
    "/admin/tickets", "/admin/tags", "/admin/user-groups", "/admin/roles",
    "/admin/settings", "/admin/ads/campaigns", "/admin/ads/boosts/active",
    "/admin/affiliate-program/members", "/admin/affiliate-providers", "/admin/appeals",
    "/admin/blog/review-queue", "/admin/gdpr/deletion-requests", "/admin/gdpr/exports",
    "/admin/learn/tracks", "/admin/learn/tracks/{id}/enrollments",
    "/admin/referral/program", "/admin/referral/referrals", "/admin/reviews",
    "/admin/search/analytics", "/admin/affiliate-links",
}


def snake_part(s):
    s = re.sub(r"([A-Z]+)([A-Z][a-z])", r"\1_\2", s)
    s = re.sub(r"([a-z0-9])([A-Z])", r"\1_\2", s)
    return s.lower()


def method_name(method, path, existing):
    parts = [p.strip("{}") for p in path.split("/") if p and not p.startswith("{")]
    if not parts:
        base = "root"
    else:
        base = "_".join(parts)
    base_snake = snake_part(base).replace("-", "_")

    if method == "GET":
        if path in LIST_ENDPOINTS:
            prefix = "list"
        elif len(parts) <= 1:
            prefix = "list"
        else:
            prefix = "get"
    elif method == "POST":
        prefix = "create"
    elif method == "PATCH":
        prefix = "update"
    elif method == "PUT":
        prefix = "replace"
    elif method == "DELETE":
        prefix = "delete"
    else:
        prefix = method.lower()

    name = f"{prefix}{base_snake.replace('_', '').title().replace('_', '')}"
    name = re.sub(r"[^a-zA-Z0-9]", "", name)

    original = name
    counter = 1
    while name in existing:
        name = f"{original}{counter}"
        counter += 1
    existing.add(name)
    return name


def generate_domain_methods(routes, app):
    from collections import defaultdict
    domains = defaultdict(list)
    for method, path in routes:
        domain = path_to_domain(path, app)
        domains[domain].append((method, path))

    lines = []
    existing = set()

    for domain in sorted(domains.keys()):
        domain_routes = domains[domain]
        lines.append(f"\nexport const {domain}Api = {{")

        for method, path in sorted(domain_routes):
            mname = method_name(method, path, existing)
            params = re.findall(r"\{(\w+)\}", path)
            param_decls = ", ".join(params)

            if method == "GET":
                if params:
                    lines.append(f"  {mname}: ({param_decls}: string) => api<unknown>(`{path}`),")
                elif path in LIST_ENDPOINTS:
                    lines.append(f"  {mname}: (params?: string) => api<unknown>(`{path}${{params ? `?${params}` : ''}}`),")
                else:
                    lines.append(f"  {mname}: () => api<unknown>('{path}'),")
            elif method == "POST":
                if params:
                    lines.append(f"  {mname}: (body: Record<string, unknown>) => api<unknown>(`{path}`, {{ method: 'POST', body }}),")
                else:
                    lines.append(f"  {mname}: (body: Record<string, unknown>) => api<unknown>('{path}', {{ method: 'POST', body }}),")
            elif method == "PATCH":
                if params:
                    lines.append(f"  {mname}: ({param_decls}: string, body: Record<string, unknown>) => api<unknown>(`{path}`, {{ method: 'PATCH', body }}),")
                else:
                    lines.append(f"  {mname}: (body: Record<string, unknown>) => api<unknown>('{path}', {{ method: 'PATCH', body }}),")
            elif method == "PUT":
                if params:
                    lines.append(f"  {mname}: ({param_decls}: string, body: Record<string, unknown>) => api<unknown>(`{path}`, {{ method: 'PUT', body }}),")
                else:
                    lines.append(f"  {mname}: (body: Record<string, unknown>) => api<unknown>('{path}', {{ method: 'PUT', body }}),")
            elif method == "DELETE":
                if params:
                    lines.append(f"  {mname}: ({param_decls}: string) => api<{{ deleted: boolean }}>(`{path}`, {{ method: 'DELETE' }}),")
                else:
                    lines.append(f"  {mname}: () => api<{{ deleted: boolean }}>('{path}', {{ method: 'DELETE' }}),")

        lines.append("};\n")

    return "\n".join(lines)


if __name__ == "__main__":
    admin_routes = [(m, p) for a, m, p in UNCONSUMED if a == "admin"]
    control_routes = [(m, p) for a, m, p in UNCONSUMED if a == "control"]
    main_routes = [(m, p) for a, m, p in UNCONSUMED if a == "main"]

    print("=== ADMIN API CLIENT METHODS ===")
    print(generate_domain_methods(admin_routes, "admin"))

    print("\n=== CONTROL API CLIENT METHODS ===")
    control_out = generate_domain_methods(control_routes, "control")
    if control_out.strip():
        print("export const controlDomainApi = {")
        print(control_out)
        print("};")
    else:
        print("// No control methods")

    print("\n=== MAIN APP ROUTE CONSUMPTION ===")
    print(f"// {len(main_routes)} main routes need +page.server.ts / +page.svelte files")
    print("// See unconsumed list above for details")
