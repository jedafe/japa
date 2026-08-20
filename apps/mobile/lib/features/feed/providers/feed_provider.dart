import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/feed/services/feed_service.dart';

final feedServiceProvider = Provider<FeedService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return FeedService(client);
});

final feedProvider = FutureProvider.autoDispose<List<Content>>((ref) async {
  final service = ref.watch(feedServiceProvider);
  return service.fetchFeed();
});

/// Listens to [realtimeEventsProvider] and invalidates [feedProvider] on
/// `feed_update` events. Keep this provider alive by watching it in a long-lived
/// widget (e.g., the app shell or [FeedScreen]) so the listener survives rebuilds.
final feedRealtimeSyncProvider = Provider<void>((ref) {
  ref.listen(realtimeEventsProvider, (previous, next) {
    next.whenData((event) {
      if (event['type'] == 'feed_update') {
        ref.invalidate(feedProvider);
      }
    });
  });
});
