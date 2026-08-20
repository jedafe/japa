import 'package:japaconnect_core/api/client.dart';
import 'package:japaconnect_core/geo/models.dart';

/// Geo data service. Wraps the public geo hierarchy endpoints so mobile
/// features can consume the canonical 250 primary geo space countries and
/// drill down into states/provinces and cities.
///
/// Ref: japaconnect-engineering/sections/05-geo-first-space-system.md
class GeoService {
  final JcApiClient _client;

  GeoService(this._client);

  /// Returns every seeded country-level primary space (~250 countries).
  Future<List<GeoSpace>> listCountries() async {
    final result = await _client.get<List<GeoSpace>>(
      '/geo/countries',
      parse: (json) {
        final data = (json as Map<String, dynamic>)['data'] as List<dynamic>? ?? [];
        return data
            .cast<Map<String, dynamic>>()
            .map(GeoSpace.fromJson)
            .where((s) => s.isCountry)
            .toList();
      },
    );
    return result;
  }

  /// Returns the state/province spaces for the country with the given
  /// ISO geo code.
  Future<List<GeoSpace>> listStates(String countryCode) async {
    final result = await _client.get<List<GeoSpace>>(
      '/geo/countries/$countryCode/states',
      parse: (json) {
        final data = (json as Map<String, dynamic>)['data'] as List<dynamic>? ?? [];
        return data.cast<Map<String, dynamic>>().map(GeoSpace.fromJson).toList();
      },
    );
    return result;
  }

  /// Returns the city spaces for the state with the given space id.
  Future<List<GeoSpace>> listCities(String stateId) async {
    final result = await _client.get<List<GeoSpace>>(
      '/geo/states/$stateId/cities',
      parse: (json) {
        final data = (json as Map<String, dynamic>)['data'] as List<dynamic>? ?? [];
        return data.cast<Map<String, dynamic>>().map(GeoSpace.fromJson).toList();
      },
    );
    return result;
  }
}
