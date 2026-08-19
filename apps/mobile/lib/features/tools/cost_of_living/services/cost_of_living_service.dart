import 'package:japaconnect_core/japaconnect_core.dart';

class CostOfLivingService {
  final JcApiClient _client;
  CostOfLivingService(this._client);

  Future<Map<String, dynamic>> compare(
      String origin, String destination) async {
    final response = await _client.get<Map<String, dynamic>>(
      '/calculator/cost-of-living/compare',
      query: {'origin': origin, 'destination': destination},
      parse: (json) => json as Map<String, dynamic>,
    );
    return response;
  }
}
