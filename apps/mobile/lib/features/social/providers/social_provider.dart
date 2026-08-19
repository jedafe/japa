import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/social/services/social_service.dart';

final socialServiceProvider = Provider<SocialService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return SocialService(client);
});

final notificationsProvider =
    FutureProvider.autoDispose<List<Map<String, dynamic>>>((ref) async {
  final service = ref.watch(socialServiceProvider);
  return service.listNotifications();
});

final conversationsProvider =
    FutureProvider.autoDispose<List<Map<String, dynamic>>>((ref) async {
  final service = ref.watch(socialServiceProvider);
  return service.listConversations();
});

final conversationMessagesProvider = FutureProvider.autoDispose
    .family<List<Map<String, dynamic>>, String>((ref, id) async {
  final service = ref.watch(socialServiceProvider);
  return service.getConversationMessages(id);
});

/// Listens to [realtimeEventsProvider] and [pushServiceProvider.foregroundMessages]
/// and invalidates the relevant providers for `notification` and `dm` events.
/// For `dm` events, also invalidates the specific
/// `conversationMessagesProvider(conversationId)` when the payload includes a
/// `conversation_id`. The stream subscriptions are cancelled when this provider
/// is disposed.
final socialRealtimeSyncProvider = Provider<void>((ref) {
  ref.listen(realtimeEventsProvider, (previous, next) {
    next.whenData((event) {
      final type = event['type'] as String?;
      switch (type) {
        case 'notification':
          ref.invalidate(notificationsProvider);
          break;
        case 'dm':
          ref.invalidate(conversationsProvider);
          final conversationId = event['conversation_id'] as String?;
          if (conversationId != null) {
            ref.invalidate(conversationMessagesProvider(conversationId));
          }
          break;
      }
    });
  });

  final pushSub = ref.watch(pushServiceProvider).foregroundMessages.listen((_) {
    ref.invalidate(notificationsProvider);
  });
  ref.onDispose(pushSub.cancel);
});
