import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/tools/remittance/services/remittance_service.dart';

final remittanceServiceProvider = Provider<RemittanceService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return RemittanceService(client);
});

final remittanceProvidersProvider =
    FutureProvider.autoDispose<List<RemittanceProvider>>((ref) async {
  final service = ref.watch(remittanceServiceProvider);
  return service.listProviders();
});

final remittanceRatesProvider = FutureProvider.autoDispose
    .family<List<RemittanceRate>, ({String send, String receive})>(
  (ref, corridor) async {
    final service = ref.watch(remittanceServiceProvider);
    return service.getRates(corridor.send, corridor.receive);
  },
);
