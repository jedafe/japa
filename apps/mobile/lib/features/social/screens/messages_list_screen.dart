import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/social/providers/social_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class MessagesListScreen extends ConsumerWidget {
  const MessagesListScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    ref.watch(socialRealtimeSyncProvider); // keep realtime listener alive
    final state = ref.watch(conversationsProvider);
    final t = ref.watch(tProvider);
    return Scaffold(
      appBar: AppBar(title: Text(t('social.messages'))),
      body: state.when(
        data: (items) => ListView.builder(
          itemCount: items.length,
          itemBuilder: (_, i) => ListTile(
            title: Text(items[i]['title'] ??
                items[i]['with_user']?['username'] ??
                t('social.conversation')),
            onTap: () => context.push('/messages/${items[i]['id']}'),
          ),
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
            error: e, onRetry: () => ref.invalidate(conversationsProvider)),
      ),
    );
  }
}
