import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/api/client.dart';
import 'package:japaconnect_core/geo/models.dart';
import 'package:japaconnect_core/geo/service.dart';

final geoServiceProvider = Provider<GeoService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return GeoService(client);
});

/// Loads the full 250 primary geo space countries once per session.
/// Consumers should watch this provider for country selectors.
final countriesProvider = FutureProvider.autoDispose<List<GeoSpace>>((ref) async {
  final service = ref.watch(geoServiceProvider);
  return service.listCountries();
});

/// Loads states/provinces for a given country geo_code (e.g. "NG").
/// Consumers should watch this provider for state selectors.
final statesProvider = FutureProvider.autoDispose
    .family<List<GeoSpace>, String>((ref, countryCode) async {
  if (countryCode.isEmpty) return [];
  final service = ref.watch(geoServiceProvider);
  return service.listStates(countryCode);
});

/// Loads cities for a given state space id.
/// Consumers should watch this provider for city selectors.
final citiesProvider = FutureProvider.autoDispose
    .family<List<GeoSpace>, String>((ref, stateId) async {
  if (stateId.isEmpty) return [];
  final service = ref.watch(geoServiceProvider);
  return service.listCities(stateId);
});
