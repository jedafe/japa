// Command seed creates the default admin user, profile, group memberships,
// spaces, and preferences for a fresh JapaConnect database.
//
// It is idempotent: if the admin user already exists, it prints a message
// and exits cleanly. Run it after migrations:
//
//	source .env && go run ./cmd/seed/
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
)

// Seeded UUIDs — must match the values in the migration seed data.
const (
	superAdminGroupID = "00000000-0000-0000-0000-000000000010"
	membersGroupID    = "00000000-0000-0000-0000-000000000020"
	countrySpaceType  = "00000000-0000-0000-0000-000000000060"
)

// Default admin display values (non-sensitive).
const (
	adminUsername    = "admin"
	adminDisplayName = "Platform Admin"
)

// defaultSpace describes a space to create during seeding.
type defaultSpace struct {
	Name        string
	Slug        string
	Description string
}

var defaultSpaces = []defaultSpace{
	{Name: "Nigeria", Slug: "nigeria", Description: "Community for Nigerians at home and abroad"},
	{Name: "United Kingdom", Slug: "united-kingdom", Description: "Nigerians in the UK"},
	{Name: "United States", Slug: "united-states", Description: "Nigerians in the US"},
	{Name: "Canada", Slug: "canada", Description: "Nigerians in Canada"},
}

func main() {
	if err := run(); err != nil {
		slog.Error("seed failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Source admin credentials from environment variables (AUDIT-BE-017).
	adminEmail := os.Getenv("JC_SEED_ADMIN_EMAIL")
	adminPassword := os.Getenv("JC_SEED_ADMIN_PASSWORD")

	if adminEmail == "" {
		adminEmail = "admin@japaconnect.com"
	}

	if adminPassword == "" {
		if cfg.IsProduction() {
			return errors.New("JC_SEED_ADMIN_PASSWORD must be set in production")
		}

		slog.Warn("JC_SEED_ADMIN_PASSWORD not set, using default dev password")

		adminPassword = "Admin123!"
	}

	ctx := context.Background()

	database, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}

	defer database.Close()

	// Parse the tenant ID once.
	tid := parseUUID("")

	// ── Check if admin already exists (idempotent) ─────────────────────
	var existingID pgtype.UUID

	err = database.Reader.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, adminEmail).Scan(&existingID)

	var userID pgtype.UUID

	if err == nil {
		fmt.Println("seed: admin user already exists, skipping admin creation")

		userID = existingID
	} else {
		// ── Create admin user ──────────────────────────────────────────
		hash, err := auth.HashPassword(adminPassword)
		if err != nil {
			return fmt.Errorf("hashing password: %w", err)
		}

		err = database.Writer.QueryRow(ctx, `INSERT INTO users (email, username, password_hash, profile_type, status) VALUES ($1, $2, $3, $4, 'personal', 'active')
			 RETURNING id`, tid, adminEmail, adminUsername, hash).Scan(&userID)
		if err != nil {
			return fmt.Errorf("creating admin user: %w", err)
		}

		fmt.Printf("seed: created admin user %s (%s)\n", adminEmail, userID.String())

		// ── Create user profile ────────────────────────────────────────
		_, err = database.Writer.Exec(ctx, `INSERT INTO user_profiles (user_id, display_name) VALUES ($1, $2, $3)`, tid, userID, adminDisplayName)
		if err != nil {
			return fmt.Errorf("creating admin profile: %w", err)
		}

		fmt.Println("seed: created admin profile")
	}

	// ── Add admin to system groups ─────────────────────────────────────
	for _, groupPair := range []struct {
		name string
		id   string
	}{
		{name: "super_admin", id: superAdminGroupID},
		{name: "members", id: membersGroupID},
	} {
		gid := parseUUID(groupPair.id)

		_, err = database.Writer.Exec(ctx, `INSERT INTO user_group_memberships (user_id, group_id) VALUES ($1, $2, $3)
			 ON CONFLICT (user_id, group_id) DO NOTHING`, tid, userID, gid)
		if err != nil {
			return fmt.Errorf("adding admin to %s group: %w", groupPair.name, err)
		}

		fmt.Printf("seed: added admin to %s group\n", groupPair.name)
	}

	// ── Create default spaces ──────────────────────────────────────────
	stID := parseUUID(countrySpaceType)

	for _, s := range defaultSpaces {
		var spaceID pgtype.UUID

		err = database.Writer.QueryRow(ctx, `INSERT INTO spaces (
				space_type_id, name, slug, description, level,
				visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status
			) VALUES ($1, $2, $3, $4, $5, 'country',
				'public', 'members', 'members', 'members', 'open', 'active')
			ON CONFLICT (slug) DO UPDATE SET slug = EXCLUDED.slug
			RETURNING id`, tid, stID, s.Name, s.Slug, s.Description).Scan(&spaceID)
		if err != nil {
			return fmt.Errorf("creating space %s: %w", s.Slug, err)
		}

		// Add admin as member of the space.
		_, err = database.Writer.Exec(ctx, `INSERT INTO space_members (space_id, user_id, role) VALUES ($1, $2, $3, 'admin')
			 ON CONFLICT (space_id, user_id) DO NOTHING`, spaceID, userID, tid)
		if err != nil {
			return fmt.Errorf("adding admin to space %s: %w", s.Slug, err)
		}

		fmt.Printf("seed: created space %q (%s)\n", s.Name, spaceID.String())
	}

	// ── Create user preferences ────────────────────────────────────────
	_, err = database.Writer.Exec(ctx, `INSERT INTO user_preferences (user_id, dark_mode, language) VALUES ($1, $2, 'system', 'en-NG')
		 ON CONFLICT (user_id) DO NOTHING`, tid, userID)
	if err != nil {
		return fmt.Errorf("creating user preferences: %w", err)
	}

	fmt.Println("seed: created user preferences")

	// ── Seed demo content ──────────────────────────────────────────────
	if err := seedDemoContent(ctx, database, tid, userID); err != nil {
		return fmt.Errorf("seeding demo content: %w", err)
	}

	fmt.Println("\nseed: complete ✓")

	return nil
}

// seedDemoContent creates demo data across all major features.
func seedDemoContent(ctx context.Context, database *db.DB, tid, adminID pgtype.UUID) error {
	fmt.Println("\nseed: starting demo content seeding...")

	// Step 1: Create demo users
	userIDs, err := seedUsers(ctx, database, tid, adminID)
	if err != nil {
		return fmt.Errorf("seeding users: %w", err)
	}

	// Step 2: Seed tags
	tagIDs, err := seedTags(ctx, database, tid)
	if err != nil {
		return fmt.Errorf("seeding tags: %w", err)
	}

	// Step 3: Seed blog posts
	if err := seedBlogPosts(ctx, database, tid, userIDs, tagIDs); err != nil {
		return fmt.Errorf("seeding blog posts: %w", err)
	}

	// Step 4: Seed forum threads and replies
	if err := seedForumContent(ctx, database, tid, userIDs, tagIDs); err != nil {
		return fmt.Errorf("seeding forum content: %w", err)
	}

	// Step 5: Seed comparisons
	if err := seedComparisons(ctx, database, tid, userIDs); err != nil {
		return fmt.Errorf("seeding comparisons: %w", err)
	}

	// Step 6: Seed organizations
	orgIDs, err := seedOrganizations(ctx, database, tid, userIDs)
	if err != nil {
		return fmt.Errorf("seeding organizations: %w", err)
	}

	// Step 7: Seed reviews
	if err := seedReviews(ctx, database, tid, userIDs, orgIDs); err != nil {
		return fmt.Errorf("seeding reviews: %w", err)
	}

	// Step 8: Seed learning tracks
	if err := seedLearning(ctx, database, tid, userIDs); err != nil {
		return fmt.Errorf("seeding learning: %w", err)
	}

	// Step 9: Seed tickets
	if err := seedTickets(ctx, database, tid, userIDs); err != nil {
		return fmt.Errorf("seeding tickets: %w", err)
	}

	// Step 10: Seed bookmarks, follows, comments
	if err := seedEngagement(ctx, database, tid, userIDs); err != nil {
		return fmt.Errorf("seeding engagement: %w", err)
	}

	fmt.Println("seed: demo content seeding complete")

	return nil
}

