import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/tools/cost_of_living/services/cost_of_living_service.dart';

final costOfLivingServiceProvider = Provider<CostOfLivingService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return CostOfLivingService(client);
});

/// Loads cost-of-living cities for a given country geo code.
/// An empty country code resolves to an empty list without a network call.
final costOfLivingCitiesProvider =
    FutureProvider.autoDispose.family<List<Map<String, dynamic>>, String>(
  (ref, countryCode) async {
    if (countryCode.isEmpty) return [];
    final service = ref.watch(costOfLivingServiceProvider);
    return service.listCities(countryCode: countryCode);
  },
);

/// Compares the selected origin and destination city IDs.
/// Returns an empty map until both cities are selected.
final costOfLivingComparisonProvider = FutureProvider.autoDispose
    .family<Map<String, dynamic>, ({String originCityId, String destinationCityId})>(
  (ref, args) async {
    if (args.originCityId.isEmpty || args.destinationCityId.isEmpty) {
      return {};
    }
    final service = ref.watch(costOfLivingServiceProvider);
    final response =
        await service.compare([args.originCityId, args.destinationCityId]);
    return (response['data'] as Map<String, dynamic>? ?? {});
  },
);
