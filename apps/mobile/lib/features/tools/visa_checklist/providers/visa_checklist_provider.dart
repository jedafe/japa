import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/tools/visa_checklist/services/visa_checklist_service.dart';

final visaChecklistServiceProvider = Provider<VisaChecklistService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return VisaChecklistService(client);
});

final visaChecklistProvider =
    FutureProvider.autoDispose<List<Map<String, dynamic>>>((ref) async {
  final service = ref.watch(visaChecklistServiceProvider);
  return service.listChecklists();
});