// seedUsers creates 8 demo users with profiles.
func seedUsers(ctx context.Context, database *db.DB, tid, adminID pgtype.UUID) ([]pgtype.UUID, error) {
	fmt.Println("seed: creating demo users...")

	var userIDs []pgtype.UUID
	userIDs = append(userIDs, adminID)

	type demoUser struct {
		email       string
		username    string
		displayName string
	}

	users := []demoUser{
		{email: "user1@japaconnect.com", username: "chioma_explorer", displayName: "Chioma Explorer"},
		{email: "user2@japaconnect.com", username: "ade_navigator", displayName: "Ade Navigator"},
		{email: "user3@japaconnect.com", username: "nneka_globetrotter", displayName: "Nneka Globetrotter"},
		{email: "user4@japaconnect.com", username: "emeka_wanderer", displayName: "Emeka Wanderer"},
		{email: "user5@japaconnect.com", username: "ifeoma_pathfinder", displayName: "Ifeoma Pathfinder"},
		{email: "user6@japaconnect.com", username: "obinna_pioneer", displayName: "Obinna Pioneer"},
		{email: "user7@japaconnect.com", username: "amara_settler", displayName: "Amara Settler"},
		{email: "user8@japaconnect.com", username: "chidi_journeyman", displayName: "Chidi Journeyman"},
	}

	hash, err := auth.HashPassword("Demo1234!")
	if err != nil {
		return nil, fmt.Errorf("hashing demo password: %w", err)
	}

	for _, u := range users {
		var uid pgtype.UUID

		err = database.Writer.QueryRow(ctx, `INSERT INTO users (email, username, password_hash, profile_type, status) VALUES ($1, $2, $3, $4, 'personal', 'active')
			 ON CONFLICT (email) DO NOTHING
			 RETURNING id`, tid, u.email, u.username, hash).Scan(&uid)
		if err != nil {
			// If ON CONFLICT fired (no RETURNING), try to fetch existing
			err2 := database.Reader.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, u.email).Scan(&uid)
			if err2 != nil {
				return nil, fmt.Errorf("creating user %s: %w", u.email, err)
			}
		}

		userIDs = append(userIDs, uid)

		// Create profile
		_, _ = database.Writer.Exec(ctx, `INSERT INTO user_profiles (user_id, display_name) VALUES ($1, $2, $3)
			 ON CONFLICT (user_id) DO NOTHING`, tid, uid, u.displayName)

		// Add to members group
		gid := parseUUID(membersGroupID)
		_, _ = database.Writer.Exec(ctx, `INSERT INTO user_group_memberships (user_id, group_id) VALUES ($1, $2, $3)
			 ON CONFLICT (user_id, group_id) DO NOTHING`, tid, uid, gid)

		// Add user preferences
		_, _ = database.Writer.Exec(ctx, `INSERT INTO user_preferences (user_id, dark_mode, language) VALUES ($1, $2, 'system', 'en-NG')
			 ON CONFLICT (user_id) DO NOTHING`, tid, uid)

		fmt.Printf("seed: created user %s (%s)\n", u.email, uid.String())
	}

	return userIDs, nil
}

// seedTags creates 15 tags with relocation/tech topics.
func seedTags(ctx context.Context, database *db.DB, tid pgtype.UUID) ([]pgtype.UUID, error) {
	fmt.Println("seed: creating tags...")

	type tagDef struct {
		name string
		slug string
	}

	tags := []tagDef{
		{name: "Visa Tips", slug: "visa-tips"},
		{name: "UK Healthcare", slug: "uk-healthcare"},
		{name: "Nigerian Food Abroad", slug: "nigerian-food-abroad"},
		{name: "Tech Jobs", slug: "tech-jobs"},
		{name: "Remittance", slug: "remittance"},
		{name: "Housing", slug: "housing"},
		{name: "Education", slug: "education"},
		{name: "Banking", slug: "banking"},
		{name: "Career", slug: "career"},
		{name: "Entrepreneurship", slug: "entrepreneurship"},
		{name: "Culture Shock", slug: "culture-shock"},
		{name: "Driving License", slug: "driving-license"},
		{name: "Insurance", slug: "insurance"},
		{name: "Tax Planning", slug: "tax-planning"},
		{name: "Mental Health", slug: "mental-health"},
	}

	var tagIDs []pgtype.UUID

	for _, t := range tags {
		var tagID pgtype.UUID

		err := database.Writer.QueryRow(ctx, `INSERT INTO tags (name, slug) VALUES ($1, $2, $3)
			 ON CONFLICT (slug) DO NOTHING
			 RETURNING id`, tid, t.name, t.slug).Scan(&tagID)
		if err != nil {
			err2 := database.Reader.QueryRow(ctx, `SELECT id FROM tags WHERE slug = $1`, t.slug).Scan(&tagID)
			if err2 != nil {
				return nil, fmt.Errorf("creating tag %s: %w", t.slug, err)
			}
		}

		tagIDs = append(tagIDs, tagID)
	}

	fmt.Printf("seed: created %d tags\n", len(tags))

	return tagIDs, nil
}

