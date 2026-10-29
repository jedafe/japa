// Hide Flutter's Notification — the japaconnect_core model is the one used here.
import 'package:flutter/material.dart' hide Notification;
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/social/providers/social_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class NotificationsScreen extends ConsumerWidget {
  const NotificationsScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    ref.watch(socialRealtimeSyncProvider); // keep realtime listener alive
    final state = ref.watch(notificationsProvider);
    final t = ref.watch(tProvider);
    return Scaffold(
      appBar: AppBar(
        title: Text(t('social.notifications')),
        actions: [
          // BUG-024: mark-all-read affordance (service already supports it).
          IconButton(
            icon: const Icon(Icons.done_all),
            tooltip: t('notifications.mark_all_read'),
            onPressed: () => ref
                .read(socialServiceProvider)
                .markAllNotificationsRead()
                .then((_) => ref.invalidate(notificationsProvider)),
          ),
        ],
      ),
      body: state.when(
        data: (items) => ListView.builder(
          itemCount: items.length,
          itemBuilder: (_, i) {
            final n = items[i];
            final unread = !(n.isRead ?? false);
            return ListTile(
              // BUG-024: tap marks the notification read AND deep-links to
              // its action_url destination when a mobile route exists —
              // matches the web notification centre's open behaviour.
              onTap: () => _openNotification(context, ref, n),
              leading: unread
                  ? Icon(Icons.circle,
                      size: 10, color: Theme.of(context).colorScheme.primary)
                  : const SizedBox(width: 10),
              title: Text(
                n.title ?? n.typeLabel ?? '',
                style: TextStyle(
                    fontWeight: unread ? FontWeight.w600 : FontWeight.normal),
              ),
              subtitle: (n.body?.isNotEmpty ?? false) ? Text(n.body!) : null,
              trailing: unread
                  ? IconButton(
                      icon: const Icon(Icons.check),
                      onPressed: () => ref
                          .read(socialServiceProvider)
                          .markNotificationRead(n.id)
                          .then((_) => ref.invalidate(notificationsProvider)),
                    )
                  : null,
            );
          },
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
            error: e, onRetry: () => ref.invalidate(notificationsProvider)),
      ),
    );
  }

  /// Tap-through: mark read (when unread) then navigate to the mapped route.
  void _openNotification(
      BuildContext context, WidgetRef ref, Notification n) {
    if (!(n.isRead ?? false)) {
      ref
          .read(socialServiceProvider)
          .markNotificationRead(n.id)
          .then((_) => ref.invalidate(notificationsProvider));
    }
    final route = _resolveNotificationRoute(n.actionUrl);
    if (route != null && context.mounted) context.push(route);
  }

  /// BUG-024: map a web action_url to a mobile GoRouter path.
  /// Web and mobile share most first segments; a few web-only shapes get
  /// explicit translations. Returns null when no mobile route exists —
  /// the row still marks read in that case.
  static String? _resolveNotificationRoute(String? actionUrl) {
    if (actionUrl == null || actionUrl.isEmpty || !actionUrl.startsWith('/')) {
      return null;
    }
    final uri = Uri.tryParse(actionUrl);
    if (uri == null) return null;
    final path = uri.path;

    // Web-only shapes → mobile equivalents.
    if (path == '/messages/requests') return '/messages';
    if (path == '/messages') {
      final c = uri.queryParameters['c'];
      return (c != null && c.isNotEmpty) ? '/messages/$c' : '/messages';
    }
    if (path == '/' || path == '/feed' || path == '/home') return '/';
    if (path == '/wallet') return '/profile/wallet';
    if (path == '/bookmarks') return '/profile/bookmarks';
    if (path == '/me' ||
        path.startsWith('/me/') ||
        path == '/settings' ||
        path.startsWith('/settings/')) {
      return '/profile/settings';
    }
    if (path.startsWith('/profile/')) return '/profile';

    // First segments that share the same path shape on web and mobile.
    const shared = {
      '/notifications', '/discover', '/trending', '/compose', '/spaces',
      '/blog', '/guides', '/forum', '/comparisons', '/tools', '/academy',
      '/leaderboard', '/jobs', '/profile',
    };
    final segs = path.split('/').where((s) => s.isNotEmpty).toList();
    if (segs.isEmpty) return '/';
    if (shared.contains('/${segs.first}')) return path;
    return null;
  }
}
