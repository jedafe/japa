import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import '../i18n/app_localizations.dart';

/// Fetches GET /feature-flags once for the shell — used to hide nav tabs
/// whose backing feature is disabled (§42).
final appShellFlagsProvider =
    FutureProvider.autoDispose<FeatureFlags>((ref) async {
  final client = ref.watch(jcApiClientProvider);
  return client.get<FeatureFlags>(
    '/feature-flags',
    parse: (json) => FeatureFlags.fromJson(json as Map<String, dynamic>),
  );
});

class AppShell extends ConsumerWidget {
  final Widget child;

  const AppShell({super.key, required this.child});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final t = ref.watch(tProvider);
    final flags = ref.watch(appShellFlagsProvider);
    // Show the tab while flags are still loading (avoids a layout jump),
    // hide it only once the flag is known to be off.
    final notificationsEnabled =
        flags.value?.isEnabled('feature.notifications') ?? true;

    final destinations = <_NavDestination>[
      _NavDestination(
        icon: Icons.home,
        label: t('nav.home'),
        path: '/',
        matches: (p) => p == '/' || p == '/discover' || p == '/trending',
      ),
      _NavDestination(
        icon: Icons.group,
        label: t('nav.spaces'),
        path: '/spaces',
        matches: (p) => p.startsWith('/spaces'),
      ),
      _NavDestination(
        icon: Icons.add_circle,
        label: t('nav.compose'),
        path: '/compose',
        isPush: true,
        matches: (p) => p == '/compose',
      ),
      if (notificationsEnabled)
        _NavDestination(
          icon: Icons.notifications,
          label: t('nav.notifications'),
          path: '/notifications',
          matches: (p) => p.startsWith('/notifications'),
        ),
      _NavDestination(
        icon: Icons.person,
        label: t('nav.profile'),
        path: '/profile',
        matches: (p) => p.startsWith('/profile'),
      ),
    ];

    final currentPath = GoRouterState.of(context).uri.path;
    var currentIndex = destinations.indexWhere((d) => d.matches(currentPath));
    if (currentIndex == -1) currentIndex = 0;

    return Scaffold(
      body: child,
      bottomNavigationBar: BottomNavigationBar(
        currentIndex: currentIndex,
        onTap: (i) {
          final dest = destinations[i];
          if (dest.isPush) {
            context.push(dest.path);
          } else {
            context.go(dest.path);
          }
        },
        items: [
          for (final d in destinations)
            BottomNavigationBarItem(icon: Icon(d.icon), label: d.label),
        ],
      ),
    );
  }
}

class _NavDestination {
  final IconData icon;
  final String label;
  final String path;
  final bool isPush;
  final bool Function(String) matches;

  const _NavDestination({
    required this.icon,
    required this.label,
    required this.path,
    required this.matches,
    this.isPush = false,
  });
}
