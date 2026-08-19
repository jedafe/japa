import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'auth_guard.dart';
import 'app_shell.dart';
import '../features/auth/screens/login_screen.dart';
import '../features/auth/screens/register_screen.dart';
import '../features/auth/screens/totp_screen.dart';
import '../features/feed/screens/feed_screen.dart';
import '../features/feed/screens/discover_screen.dart';
import '../features/feed/screens/trending_screen.dart';
import '../features/content/screens/blog_list_screen.dart';
import '../features/content/screens/blog_detail_screen.dart';
import '../features/content/screens/guide_list_screen.dart';
import '../features/content/screens/guide_detail_screen.dart';
import '../features/content/screens/comparison_list_screen.dart';
import '../features/content/screens/comparison_detail_screen.dart';
import '../features/forum/screens/forum_list_screen.dart';
import '../features/forum/screens/forum_detail_screen.dart';
import '../features/forum/screens/forum_reply_screen.dart';
import '../features/tools/japabot/screens/japabot_screen.dart';
import '../features/spaces/screens/space_list_screen.dart';
import '../features/spaces/screens/space_detail_screen.dart';
import '../features/social/screens/notifications_screen.dart';
import '../features/social/screens/messages_list_screen.dart';
import '../features/social/screens/message_thread_screen.dart';
import '../features/learning/screens/course_list_screen.dart';
import '../features/learning/screens/course_detail_screen.dart';
import '../features/learning/screens/lesson_player_screen.dart';
import '../features/profile/screens/profile_screen.dart';
import '../features/profile/screens/settings_screen.dart';
import '../features/profile/screens/wallet_screen.dart';
import '../features/profile/screens/bookmarks_screen.dart';
import '../features/gamification/screens/badges_screen.dart';
import '../features/gamification/screens/leaderboard_screen.dart';
import '../features/jobs/screens/jobs_list_screen.dart';
import '../features/jobs/screens/job_detail_screen.dart';

final jcRouterProvider = Provider<GoRouter>((ref) {
  final loggedIn = ref.watch(isAuthenticatedProvider);
  return GoRouter(
    initialLocation: '/',
    redirect: (context, state) => AuthGuard.redirect(loggedIn, state),
    routes: [
      GoRoute(
        path: '/login',
        builder: (_, _) => const LoginScreen(),
      ),
      GoRoute(path: '/register', builder: (_, _) => const RegisterScreen()),
      GoRoute(path: '/totp', builder: (_, _) => const TOTPScreen()),
      ShellRoute(
        builder: (_, _, child) => AppShell(child: child),
        routes: [
          GoRoute(path: '/', builder: (_, _) => const FeedScreen()),
          GoRoute(path: '/discover', builder: (_, _) => const DiscoverScreen()),
          GoRoute(path: '/trending', builder: (_, _) => const TrendingScreen()),
        ],
      ),
      GoRoute(path: '/blog', builder: (_, _) => const BlogListScreen()),
      GoRoute(
        path: '/blog/:slug',
        builder: (ctx, state) =>
            BlogDetailScreen(slug: state.pathParameters['slug']!),
      ),
      GoRoute(path: '/guides', builder: (_, _) => const GuideListScreen()),
      GoRoute(
        path: '/guides/:slug',
        builder: (ctx, state) =>
            GuideDetailScreen(slug: state.pathParameters['slug']!),
      ),
      GoRoute(
          path: '/comparisons',
          builder: (_, _) => const ComparisonListScreen()),
      GoRoute(
        path: '/comparisons/:id',
        builder: (ctx, state) =>
            ComparisonDetailScreen(id: state.pathParameters['id']!),
      ),
      GoRoute(path: '/forum', builder: (_, _) => const ForumListScreen()),
      GoRoute(
        path: '/forum/:id',
        builder: (ctx, state) =>
            ForumDetailScreen(slug: state.pathParameters['id']!),
      ),
      GoRoute(
        path: '/forum/:id/reply',
        builder: (ctx, state) =>
            ForumReplyScreen(slug: state.pathParameters['id']!),
      ),
      GoRoute(
          path: '/spaces', builder: (_, _) => const SpaceListScreen()),
      GoRoute(
        path: '/spaces/:slug',
        builder: (ctx, state) =>
            SpaceDetailScreen(slug: state.pathParameters['slug']!),
      ),
      GoRoute(
        path: '/tools/japabot',
        builder: (_, _) => const JapaBotScreen(),
      ),
      GoRoute(
          path: '/notifications',
          builder: (_, _) => const NotificationsScreen()),
      GoRoute(
          path: '/messages',
          builder: (_, _) => const MessagesListScreen()),
      GoRoute(
        path: '/messages/:id',
        builder: (ctx, state) =>
            MessageThreadScreen(id: state.pathParameters['id']!),
      ),
      GoRoute(
        path: '/learn/university/catalog',
        builder: (_, _) => const CourseListScreen(),
      ),
      GoRoute(
        path: '/learn/university/catalog/:slug',
        builder: (ctx, state) =>
            CourseDetailScreen(slug: state.pathParameters['slug']!),
      ),
      GoRoute(
        path:
            '/learn/tracks/:trackSlug/modules/:moduleSlug/lessons/:lessonSlug',
        builder: (ctx, state) => LessonPlayerScreen(
          trackSlug: state.pathParameters['trackSlug']!,
          moduleSlug: state.pathParameters['moduleSlug']!,
          lessonSlug: state.pathParameters['lessonSlug']!,
        ),
      ),
      GoRoute(path: '/profile', builder: (_, _) => const ProfileScreen()),
      GoRoute(
          path: '/profile/settings',
          builder: (_, _) => const SettingsScreen()),
      GoRoute(
          path: '/profile/wallet',
          builder: (_, _) => const WalletScreen()),
      GoRoute(
          path: '/profile/bookmarks',
          builder: (_, _) => const BookmarksScreen()),
      GoRoute(
          path: '/profile/badges',
          builder: (_, _) => const BadgesScreen()),
      GoRoute(
          path: '/leaderboard',
          builder: (_, _) => const LeaderboardScreen()),
      GoRoute(path: '/jobs', builder: (_, _) => const JobsListScreen()),
      GoRoute(
        path: '/jobs/:slug',
        builder: (ctx, state) =>
            JobDetailScreen(slug: state.pathParameters['slug']!),
      ),
    ],
  );
});
