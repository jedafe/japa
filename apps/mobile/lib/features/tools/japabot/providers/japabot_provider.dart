import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/tools/japabot/services/japabot_service.dart';

final japabotServiceProvider = Provider<JapaBotService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return JapaBotService(client);
});

final japabotConversationsProvider =
    FutureProvider.autoDispose<List<JapaBotConversation>>((ref) async {
  final service = ref.watch(japabotServiceProvider);
  return service.listConversations();
});

final japabotMessagesProvider = FutureProvider.autoDispose
    .family<List<JapaBotMessage>, String>((ref, id) async {
  // 'new' is the client-side sentinel for a thread that doesn't exist yet —
  // the server creates it on the first streamed turn and returns the id in
  // the `conversation` event.
  if (id.isEmpty || id == 'new') return <JapaBotMessage>[];
  final service = ref.watch(japabotServiceProvider);
  final detail = await service.getConversation(id);
  return detail.messages;
});

/// The caller's tool catalog (ADR-130) — empty → the chat screen hides the
/// tools toggle and turns stay pure chat.
final japabotCapabilitiesProvider =
    FutureProvider.autoDispose<List<Map<String, dynamic>>>((ref) async {
  final service = ref.watch(japabotServiceProvider);
  return service.getCapabilities();
});
