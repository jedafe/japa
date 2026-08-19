import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/tools/scholarships/services/scholarships_service.dart';

final scholarshipsServiceProvider = Provider<ScholarshipsService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return ScholarshipsService(client);
});

final scholarshipsProvider =
    FutureProvider.autoDispose<List<Map<String, dynamic>>>((ref) async {
  final service = ref.watch(scholarshipsServiceProvider);
  return service.listScholarships();
});
