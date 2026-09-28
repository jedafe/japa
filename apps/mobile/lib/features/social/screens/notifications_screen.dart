import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
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
      appBar: AppBar(title: Text(t('social.notifications'))),
      body: state.when(
        data: (items) => ListView.builder(
          itemCount: items.length,
          itemBuilder: (_, i) {
            final n = items[i];
            final unread = !(n.isRead ?? false);
            return ListTile(
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
}
