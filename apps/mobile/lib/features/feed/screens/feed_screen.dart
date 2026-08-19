import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/feed/providers/feed_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class FeedScreen extends ConsumerWidget {
  const FeedScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    ref.watch(feedRealtimeSyncProvider); // keep realtime listener alive
    final feed = ref.watch(feedProvider);
    final t = ref.watch(tProvider);
    return Scaffold(
      appBar: AppBar(title: Text(t('feed.title'))),
      body: RefreshIndicator(
        onRefresh: () async => ref.invalidate(feedProvider),
        child: feed.when(
          data: (items) => ListView.builder(
            itemCount: items.length,
            itemBuilder: (_, i) => FeedCard(item: items[i]),
          ),
          loading: () => const LoadingShimmer(),
          error: (e, _) => JcErrorWidget(error: e, onRetry: () => ref.invalidate(feedProvider)),
        ),
      ),
    );
  }
}