// seedBlogPosts creates 12 blog post contents across spaces.
func seedBlogPosts(ctx context.Context, database *db.DB, tid pgtype.UUID, userIDs, tagIDs []pgtype.UUID) error {
	fmt.Println("seed: creating blog posts...")

	// Get space IDs
	spaceIDs, err := getSpaceIDs(ctx, database, tid)
	if err != nil {
		return err
	}

	type blogDef struct {
		title        string
		slug         string
		body         string
		bodyHTML     string
		spaceIdx     int
		authorIdx    int
		tagIndices   []int
		voteScore    int
		viewCount    int
		commentCount int
	}

	blogs := []blogDef{
		{
			title:        "Complete Guide to UK Skilled Worker Visa 2024",
			slug:         "demo-blog-post-1-uk-skilled-worker-visa",
			body:         "The UK Skilled Worker visa is one of the most popular pathways for Nigerians looking to relocate. This guide covers eligibility requirements, application process, and tips for success. You'll need a job offer from a licensed sponsor, meet the salary threshold, and demonstrate English proficiency. The application fee varies based on duration and whether you're applying from inside or outside the UK.",
			bodyHTML:     "<h2>UK Skilled Worker Visa Guide</h2><p>The UK Skilled Worker visa is one of the most popular pathways for Nigerians looking to relocate. This guide covers eligibility requirements, application process, and tips for success.</p><p>You'll need a job offer from a licensed sponsor, meet the salary threshold, and demonstrate English proficiency. The application fee varies based on duration and whether you're applying from inside or outside the UK.</p>",
			spaceIdx:     1,
			authorIdx:    1,
			tagIndices:   []int{0, 6},
			voteScore:    42,
			viewCount:    1250,
			commentCount: 8,
		},
		{
			title:        "How to Find Affordable Housing in London",
			slug:         "demo-blog-post-2-housing-london",
			body:         "Finding affordable housing in London can be challenging, but with the right approach, it's possible. Start by researching different boroughs, understanding transport links, and setting a realistic budget. Consider shared accommodation initially, explore council housing options, and look into Housing Association properties. Many areas outside Zone 1 offer significantly lower rents while maintaining good connectivity.",
			bodyHTML:     "<h2>Affordable Housing in London</h2><p>Finding affordable housing in London can be challenging, but with the right approach, it's possible.</p><p>Start by researching different boroughs, understanding transport links, and setting a realistic budget. Consider shared accommodation initially, explore council housing options, and look into Housing Association properties.</p>",
			spaceIdx:     1,
			authorIdx:    2,
			tagIndices:   []int{5},
			voteScore:    35,
			viewCount:    980,
			commentCount: 12,
		},
		{
			title:        "Navigating the Canadian Express Entry System",
			slug:         "demo-blog-post-3-canadian-express-entry",
			body:         "Canada's Express Entry system is a points-based immigration pathway that manages applications for three federal economic programs: Federal Skilled Worker, Federal Skilled Trades, and Canadian Experience Class. Understanding the Comprehensive Ranking System (CRS) is crucial for maximizing your chances of receiving an Invitation to Apply (ITA).",
			bodyHTML:     "<h2>Canadian Express Entry System</h2><p>Canada's Express Entry system is a points-based immigration pathway that manages applications for three federal economic programs.</p><p>Understanding the Comprehensive Ranking System (CRS) is crucial for maximizing your chances of receiving an Invitation to Apply (ITA).</p>",
			spaceIdx:     3,
			authorIdx:    3,
			tagIndices:   []int{0, 2},
			voteScore:    28,
			viewCount:    850,
			commentCount: 6,
		},
		{
			title:        "Starting a Tech Career in the US: H1B Visa Guide",
			slug:         "demo-blog-post-4-us-tech-career-h1b",
			body:         "The H1B visa is the primary pathway for skilled workers in specialty occupations to work in the United States. For Nigerian tech professionals, understanding the lottery system, cap gaps, and employer sponsorship requirements is essential. This guide walks through the entire process from finding a sponsoring employer to adjusting status.",
			bodyHTML:     "<h2>US Tech Career: H1B Visa Guide</h2><p>The H1B visa is the primary pathway for skilled workers in specialty occupations to work in the United States.</p><p>For Nigerian tech professionals, understanding the lottery system, cap gaps, and employer sponsorship requirements is essential.</p>",
			spaceIdx:     2,
			authorIdx:    4,
			tagIndices:   []int{3, 0},
			voteScore:    55,
			viewCount:    2100,
			commentCount: 15,
		},
		{
			title:        "Sending Money Home: Best Remittance Options for Nigerians",
			slug:         "demo-blog-post-5-remittance-options",
			body:         "Remitting money to Nigeria from abroad doesn't have to be expensive. Compare the best services including Wise, Remitly, WorldRemit, and traditional bank transfers. We break down fees, exchange rates, transfer speeds, and reliability for each option. Learn how to maximize the value of every transfer.",
			bodyHTML:     "<h2>Best Remittance Options</h2><p>Remitting money to Nigeria from abroad doesn't have to be expensive. Compare the best services including Wise, Remitly, WorldRemit, and traditional bank transfers.</p><p>We break down fees, exchange rates, transfer speeds, and reliability for each option.</p>",
			spaceIdx:     0,
			authorIdx:    5,
			tagIndices:   []int{4, 7},
			voteScore:    63,
			viewCount:    3200,
			commentCount: 20,
		},
		{
			title:        "Understanding NHS Healthcare in the UK",
			slug:         "demo-blog-post-6-nhs-healthcare-uk",
			body:         "The National Health Service (NHS) provides free healthcare to UK residents, but understanding how to access services can be confusing for newcomers. This guide explains GP registration, dental services, emergency care, mental health support, and how the immigration health surcharge works.",
			bodyHTML:     "<h2>Understanding NHS Healthcare</h2><p>The National Health Service (NHS) provides free healthcare to UK residents, but understanding how to access services can be confusing for newcomers.</p><p>This guide explains GP registration, dental services, emergency care, mental health support, and how the immigration health surcharge works.</p>",
			spaceIdx:     1,
			authorIdx:    6,
			tagIndices:   []int{1, 14},
			voteScore:    47,
			viewCount:    1800,
			commentCount: 11,
		},
		{
			title:        "Building Your Emergency Fund as an Immigrant",
			slug:         "demo-blog-post-7-emergency-fund",
			body:         "Financial security starts with an emergency fund. As a Nigerian immigrant, you face unique financial challenges including sending money home, building credit in a new country, and managing unexpected expenses. Learn how to save effectively, where to keep your emergency fund, and how much you really need.",
			bodyHTML:     "<h2>Building Your Emergency Fund</h2><p>Financial security starts with an emergency fund. As a Nigerian immigrant, you face unique financial challenges.</p><p>Learn how to save effectively, where to keep your emergency fund, and how much you really need.</p>",
			spaceIdx:     0,
			authorIdx:    7,
			tagIndices:   []int{7, 13},
			voteScore:    38,
			viewCount:    1100,
			commentCount: 7,
		},
		{
			title:        "Dealing with Culture Shock: A Nigerian's Perspective",
			slug:         "demo-blog-post-8-culture-shock",
			body:         "Culture shock is real and affects everyone differently. From food nostalgia to social norms, weather adjustments to communication styles - this personal account shares practical tips for managing the emotional journey of relocation. You're not alone in feeling overwhelmed.",
			bodyHTML:     "<h2>Dealing with Culture Shock</h2><p>Culture shock is real and affects everyone differently. From food nostalgia to social norms, weather adjustments to communication styles.</p><p>This personal account shares practical tips for managing the emotional journey of relocation. You're not alone in feeling overwhelmed.</p>",
			spaceIdx:     0,
			authorIdx:    1,
			tagIndices:   []int{10, 14},
			voteScore:    72,
			viewCount:    4500,
			commentCount: 25,
		},
		{
			title:        "How to Get a UK Driving License as a New Resident",
			slug:         "demo-blog-post-9-uk-driving-license",
			body:         "Getting a driving license in the UK involves a specific process for overseas license holders. Nigerians can use their existing license for 12 months, but you'll eventually need to pass the UK theory and practical tests. This guide covers the entire process including provisional license applications.",
			bodyHTML:     "<h2>UK Driving License Guide</h2><p>Getting a driving license in the UK involves a specific process for overseas license holders.</p><p>Nigerians can use their existing license for 12 months, but you'll eventually need to pass the UK theory and practical tests.</p>",
			spaceIdx:     1,
			authorIdx:    2,
			tagIndices:   []int{11},
			voteScore:    29,
			viewCount:    720,
			commentCount: 5,
		},
		{
			title:        "Tax Planning for Nigerian Diaspora in the US",
			slug:         "demo-blog-post-10-tax-planning-us",
			body:         "US tax obligations follow you worldwide. As a Nigerian living in the US, you need to understand FBAR requirements, foreign income reporting, tax treaties, and how to avoid double taxation. Proper tax planning can save you thousands and keep you compliant.",
			bodyHTML:     "<h2>Tax Planning for Nigerian Diaspora in the US</h2><p>US tax obligations follow you worldwide. As a Nigerian living in the US, you need to understand FBAR requirements, foreign income reporting, tax treaties, and how to avoid double taxation.</p>",
			spaceIdx:     2,
			authorIdx:    3,
			tagIndices:   []int{13, 7},
			voteScore:    33,
			viewCount:    900,
			commentCount: 9,
		},
		{
			title:        "Starting a Business in Canada as a Nigerian Immigrant",
			slug:         "demo-blog-post-11-business-canada",
			body:         "Canada offers excellent opportunities for immigrant entrepreneurs. Learn about business registration, startup visas, funding options, and how to leverage your Nigerian business experience in the Canadian market. From sole proprietorship to incorporation, we cover all the essentials.",
			bodyHTML:     "<h2>Starting a Business in Canada</h2><p>Canada offers excellent opportunities for immigrant entrepreneurs. Learn about business registration, startup visas, funding options, and how to leverage your Nigerian business experience in the Canadian market.</p>",
			spaceIdx:     3,
			authorIdx:    4,
			tagIndices:   []int{9, 8},
			voteScore:    41,
			viewCount:    1350,
			commentCount: 10,
		},
		{
			title:        "Mental Health Resources for Nigerian Immigrants",
			slug:         "demo-blog-post-12-mental-health",
			body:         "Mental health is just as important as physical health, but stigma often prevents Nigerians from seeking help. This guide covers culturally sensitive therapy options, community support groups, and self-care strategies. Whether you're dealing with homesickness, discrimination, or adjustment stress, help is available.",
			bodyHTML:     "<h2>Mental Health Resources</h2><p>Mental health is just as important as physical health, but stigma often prevents Nigerians from seeking help.</p><p>This guide covers culturally sensitive therapy options, community support groups, and self-care strategies.</p>",
			spaceIdx:     0,
			authorIdx:    5,
			tagIndices:   []int{14, 10},
			voteScore:    58,
			viewCount:    2800,
			commentCount: 18,
		},
	}

	for i, b := range blogs {
		var contentID pgtype.UUID

		// Create content
		err := database.Writer.QueryRow(ctx, `INSERT INTO contents (
				author_id, space_id, content_type, title, slug,
				body, body_html, status, visibility, read_time_minutes,
				publish_at, published_at
			) VALUES ($1, $2, $3, 'blog_post', $4, $5, $6, $7, 'published', 'public', $8, NOW(), NOW())
			ON CONFLICT (slug) DO UPDATE SET slug = EXCLUDED.slug
			RETURNING id`, tid, userIDs[b.authorIdx], spaceIDs[b.spaceIdx], b.title, b.slug, b.body, b.bodyHTML, len(b.body)/200+1).Scan(&contentID)
		if err != nil {
			return fmt.Errorf("creating blog post %d: %w", i+1, err)
		}

		// Create content score (critical for feed!)
		_, err = database.Writer.Exec(ctx, `INSERT INTO content_scores (content_id, vote_score, comment_count, view_count, bookmark_count, velocity_score) VALUES ($1, $2, $3, $4, 0, $5::numeric / GREATEST(EXTRACT(EPOCH FROM (NOW() - (SELECT created_at FROM contents WHERE id = $1))) / 3600, 1))
			 ON CONFLICT (content_id) DO NOTHING`, contentID, b.voteScore, b.commentCount, b.viewCount, b.voteScore)
		if err != nil {
			return fmt.Errorf("creating content score for blog %d: %w", i+1, err)
		}

		// Link tags
		for _, tagIdx := range b.tagIndices {
			if tagIdx < len(tagIDs) {
				_, _ = database.Writer.Exec(ctx, `INSERT INTO content_tags (content_id, tag_id) VALUES ($1, $2)
					 ON CONFLICT (content_id, tag_id) DO NOTHING`, contentID, tagIDs[tagIdx])
			}
		}
	}

	fmt.Printf("seed: created %d blog posts\n", len(blogs))

	return nil
}

