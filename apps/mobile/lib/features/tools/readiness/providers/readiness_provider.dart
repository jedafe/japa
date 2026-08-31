import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/tools/readiness/services/readiness_service.dart';

final readinessServiceProvider = Provider<ReadinessService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return ReadinessService(client);
});

/// Active assessment form + scoring metadata.
final readinessProvider =
    FutureProvider.autoDispose<Map<String, dynamic>>((ref) async {
  final service = ref.watch(readinessServiceProvider);
  return service.getAssessment();
});

/// Authenticated user's assessment history. Fails gracefully for guests.
final readinessHistoryProvider =
    FutureProvider.autoDispose<List<dynamic>>((ref) async {
  final service = ref.watch(readinessServiceProvider);
  final auth = ref.watch(isAuthenticatedProvider);
  if (!auth) return [];
  return service.getHistory();
});
