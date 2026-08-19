import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/tools/remittance/services/remittance_service.dart';

final remittanceServiceProvider = Provider<RemittanceService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return RemittanceService(client);
});

final remittanceProvidersProvider =
    FutureProvider.autoDispose<List<Map<String, dynamic>>>((ref) async {
  final service = ref.watch(remittanceServiceProvider);
  return service.listProviders();
});