// seedForumContent creates forum threads and replies.
func seedForumContent(ctx context.Context, database *db.DB, tid pgtype.UUID, userIDs, tagIDs []pgtype.UUID) error {
	fmt.Println("seed: creating forum threads...")

	spaceIDs, err := getSpaceIDs(ctx, database, tid)
	if err != nil {
		return err
	}

	// Create forum tags
	forumTagIDs, err := seedForumTags(ctx, database, tid)
	if err != nil {
		return err
	}

	type threadDef struct {
		title       string
		slug        string
		body        string
		spaceIdx    int
		authorIdx   int
		solved      bool
		forumTagIdx int
		replies     int
	}

	threads := []threadDef{
		{title: "How long does UK visa processing take in 2024?", slug: "demo-forum-thread-1", body: "I applied for my UK skilled worker visa 3 weeks ago and haven't heard back. Anyone with recent experience on processing times?", spaceIdx: 1, authorIdx: 1, solved: true, forumTagIdx: 0, replies: 4},
		{title: "Best Nigerian restaurants in Toronto", slug: "demo-forum-thread-2", body: "Just moved to Toronto and missing home food badly. Any recommendations for authentic Nigerian restaurants?", spaceIdx: 3, authorIdx: 2, solved: false, forumTagIdx: 1, replies: 3},
		{title: "H1B lottery chances for 2024", slug: "demo-forum-thread-3", body: "What are the realistic chances of getting selected in the H1B lottery this year? I'm a software engineer with 5 years experience.", spaceIdx: 2, authorIdx: 3, solved: false, forumTagIdx: 2, replies: 5},
		{title: "How to open a bank account in the UK", slug: "demo-forum-thread-4", body: "I just arrived in London and need to open a bank account. What documents do I need? Which banks are most immigrant-friendly?", spaceIdx: 1, authorIdx: 4, solved: true, forumTagIdx: 3, replies: 3},
		{title: "Canadian healthcare for new immigrants", slug: "demo-forum-thread-5", body: "Does anyone know how long I have to wait for OHIP coverage after arriving in Ontario? I have a chronic condition that needs monitoring.", spaceIdx: 3, authorIdx: 5, solved: true, forumTagIdx: 4, replies: 2},
		{title: "Dealing with discrimination at work", slug: "demo-forum-thread-6", body: "I've been experiencing subtle discrimination at my workplace. How do you handle this without risking your job or visa status?", spaceIdx: 2, authorIdx: 6, solved: false, forumTagIdx: 5, replies: 4},
		{title: "Best remittance service for US to Nigeria", slug: "demo-forum-thread-7", body: "Which service gives the best exchange rate for sending money to Nigeria from the US? I've been using WorldRemit but looking for alternatives.", spaceIdx: 2, authorIdx: 7, solved: false, forumTagIdx: 3, replies: 3},
		{title: "Navigating the UK housing market", slug: "demo-forum-thread-8", body: "Looking for advice on renting in the UK as a new immigrant. What should I know about tenancy agreements, deposits, and tenant rights?", spaceIdx: 1, authorIdx: 1, solved: false, forumTagIdx: 4, replies: 2},
		{title: "Canadian citizenship application timeline", slug: "demo-forum-thread-9", body: "How long did it take you to get Canadian citizenship after PR? I'm approaching my 3-year mark and wondering about the process.", spaceIdx: 3, authorIdx: 2, solved: false, forumTagIdx: 0, replies: 3},
		{title: "Starting a business in the US as a Nigerian", slug: "demo-forum-thread-10", body: "I want to start a tech consulting business in the US. What visa options are available? Can I use my H1B to start a side business?", spaceIdx: 2, authorIdx: 3, solved: false, forumTagIdx: 2, replies: 4},
		{title: "Mental health support for immigrants", slug: "demo-forum-thread-11", body: "Feeling overwhelmed and homesick. Does anyone know of culturally sensitive therapists who understand the immigrant experience?", spaceIdx: 0, authorIdx: 4, solved: false, forumTagIdx: 5, replies: 3},
		{title: "Getting a UK driving license - tips and tricks", slug: "demo-forum-thread-12", body: "Just passed my UK driving test! Here are my tips for theory and practical exams, especially for those used to driving on the right.", spaceIdx: 1, authorIdx: 5, solved: false, forumTagIdx: 1, replies: 2},
	}

	for i, t := range threads {
		var threadID pgtype.UUID

		err := database.Writer.QueryRow(ctx, `INSERT INTO forum_threads (
				space_id, author_user_id, title, slug, body, solved
			) VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (slug) WHERE deleted_at IS NULL DO UPDATE SET slug = EXCLUDED.slug
			RETURNING id`, tid, spaceIDs[t.spaceIdx], userIDs[t.authorIdx], t.title, t.slug, fmt.Sprintf(`[{"type":"paragraph","content":[{"type":"text","text":"%s"}]}]`, t.body), t.solved).Scan(&threadID)
		if err != nil {
			return fmt.Errorf("creating forum thread %d: %w", i+1, err)
		}

		// Link forum tag
		if t.forumTagIdx < len(forumTagIDs) {
			_, _ = database.Writer.Exec(ctx, `INSERT INTO forum_thread_tags (thread_id, tag_id) VALUES ($1, $2)
				 ON CONFLICT DO NOTHING`, threadID, forumTagIDs[t.forumTagIdx])
		}

		// Create replies
		for j := range t.replies {
			replyAuthorIdx := (t.authorIdx + j + 1) % len(userIDs)
			replyBody := fmt.Sprintf(`[{"type":"paragraph","content":[{"type":"text","text":"This is reply %d to the thread about %s. I hope this helps!"}]}]`, j+1, strings.ToLower(t.title[:min(30, len(t.title))]))

			_, err = database.Writer.Exec(ctx, `INSERT INTO forum_replies (
					thread_id, author_user_id, body
				) VALUES ($1, $2, $3, $4)`, tid, threadID, userIDs[replyAuthorIdx], replyBody)
			if err != nil {
				return fmt.Errorf("creating reply for thread %d: %w", i+1, err)
			}
		}

		// Update reply count
		_, _ = database.Writer.Exec(ctx, `UPDATE forum_threads SET reply_count = $1 WHERE id = $2`, t.replies, threadID)
	}

	fmt.Printf("seed: created %d forum threads with replies\n", len(threads))

	return nil
}

// seedForumTags creates forum tags.
func seedForumTags(ctx context.Context, database *db.DB, tid pgtype.UUID) ([]pgtype.UUID, error) {
	type ftDef struct {
		name string
		slug string
	}

	forumTags := []ftDef{
		{name: "Visa Discussion", slug: "visa-discussion"},
		{name: "Food & Culture", slug: "food-culture"},
		{name: "Tech Careers", slug: "tech-careers"},
		{name: "Banking & Finance", slug: "banking-finance"},
		{name: "Healthcare", slug: "healthcare"},
		{name: "Mental Health", slug: "mental-health"},
	}

	var ids []pgtype.UUID

	for _, ft := range forumTags {
		var id pgtype.UUID

		err := database.Writer.QueryRow(ctx, `INSERT INTO forum_tags (name, slug) VALUES ($1, $2, $3)
			 ON CONFLICT (slug) DO NOTHING
			 RETURNING id`, tid, ft.name, ft.slug).Scan(&id)
		if err != nil {
			err2 := database.Reader.QueryRow(ctx, `SELECT id FROM forum_tags WHERE slug = $1`, ft.slug).Scan(&id)
			if err2 != nil {
				return nil, fmt.Errorf("creating forum tag %s: %w", ft.slug, err)
			}
		}

		ids = append(ids, id)
	}

	return ids, nil
}

