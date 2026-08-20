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
          itemBuilder: (_, i) => ListTile(
            title: Text(items[i].title ?? ''),
            trailing: IconButton(
              icon: const Icon(Icons.check),
              onPressed: () => ref
                  .read(socialServiceProvider)
                  .markNotificationRead(items[i].id),
            ),
          ),
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
            error: e, onRetry: () => ref.invalidate(notificationsProvider)),
      ),
    );
  }
}
