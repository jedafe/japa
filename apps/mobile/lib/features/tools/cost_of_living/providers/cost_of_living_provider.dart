import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/tools/cost_of_living/services/cost_of_living_service.dart';

final costOfLivingServiceProvider = Provider<CostOfLivingService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return CostOfLivingService(client);
});

final costOfLivingProvider = FutureProvider.autoDispose
    .family<Map<String, dynamic>, ({String origin, String destination})>(
        (ref, args) async {
  final service = ref.watch(costOfLivingServiceProvider);
  return service.compare(args.origin, args.destination);
});