// seedComparisons creates 10 comparison contents.
func seedComparisons(ctx context.Context, database *db.DB, tid pgtype.UUID, userIDs []pgtype.UUID) error {
	fmt.Println("seed: creating comparisons...")

	spaceIDs, err := getSpaceIDs(ctx, database, tid)
	if err != nil {
		return err
	}

	type compDef struct {
		title     string
		slug      string
		spaceIdx  int
		authorIdx int
		items     []string
		dims      []string
	}

	comparisons := []compDef{
		{
			title:     "UK vs Canada vs Australia: Best Country for Nigerians",
			slug:      "demo-comparison-1-uk-canada-australia",
			spaceIdx:  0,
			authorIdx: 1,
			items:     []string{"United Kingdom", "Canada", "Australia"},
			dims:      []string{"Cost of Living", "Healthcare Quality", "Weather", "Job Market", "Immigration Ease"},
		},
		{
			title:     "Top 5 Banks for Nigerian Diaspora",
			slug:      "demo-comparison-2-banks-diaspora",
			spaceIdx:  0,
			authorIdx: 2,
			items:     []string{"Barclays UK", "TD Bank Canada", "Chase US", "FirstBank Nigeria", "Wise"},
			dims:      []string{"Transfer Fees", "Exchange Rate", "Customer Service", "Mobile App"},
		},
		{
			title:     "H1B vs O1 vs EB1: US Visa Comparison",
			slug:      "demo-comparison-3-us-visa-types",
			spaceIdx:  2,
			authorIdx: 3,
			items:     []string{"H1B Visa", "O1 Visa", "EB1 Green Card"},
			dims:      []string{"Processing Time", "Cost", "Requirements", "Flexibility", "Path to Green Card"},
		},
		{
			title:     "UK Universities vs Canadian Universities",
			slug:      "demo-comparison-4-uk-canada-universities",
			spaceIdx:  1,
			authorIdx: 4,
			items:     []string{"University of Toronto", "University of Oxford", "McGill University", "University of Edinburgh"},
			dims:      []string{"Tuition Fees", "Living Costs", "Post-Graduation Work", "Research Quality"},
		},
		{
			title:     "Remote Work Platforms for Nigerian Developers",
			slug:      "demo-comparison-5-remote-work-platforms",
			spaceIdx:  0,
			authorIdx: 5,
			items:     []string{"Upwork", "Toptal", "Turing", "Andela", "Freelancer"},
			dims:      []string{"Pay Rate", "Client Quality", "Payment Reliability", "Support"},
		},
		{
			title:     "Healthcare Systems: UK NHS vs Canada OHIP vs US Insurance",
			slug:      "demo-comparison-6-healthcare-systems",
			spaceIdx:  0,
			authorIdx: 6,
			items:     []string{"UK NHS", "Canada OHIP", "US Private Insurance"},
			dims:      []string{"Coverage", "Wait Times", "Cost", "Quality"},
		},
		{
			title:     "Immigration Lawyers: Top Rated for Nigerian Cases",
			slug:      "demo-comparison-7-immigration-lawyers",
			spaceIdx:  0,
			authorIdx: 7,
			items:     []string{"Fragomen", "Berry Appleman & Leiden", "Jackson Lewis"},
			dims:      []string{"Success Rate", "Cost", "Specialization", "Client Reviews"},
		},
		{
			title:     "Best Cities for Nigerian Professionals Abroad",
			slug:      "demo-comparison-8-best-cities",
			spaceIdx:  0,
			authorIdx: 1,
			items:     []string{"London", "Toronto", "New York", "Sydney", "Dubai"},
			dims:      []string{"Job Opportunities", "Nigerian Community", "Cost of Living", "Quality of Life"},
		},
		{
			title:     "Online Learning Platforms for Skill Development",
			slug:      "demo-comparison-9-learning-platforms",
			spaceIdx:  0,
			authorIdx: 2,
			items:     []string{"Coursera", "Udemy", "LinkedIn Learning", "Pluralsight"},
			dims:      []string{"Price", "Course Quality", "Certification Value", "Variety"},
		},
		{
			title:     "Nigerian Food Delivery Services Abroad",
			slug:      "demo-comparison-10-food-delivery",
			spaceIdx:  0,
			authorIdx: 3,
			items:     []string{"AfroBasket UK", "Jumia Food", "UberEats African", "Local Nigerian Shops"},
			dims:      []string{"Authenticity", "Delivery Speed", "Price Range", "Availability"},
		},
	}

	for i, c := range comparisons {
		var contentID pgtype.UUID

		err := database.Writer.QueryRow(ctx, `INSERT INTO contents (
				author_id, space_id, content_type, title, slug,
				body, body_html, status, visibility, read_time_minutes,
				publish_at, published_at
			) VALUES ($1, $2, $3, 'comparison', $4, $5, $6, $7, 'published', 'public', $8, NOW(), NOW())
			ON CONFLICT (slug) DO UPDATE SET slug = EXCLUDED.slug
			RETURNING id`, tid, userIDs[c.authorIdx], spaceIDs[c.spaceIdx], c.title, c.slug, "Comparison of "+c.title, fmt.Sprintf("<h2>%s</h2><p>This comparison helps you make informed decisions.</p>", c.title), 3).Scan(&contentID)
		if err != nil {
			return fmt.Errorf("creating comparison %d: %w", i+1, err)
		}

		// Create content score
		_, _ = database.Writer.Exec(ctx, `INSERT INTO content_scores (content_id, vote_score, comment_count, view_count, bookmark_count, velocity_score) VALUES ($1, $2, 0, $3, 0, 0)
			 ON CONFLICT (content_id) DO NOTHING`, contentID, 20+i*3, 500+i*100)

		// Create comparison items
		itemIDs := make([]pgtype.UUID, len(c.items))

		for j, item := range c.items {
			var itemID pgtype.UUID

			err := database.Writer.QueryRow(ctx, `INSERT INTO comparison_items (content_id, title, position) VALUES ($1, $2, $3, $4)
				 RETURNING id`, contentID, tid, item, j).Scan(&itemID)
			if err != nil {
				return fmt.Errorf("creating comparison item: %w", err)
			}

			itemIDs[j] = itemID
		}

		// Create comparison dimensions
		for k, dim := range c.dims {
			var dimID pgtype.UUID

			err := database.Writer.QueryRow(ctx, `INSERT INTO comparison_dimensions (content_id, label, dimension_type, position) VALUES ($1, $2, $3, 'text', $4)
				 RETURNING id`, contentID, tid, dim, k).Scan(&dimID)
			if err != nil {
				return fmt.Errorf("creating comparison dimension: %w", err)
			}

			// Create values for each item
			for _, itemID := range itemIDs {
				_, _ = database.Writer.Exec(ctx, `INSERT INTO comparison_values (dimension_id, item_id, value) VALUES ($1, $2, $3, 'Good')
					 ON CONFLICT (dimension_id, item_id) DO NOTHING`, dimID, itemID, tid)
			}
		}
	}

	fmt.Printf("seed: created %d comparisons\n", len(comparisons))

	return nil
}

// seedOrganizations creates 5 organizations with members.
func seedOrganizations(ctx context.Context, database *db.DB, tid pgtype.UUID, userIDs []pgtype.UUID) ([]pgtype.UUID, error) {
	fmt.Println("seed: creating organizations...")

	spaceIDs, err := getSpaceIDs(ctx, database, tid)
	if err != nil {
		return nil, err
	}

	type orgDef struct {
		name        string
		slug        string
		description string
		ownerIdx    int
		spaceIdx    int
		orgType     string
	}

	orgs := []orgDef{
		{name: "Nigerian Diaspora Network UK", slug: "demo-org-1-nigerian-diaspora-uk", description: "Connecting Nigerians in the UK for professional networking, cultural events, and community support.", ownerIdx: 1, spaceIdx: 1, orgType: "community"},
		{name: "AfroTech Canada", slug: "demo-org-2-afrotech-canada", description: "Technology community for African professionals in Canada. We host meetups, workshops, and job boards.", ownerIdx: 2, spaceIdx: 3, orgType: "business"},
		{name: "Naija Business Hub", slug: "demo-org-3-naija-business-hub", description: "Supporting Nigerian entrepreneurs in building successful businesses abroad.", ownerIdx: 3, spaceIdx: 0, orgType: "business"},
		{name: "African Students Alliance", slug: "demo-org-4-african-students-alliance", description: "Helping African students navigate education systems abroad.", ownerIdx: 4, spaceIdx: 2, orgType: "ngo"},
		{name: "Nigerian Healthcare Professionals", slug: "demo-org-5-nigerian-healthcare", description: "Network of Nigerian healthcare workers supporting each other in their careers abroad.", ownerIdx: 5, spaceIdx: 1, orgType: "community"},
	}

	var orgIDs []pgtype.UUID

	for _, o := range orgs {
		var orgID pgtype.UUID

		err := database.Writer.QueryRow(ctx, `INSERT INTO organizations (
				owner_user_id, name, slug, description, org_type, geo_space_id
			) VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (slug) DO UPDATE SET slug = EXCLUDED.slug
			RETURNING id`, tid, userIDs[o.ownerIdx], o.name, o.slug, o.description, o.orgType, spaceIDs[o.spaceIdx]).Scan(&orgID)
		if err != nil {
			return nil, fmt.Errorf("creating organization %s: %w", o.slug, err)
		}

		orgIDs = append(orgIDs, orgID)

		// Add owner as member
		_, _ = database.Writer.Exec(ctx, `INSERT INTO organization_members (organization_id, user_id, role, invited_by) VALUES ($1, $2, 'owner', $2)
			 ON CONFLICT (organization_id, user_id) DO NOTHING`, orgID, userIDs[o.ownerIdx])

		// Add 2-3 additional members
		for m := 1; m <= 2; m++ {
			memberIdx := (o.ownerIdx + m) % len(userIDs)
			_, _ = database.Writer.Exec(ctx, `INSERT INTO organization_members (organization_id, user_id, role, invited_by) VALUES ($1, $2, 'member', $3)
				 ON CONFLICT (organization_id, user_id) DO NOTHING`, orgID, userIDs[memberIdx], userIDs[o.ownerIdx])
		}

		// Follow the organization with a few users
		for f := range 3 {
			followerIdx := (o.ownerIdx + f + 3) % len(userIDs)
			_, _ = database.Writer.Exec(ctx, `INSERT INTO follows (follower_id, entity_type, entity_id) VALUES ($1, $2, 'organization', $3)
				 ON CONFLICT (follower_id, entity_type, entity_id) DO NOTHING`, tid, userIDs[followerIdx], orgID)
		}
	}

	fmt.Printf("seed: created %d organizations\n", len(orgs))

	return orgIDs, nil
}

