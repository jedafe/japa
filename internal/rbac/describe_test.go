package rbac

import "testing"

func TestDescribePermission(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"content.reactions.add":        "add content reactions",
		"spaces.create":                "create spaces",
		"spaces.moderate.any":          "moderate any space",
		"comments.delete_own":          "delete your own comments",
		"content.edit_any":             "edit any content",
		"admin.hard_delete":            "permanently delete records",
		"directory.users.view":         "view the user directory",
		"directory.spaces.view":        "view the space directory",
		"api.keys.manage":              "manage API keys",
		"academy.payout.view_all":      "view all academy payouts",
		"blog.edit_own":                "edit your own blog posts",
		"story.create":                 "create stories",
		"ai.byok":                      "use your own API keys",
		"agent.use":                    "use the AI agent",
		"admin.moderation":             "access moderation tools",
		"admin.broadcast":              "send broadcasts",
		"admin.users.manage":           "manage users",
		"analytics.admin":              "administer analytics",
		"ad.stats.view":                "view ad stats",
		"api.keys.analytics.view":      "view API keys analytics",
		"appointment_alert.manage_own": "manage your own appointment alerts",
		"assessment.view_results":      "view assessment results",
		"ai.context.admin_view":        "view AI contexts",
		"academy.revenue.list":         "list academy revenue",
		"spaces.admin":                 "administer spaces",
		"content.pin":                  "pin content",
		"feature.reactions":            "access the reaction feature",
		"api.read.content":             "read content via the API",
		"api.write.comments":           "write comments via the API",
		"arrival_checklist.enabled":    "access arrival checklists",
		"bots.admin":                   "administer bots",
		"api.keys.view.all":            "view all API keys",
		"*":                            "perform this action",
	}

	for node, want := range cases {
		if got := DescribePermission(node); got != want {
			t.Errorf("DescribePermission(%q) = %q, want %q", node, got, want)
		}
	}
}

func TestDescribeAnyPermission(t *testing.T) {
	t.Parallel()

	if got := DescribeAnyPermission("comparison.edit_own", "comparison.edit_any"); got != "edit comparisons" {
		t.Errorf("DescribeAnyPermission = %q, want %q", got, "edit comparisons")
	}

	if got := DescribeAnyPermission(); got != "perform this action" {
		t.Errorf("DescribeAnyPermission() = %q, want %q", got, "perform this action")
	}
}

func TestSpaceDenialMessage(t *testing.T) {
	t.Parallel()

	got := SpaceDenialMessage("spaces.moderate")
	want := "You don't have permission to moderate in this space — contact a space moderator or administrator."
	if got != want {
		t.Errorf("SpaceDenialMessage = %q, want %q", got, want)
	}
}

func TestDenialMessage(t *testing.T) {
	t.Parallel()

	got := DenialMessage("content.reactions.add")
	want := "Your account doesn't have permission to add content reactions — contact an administrator if you need access."
	if got != want {
		t.Errorf("DenialMessage = %q, want %q", got, want)
	}
}
