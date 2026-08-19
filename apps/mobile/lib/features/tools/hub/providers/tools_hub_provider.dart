import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/tools/hub/services/tools_hub_service.dart';

final toolsHubServiceProvider = Provider<ToolsHubService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return ToolsHubService(client);
});

final toolsHubProvider =
    FutureProvider.autoDispose<Map<String, dynamic>>((ref) async {
  final service = ref.watch(toolsHubServiceProvider);
  return service.listFeatureFlags();
});