// seedReviews creates 12 reviews on organization entities.
func seedReviews(ctx context.Context, database *db.DB, tid pgtype.UUID, userIDs, orgIDs []pgtype.UUID) error {
	fmt.Println("seed: creating reviews...")

	type reviewDef struct {
		entityIdx int
		reviewIdx int
		rating    int
		title     string
		body      string
	}

	reviews := []reviewDef{
		{entityIdx: 0, reviewIdx: 1, rating: 5, title: "Excellent community support", body: "This organization has been incredibly helpful in my transition to life in the UK. The networking events are fantastic."},
		{entityIdx: 0, reviewIdx: 2, rating: 4, title: "Great resources", body: "Good community with useful resources. Would love to see more events in the north of England."},
		{entityIdx: 1, reviewIdx: 3, rating: 5, title: "Best tech community in Canada", body: "AfroTech Canada has connected me with amazing opportunities. The workshops are top-notch."},
		{entityIdx: 1, reviewIdx: 4, rating: 4, title: "Valuable networking", body: "Great for meeting other African tech professionals. Job board could use more listings."},
		{entityIdx: 2, reviewIdx: 5, rating: 4, title: "Supportive for entrepreneurs", body: "The mentorship program has been invaluable. They really understand the unique challenges we face."},
		{entityIdx: 2, reviewIdx: 6, rating: 5, title: "Life-changing resources", body: "From business registration guidance to funding connections, this hub has everything."},
		{entityIdx: 3, reviewIdx: 7, rating: 4, title: "Helpful for students", body: "The scholarship database and application guidance have been extremely useful."},
		{entityIdx: 3, reviewIdx: 8, rating: 3, title: "Good but needs more", body: "Useful resources but limited to certain universities. Would love broader coverage."},
		{entityIdx: 4, reviewIdx: 9, rating: 5, title: "Professional network", body: "Connecting with fellow Nigerian healthcare professionals has been career-defining."},
		{entityIdx: 4, reviewIdx: 10, rating: 4, title: "Great support system", body: "The mentorship pairing is excellent. Helped me navigate NHS registration."},
		{entityIdx: 0, reviewIdx: 11, rating: 5, title: "Must join for Nigerians in UK", body: "If you're Nigerian in the UK, this is the organization to join. Period."},
		{entityIdx: 1, reviewIdx: 12, rating: 5, title: "Perfect for tech professionals", body: "The job placement support is outstanding. Landed my dream job through their network."},
	}

	for _, r := range reviews {
		if r.entityIdx >= len(orgIDs) || r.reviewIdx >= len(userIDs) {
			continue
		}

		_, err := database.Writer.Exec(ctx, `INSERT INTO reviews (entity_type, entity_id, reviewer_user_id, rating, title, body) VALUES ($1, 'organization', $2, $3, $4, $5, $6)
			 ON CONFLICT (entity_type, entity_id, reviewer_user_id) DO NOTHING`, tid, orgIDs[r.entityIdx], userIDs[r.reviewIdx], r.rating, r.title, r.body)
		if err != nil {
			return fmt.Errorf(" creating review: %w", err)
		}
	}

	// Update review aggregates
	for i, orgID := range orgIDs {
		var avgRating float64

		var reviewCount int

		err := database.Reader.QueryRow(ctx, `SELECT COALESCE(AVG(rating), 0), COUNT(*)
			 FROM reviews
			 WHERE entity_type = 'organization' AND entity_id = $1`, orgID).Scan(&avgRating, &reviewCount)
		if err != nil {
			continue
		}

		_, _ = database.Writer.Exec(ctx, `INSERT INTO review_aggregates (entity_type, entity_id, review_count, average_rating) VALUES ($1, 'organization', $2, $3, $4)
			 ON CONFLICT (entity_type, entity_id)
			 DO UPDATE SET review_count = $3, average_rating = $4, updated_at = NOW()`, tid, orgID, reviewCount, avgRating)

		fmt.Printf("seed: organization %d has %d reviews (avg: %.1f)\n", i+1, reviewCount, avgRating)
	}

	return nil
}

// trackDef defines a learning track for seeding.
type trackDef struct {
	title       string
	slug        string
	description string
	difficulty  string
	modules     []moduleDef
}

// moduleDef defines a learning module for seeding.
type moduleDef struct {
	title   string
	slug    string
	lessons []lessonDef
}

// lessonDef defines a learning lesson for seeding.
type lessonDef struct {
	title   string
	slug    string
	content string
}

