// Ref: japaconnect-engineering/sections/81-scam-alert-board.md
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/scam_alerts/services/scam_alerts_service.dart';

final scamAlertsServiceProvider = Provider<ScamAlertsService>((ref) {
  return ScamAlertsService(ref.watch(jcApiClientProvider));
});

final scamAlertsProvider = FutureProvider.autoDispose<List<ScamAlertItem>>((ref) async {
  return ref.watch(scamAlertsServiceProvider).list();
});

final scamAlertDetailProvider =
    FutureProvider.autoDispose.family<ScamAlertItem, String>((ref, id) async {
  return ref.watch(scamAlertsServiceProvider).get(id);
});

/// Canonical enum options for the report form (Rule 16 — no hardcoded lists).
final scamAlertSchemaProvider =
    FutureProvider.autoDispose<Map<String, List<Map<String, String>>>>((ref) async {
  final svc = ref.watch(scamAlertsServiceProvider);
  final types = await svc.schemaOptions('scam_types');
  final platforms = await svc.schemaOptions('platforms');
  return {'scam_types': types, 'platforms': platforms};
});
