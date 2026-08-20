import 'package:japaconnect_core/japaconnect_core.dart';

class CostOfLivingService {
  final JcApiClient _client;
  CostOfLivingService(this._client);

  /// Lists cost-of-living cities, optionally filtered by [countryCode].
  Future<List<Map<String, dynamic>>> listCities({String? countryCode}) async {
    final query = <String, dynamic>{
      'page': 1,
      'limit': 200,
    };
    if (countryCode != null && countryCode.isNotEmpty) {
      query['country_code'] = countryCode;
    }

    final response = await _client.get<Map<String, dynamic>>(
      '/calculator/cost-of-living/cities',
      query: query,
      parse: (json) => json as Map<String, dynamic>,
    );

    final data = response['data'] as Map<String, dynamic>? ?? {};
    final cities = data['cities'] as List<dynamic>? ?? [];
    return cities.cast<Map<String, dynamic>>().toList();
  }

  /// Compares the given city IDs against the baseline city.
  Future<Map<String, dynamic>> compare(List<String> cityIds) async {
    final response = await _client.get<Map<String, dynamic>>(
      '/calculator/cost-of-living/compare',
      query: {'city_ids': cityIds.join(',')},
      parse: (json) => json as Map<String, dynamic>,
    );
    return response;
  }
}