// seedLearning creates learning tracks, modules, and lessons.
func seedLearning(ctx context.Context, database *db.DB, tid pgtype.UUID, userIDs []pgtype.UUID) error {
	fmt.Println("seed: creating learning tracks...")

	tracks := []trackDef{
		{
			title:       "UK Visa Application Masterclass",
			slug:        "demo-track-1-uk-visa",
			description: "Complete guide to UK visa applications from start to finish",
			difficulty:  "beginner",
			modules: []moduleDef{
				{title: "Understanding Visa Types", slug: "demo-module-1a", lessons: []lessonDef{
					{title: "Skilled Worker Visa", slug: "demo-lesson-1a-i", content: "The Skilled Worker visa is the most common pathway..."},
					{title: "Student Visa", slug: "demo-lesson-1a-ii", content: "Student visas allow you to study in the UK..."},
				}},
				{title: "Application Process", slug: "demo-module-1b", lessons: []lessonDef{
					{title: "Document Checklist", slug: "demo-lesson-1b-i", content: "Essential documents include passport, bank statements..."},
					{title: "Biometrics Appointment", slug: "demo-lesson-1b-ii", content: "You'll need to attend a biometrics appointment..."},
				}},
			},
		},
		{
			title:       "Canadian Permanent Residency Pathways",
			slug:        "demo-track-2-canadian-pr",
			description: "Navigate the Canadian immigration system",
			difficulty:  "intermediate",
			modules: []moduleDef{
				{title: "Express Entry", slug: "demo-module-2a", lessons: []lessonDef{
					{title: "CRS Score Calculation", slug: "demo-lesson-2a-i", content: "Your Comprehensive Ranking System score..."},
					{title: "Provincial Nominee Programs", slug: "demo-lesson-2a-ii", content: "PNPs can boost your CRS score..."},
				}},
				{title: "Provincial Programs", slug: "demo-module-2b", lessons: []lessonDef{
					{title: "Ontario PNP", slug: "demo-lesson-2b-i", content: "Ontario's Immigrant Nominee Program..."},
					{title: "BC PNP Tech Pilot", slug: "demo-lesson-2b-ii", content: "BC's tech-specific immigration pathway..."},
				}},
			},
		},
		{
			title:       "US H1B Visa Process",
			slug:        "demo-track-3-us-h1b",
			description: "Master the H1B visa application process",
			difficulty:  "intermediate",
			modules: []moduleDef{
				{title: "H1B Basics", slug: "demo-module-3a", lessons: []lessonDef{
					{title: "Eligibility Requirements", slug: "demo-lesson-3a-i", content: "To qualify for H1B..."},
					{title: "Lottery System", slug: "demo-lesson-3a-ii", content: "The H1B lottery is conducted annually..."},
				}},
				{title: "Employer Sponsorship", slug: "demo-module-3b", lessons: []lessonDef{
					{title: "Finding Sponsors", slug: "demo-lesson-3b-i", content: "Look for employers on the H1B sponsor list..."},
					{title: "Labor Condition Application", slug: "demo-lesson-3b-ii", content: "Your employer must file an LCA..."},
				}},
			},
		},
		{
			title:       "Financial Planning for Immigrants",
			slug:        "demo-track-4-financial-planning",
			description: "Build wealth and financial security abroad",
			difficulty:  "beginner",
			modules: []moduleDef{
				{title: "Banking Basics", slug: "demo-module-4a", lessons: []lessonDef{
					{title: "Opening a Bank Account", slug: "demo-lesson-4a-i", content: "Steps to open your first bank account..."},
					{title: "Building Credit", slug: "demo-lesson-4a-ii", content: "Credit scores are crucial in the US/UK/Canada..."},
				}},
				{title: "Investing Abroad", slug: "demo-module-4b", lessons: []lessonDef{
					{title: "Stock Market Basics", slug: "demo-lesson-4b-i", content: "Introduction to investing in foreign markets..."},
					{title: "Retirement Accounts", slug: "demo-lesson-4b-ii", content: "401k, RRSP, pension options..."},
				}},
			},
		},
		{
			title:       "Nigerian Food Recipes Abroad",
			slug:        "demo-track-5-nigerian-food",
			description: "Cook authentic Nigerian meals with ingredients available abroad",
			difficulty:  "beginner",
			modules: []moduleDef{
				{title: "Essential Ingredients", slug: "demo-module-5a", lessons: []lessonDef{
					{title: "Finding Nigerian Ingredients", slug: "demo-lesson-5a-i", content: "Where to find yam, plantain, palm oil..."},
					{title: "Substitutions Guide", slug: "demo-lesson-5a-ii", content: "What to use when ingredients aren't available..."},
				}},
				{title: "Classic Recipes", slug: "demo-module-5b", lessons: []lessonDef{
					{title: "Jollof Rice", slug: "demo-lesson-5b-i", content: "The perfect jollof rice recipe..."},
					{title: "Egusi Soup", slug: "demo-lesson-5b-ii", content: "Making egusi soup abroad..."},
				}},
			},
		},
		{
			title:       "Career Transition Guide",
			slug:        "demo-track-6-career-transition",
			description: "Switch careers successfully in a new country",
			difficulty:  "advanced",
			modules: []moduleDef{
				{title: "Self Assessment", slug: "demo-module-6a", lessons: []lessonDef{
					{title: "Skills Audit", slug: "demo-lesson-6a-i", content: "Identify transferable skills..."},
					{title: "Market Research", slug: "demo-lesson-6a-ii", content: "Research job market demand..."},
				}},
				{title: "Job Search Strategy", slug: "demo-module-6b", lessons: []lessonDef{
					{title: "Resume Optimization", slug: "demo-lesson-6b-i", content: "Tailor your resume for local markets..."},
					{title: "Interview Preparation", slug: "demo-lesson-6b-ii", content: "Common interview questions and answers..."},
				}},
			},
		},
		{
			title:       "UK Housing Guide",
			slug:        "demo-track-7-uk-housing",
			description: "Find and secure housing in the UK",
			difficulty:  "beginner",
			modules: []moduleDef{
				{title: "Renting in the UK", slug: "demo-module-7a", lessons: []lessonDef{
					{title: "Tenant Rights", slug: "demo-lesson-7a-i", content: "Know your rights as a tenant..."},
					{title: "Deposit Protection", slug: "demo-lesson-7a-ii", content: "How deposit protection works..."},
				}},
				{title: "Buying Property", slug: "demo-module-7b", lessons: []lessonDef{
					{title: "Mortgage Basics", slug: "demo-lesson-7b-i", content: "Understanding UK mortgages..."},
					{title: "First Time Buyers", slug: "demo-lesson-7b-ii", content: "Help to Buy and shared ownership..."},
				}},
			},
		},
		{
			title:       "Mental Health & Wellness",
			slug:        "demo-track-8-mental-health",
			description: "Maintain mental health during relocation",
			difficulty:  "beginner",
			modules: []moduleDef{
				{title: "Understanding Culture Shock", slug: "demo-module-8a", lessons: []lessonDef{
					{title: "Stages of Culture Shock", slug: "demo-lesson-8a-i", content: "The four stages of culture shock..."},
					{title: "Coping Strategies", slug: "demo-lesson-8a-ii", content: "How to manage culture shock..."},
				}},
				{title: "Building Support Networks", slug: "demo-module-8b", lessons: []lessonDef{
					{title: "Finding Community", slug: "demo-lesson-8b-i", content: "How to build a support network abroad..."},
					{title: "Professional Help", slug: "demo-lesson-8b-ii", content: "When and how to seek professional help..."},
				}},
			},
		},
		{
			title:       "Canadian Driving License Guide",
			slug:        "demo-track-9-canadian-driving",
			description: "Get your Canadian driving license",
			difficulty:  "beginner",
			modules: []moduleDef{
				{title: "G1 License", slug: "demo-module-9a", lessons: []lessonDef{
					{title: "Written Test Prep", slug: "demo-lesson-9a-i", content: "Preparing for the G1 written test..."},
					{title: "Practice Questions", slug: "demo-lesson-9a-ii", content: "Sample G1 test questions..."},
				}},
				{title: "G2 and Full G", slug: "demo-module-9b", lessons: []lessonDef{
					{title: "Road Test Tips", slug: "demo-lesson-9b-i", content: "Tips for passing your road test..."},
					{title: "Highway Driving", slug: "demo-lesson-9b-ii", content: "Highway driving techniques..."},
				}},
			},
		},
		{
			title:       "Starting a Business Abroad",
			slug:        "demo-track-10-starting-business",
			description: "Launch your business in a new country",
			difficulty:  "advanced",
			modules: []moduleDef{
				{title: "Business Planning", slug: "demo-module-10a", lessons: []lessonDef{
					{title: "Business Plan Template", slug: "demo-lesson-10a-i", content: "Creating a business plan..."},
					{title: "Market Research", slug: "demo-lesson-10a-ii", content: "Conducting market research..."},
				}},
				{title: "Legal Requirements", slug: "demo-module-10b", lessons: []lessonDef{
					{title: "Business Registration", slug: "demo-lesson-10b-i", content: "Registering your business..."},
					{title: "Tax Obligations", slug: "demo-lesson-10b-ii", content: "Understanding tax requirements..."},
				}},
			},
		},
	}

	for i, track := range tracks {
		var trackID pgtype.UUID

		err := database.Writer.QueryRow(ctx, `INSERT INTO learning_tracks (title, slug, description, difficulty, is_published) VALUES ($1, $2, $3, $4, $5, true)
			 ON CONFLICT (slug) DO UPDATE SET slug = EXCLUDED.slug
			 RETURNING id`, tid, track.title, track.slug, track.description, track.difficulty).Scan(&trackID)
		if err != nil {
			return fmt.Errorf("creating learning track %d: %w", i+1, err)
		}

		for j, mod := range track.modules {
			var moduleID pgtype.UUID

			err := database.Writer.QueryRow(ctx, `INSERT INTO learning_modules (track_id, title, slug, sort_order) VALUES ($1, $2, $3, $4, $5)
				 ON CONFLICT (slug) DO UPDATE SET slug = EXCLUDED.slug
				 RETURNING id`, tid, trackID, mod.title, mod.slug, j).Scan(&moduleID)
			if err != nil {
				return fmt.Errorf(" creating module: %w", err)
			}

			for k, lesson := range mod.lessons {
				_, err := database.Writer.Exec(ctx, `INSERT INTO learning_lessons (module_id, title, slug, content, sort_order, is_published) VALUES ($1, $2, $3, $4, $5, $6, true)
					 ON CONFLICT (slug) DO NOTHING`, tid, moduleID, lesson.title, lesson.slug, lesson.content, k)
				if err != nil {
					return fmt.Errorf(" creating lesson: %w", err)
				}
			}
		}
	}

	fmt.Printf("seed: created %d learning tracks\n", len(tracks))

	return nil
}

