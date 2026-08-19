import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/tools/japabot/services/japabot_service.dart';

final japabotServiceProvider = Provider<JapaBotService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return JapaBotService(client);
});

final japabotConversationsProvider =
    FutureProvider.autoDispose<List<Map<String, dynamic>>>((ref) async {
  final service = ref.watch(japabotServiceProvider);
  return service.listConversations();
});
