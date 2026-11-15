import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import '../i18n/app_localizations.dart';

/// Fetches GET /feature-flags once for the shell — used by the fallback
/// tabset to hide nav tabs whose backing feature is disabled (§42).
final appShellFlagsProvider =
    FutureProvider.autoDispose<FeatureFlags>((ref) async {
  final client = ref.watch(jcApiClientProvider);
  return client.get<FeatureFlags>(
    '/feature-flags',
    parse: (json) => FeatureFlags.fromJson(json as Map<String, dynamic>),
  );
});

/// GET /menus/mobile_bottom_bar — the admin-managed tab bar (ADR-054).
/// Visibility, feature flags, and ADR-128 intent hide/pin are already
/// resolved server-side (the client sends X-JC-Surface: mobile), so the
/// shell renders the items as received. Empty/missing menu → the built-in
/// §23 tabset below.
final bottomBarMenuProvider = FutureProvider.autoDispose<JcMenu?>((ref) async {
  final client = ref.watch(jcApiClientProvider);
  try {
    return await client.get<JcMenu>(
      '/menus/mobile_bottom_bar',
      parse: (json) => JcMenu.fromJson(json as Map<String, dynamic>),
    );
  } catch (_) {
    return null;
  }
});

/// The literal url 'compose' marks the compose slot in the admin-managed
/// menu — same convention as the web MobileBottomTabBar.
const _composeUrl = 'compose';

/// Icon names are the backend's kebab-case menu icons (menu_items.icon),
/// shared with the web icon map in apps/main/src/lib/components/ui/icons.ts.
const _iconMap = <String, IconData>{
  'bell': Icons.notifications,
  'compass': Icons.explore,
  'globe': Icons.public,
  'home': Icons.home,
  'log-in': Icons.login,
  'mail': Icons.mail,
  'message-square': Icons.chat_bubble_outline,
  'search': Icons.search,
  'user': Icons.person,
  'users': Icons.group,
};

/// Web routes that have a differently-named mobile equivalent. Menu urls
/// are authored against the web site; the shell remaps the known deltas.
const _pathAliases = <String, String>{
  '/explore': '/discover',
  '/search': '/discover',
};

String _mobilePath(String url) => _pathAliases[url] ?? url;

bool _matches(String menuUrl, String currentPath) {
  final url = _mobilePath(menuUrl);
  if (url == '/') {
    return currentPath == '/' ||
        currentPath == '/discover' ||
        currentPath == '/trending';
  }
  return currentPath == url || currentPath.startsWith('$url/');
}

class AppShell extends ConsumerWidget {
  final Widget child;

  const AppShell({super.key, required this.child});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final t = ref.watch(tProvider);
    final flags = ref.watch(appShellFlagsProvider);
    final menu = ref.watch(bottomBarMenuProvider).value;
    // Show the tab while flags are still loading (avoids a layout jump),
    // hide it only once the flag is known to be off.
    final notificationsEnabled =
        flags.value?.isEnabled('feature.notifications') ?? true;

    final menuItems =
        (menu?.items ?? []).where((i) => i.children.isEmpty).toList();

    final destinations = <_NavDestination>[
      if (menuItems.isNotEmpty)
        for (final i in menuItems)
          _NavDestination(
            icon: i.url == _composeUrl
                ? Icons.add_circle
                : (_iconMap[i.icon] ?? Icons.circle_outlined),
            label: i.label,
            path: i.url == _composeUrl ? '/compose' : _mobilePath(i.url),
            isPush: i.url == _composeUrl,
            matches: (p) =>
                i.url == _composeUrl ? p == '/compose' : _matches(i.url, p),
          )
      else ...[
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
      ],
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