// seedTickets creates ticket types and tickets.
func seedTickets(ctx context.Context, database *db.DB, tid pgtype.UUID, userIDs []pgtype.UUID) error {
	fmt.Println("seed: creating tickets...")

	// Create ticket types
	type ticketTypeDef struct {
		name string
		slug string
		desc string
	}

	ticketTypes := []ticketTypeDef{
		{name: "General Support", slug: "demo-ticket-type-general", desc: "General support inquiries"},
		{name: "Technical Issue", slug: "demo-ticket-type-technical", desc: "Technical problems and bugs"},
		{name: "Account Help", slug: "demo-ticket-type-account", desc: "Account-related issues"},
	}

	var typeIDs []pgtype.UUID

	for _, tt := range ticketTypes {
		var typeID pgtype.UUID

		err := database.Writer.QueryRow(ctx, `INSERT INTO ticket_types (name, slug, description) VALUES ($1, $2, $3, $4)
			 ON CONFLICT (slug) DO NOTHING
			 RETURNING id`, tid, tt.name, tt.slug, tt.desc).Scan(&typeID)
		if err != nil {
			err2 := database.Reader.QueryRow(ctx, `SELECT id FROM ticket_types WHERE slug = $1`, tt.slug).Scan(&typeID)
			if err2 != nil {
				return fmt.Errorf("creating ticket type: %w", err)
			}
		}

		typeIDs = append(typeIDs, typeID)
	}

	// Create tickets
	type ticketDef struct {
		subject      string
		description  string
		status       string
		priority     string
		typeIdx      int
		submitterIdx int
	}

	tickets := []ticketDef{
		{subject: "Cannot login to my account", description: "I'm getting an error when trying to login. My email is correct but password doesn't seem to work.", status: "open", priority: "high", typeIdx: 2, submitterIdx: 1},
		{subject: "Payment not processing", description: "I tried to purchase credits but the payment failed. My card was charged but credits weren't added.", status: "in_progress", priority: "high", typeIdx: 1, submitterIdx: 2},
		{subject: "How to reset password?", description: "I forgot my password and the reset email isn't arriving. Can you help?", status: "resolved", priority: "normal", typeIdx: 0, submitterIdx: 3},
		{subject: "Feature request: Dark mode", description: "It would be great to have a dark mode option for the platform.", status: "open", priority: "low", typeIdx: 0, submitterIdx: 4},
		{subject: "Bug: Feed not loading", description: "The main feed is showing blank. I've tried refreshing multiple times.", status: "in_progress", priority: "high", typeIdx: 1, submitterIdx: 5},
		{subject: "Can't upload profile picture", description: "When I try to upload a new profile picture, I get a 413 error.", status: "open", priority: "normal", typeIdx: 1, submitterIdx: 6},
		{subject: "Account verification stuck", description: "My email verification has been pending for 3 days. I need access to post.", status: "resolved", priority: "normal", typeIdx: 2, submitterIdx: 7},
		{subject: "How to delete my account?", description: "I want to delete my account permanently. Where is this option?", status: "closed", priority: "low", typeIdx: 2, submitterIdx: 1},
		{subject: "Mobile app crash on Android", description: "The app crashes every time I try to open the learning section on my Samsung S21.", status: "open", priority: "high", typeIdx: 1, submitterIdx: 2},
		{subject: "Need help with visa comparison tool", description: "The comparison tool isn't saving my changes. I've lost my progress twice.", status: "in_progress", priority: "normal", typeIdx: 0, submitterIdx: 3},
	}

	for i, t := range tickets {
		var ticketID pgtype.UUID

		err := database.Writer.QueryRow(ctx, `INSERT INTO tickets (
				type_id, subject, description, status, priority, submitter_user_id
			) VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id`, tid, typeIDs[t.typeIdx], t.subject, t.description, t.status, t.priority, userIDs[t.submitterIdx]).Scan(&ticketID)
		if err != nil {
			return fmt.Errorf("creating ticket %d: %w", i+1, err)
		}

		// Add 2 comments to each ticket
		for c := range 2 {
			commenterIdx := (t.submitterIdx + c + 1) % len(userIDs)

			commentBody := "Thank you for reaching out. We're looking into this issue and will get back to you shortly."
			if c == 1 {
				commentBody = "We've identified the root cause and are working on a fix. Expected resolution time: 24 hours."
			}

			_, _ = database.Writer.Exec(ctx, `INSERT INTO ticket_comments (ticket_id, user_id, body) VALUES ($1, $2, $3)`, ticketID, userIDs[commenterIdx], commentBody)
		}
	}

	fmt.Printf("seed: created %d tickets with comments\n", len(tickets))

	return nil
}

// seedEngagement creates bookmarks, follows, and comments.
func seedEngagement(ctx context.Context, database *db.DB, tid pgtype.UUID, userIDs []pgtype.UUID) error {
	fmt.Println("seed: creating engagement data...")

	// Get all content IDs
	rows, err := database.Reader.Query(ctx, `SELECT id FROM contents WHERE status = 'published' ORDER BY created_at LIMIT 30`)
	if err != nil {
		return fmt.Errorf("querying contents: %w", err)
	}
	defer rows.Close()

	var contentIDs []pgtype.UUID

	for rows.Next() {
		var cid pgtype.UUID
		if err := rows.Scan(&cid); err != nil {
			continue
		}

		contentIDs = append(contentIDs, cid)
	}

	// Create bookmarks (20)
	for i := 0; i < 20 && i < len(contentIDs); i++ {
		userIdx := i%(len(userIDs)-1) + 1
		_, _ = database.Writer.Exec(ctx, `INSERT INTO bookmarks (user_id, entity_type, entity_id) VALUES ($1, $2, 'content', $3)
			 ON CONFLICT (user_id, entity_type, entity_id) DO NOTHING`, tid, userIDs[userIdx], contentIDs[i])
	}

	// Create follows (20)
	for i := range 20 {
		followerIdx := (i % (len(userIDs) - 1)) + 1
		followeeIdx := ((i + 1) % (len(userIDs) - 1)) + 1

		if followerIdx != followeeIdx {
			_, _ = database.Writer.Exec(ctx, `INSERT INTO follows (follower_id, entity_type, entity_id) VALUES ($1, $2, 'user', $3)
				 ON CONFLICT (follower_id, entity_type, entity_id) DO NOTHING`, tid, userIDs[followerIdx], userIDs[followeeIdx])
		}
	}

	// Create comments on blog posts (20)
	blogRows, err := database.Reader.Query(ctx, `SELECT id FROM contents WHERE content_type = 'blog_post' AND status = 'published' LIMIT 12`)
	if err != nil {
		return fmt.Errorf("querying blog posts: %w", err)
	}
	defer blogRows.Close()

	var blogIDs []pgtype.UUID

	for blogRows.Next() {
		var bid pgtype.UUID
		if err := blogRows.Scan(&bid); err != nil {
			continue
		}

		blogIDs = append(blogIDs, bid)
	}

	commentBodies := []string{
		"Thanks for sharing this! Very helpful information.",
		"Great article! I've been looking for this kind of resource.",
		"Really useful tips. Bookmarked for future reference.",
		"I had a similar experience. This resonates deeply.",
		"Excellent guide! Would love to see more content like this.",
		"This is exactly what I needed to read today. Thank you!",
		"Very informative. I'll share this with my network.",
		"Great perspective on this topic. Keep up the good work!",
	}

	for i := range 20 {
		blogIdx := i % len(blogIDs)
		authorIdx := (i % (len(userIDs) - 1)) + 1
		bodyIdx := i % len(commentBodies)

		_, _ = database.Writer.Exec(ctx, `INSERT INTO comments (content_id, author_user_id, body) VALUES ($1, $2, $3, $4)`, tid, blogIDs[blogIdx], userIDs[authorIdx], commentBodies[bodyIdx])
	}

	fmt.Println("seed: created engagement data")

	return nil
}

// getSpaceIDs returns all space IDs for the tenant.
func getSpaceIDs(ctx context.Context, database *db.DB, tid pgtype.UUID) ([]pgtype.UUID, error) {
	rows, err := database.Reader.Query(ctx, `SELECT id FROM spaces  ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("querying spaces: %w", err)
	}
	defer rows.Close()

	var ids []pgtype.UUID

	for rows.Next() {
		var id pgtype.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}

		ids = append(ids, id)
	}

	return ids, nil
}

// parseUUID wraps a hardcoded UUID string into pgtype.UUID.
// Panics on bad input — these are compile-time constants that must be valid.
func parseUUID(s string) pgtype.UUID {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		panic("seed: invalid UUID constant " + s + ": " + err.Error())
	}

	return u
}
