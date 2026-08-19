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
    ],
  );
});
