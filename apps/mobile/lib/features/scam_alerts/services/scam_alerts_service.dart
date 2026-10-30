// Ref: japaconnect-engineering/sections/81-scam-alert-board.md
import 'package:japaconnect_core/japaconnect_core.dart';

/// A single confirmed scam report as returned by GET /api/v1/scam-alerts.
/// Public DTO — reporter identity and moderation provenance are redacted
/// server-side.
class ScamAlertItem {
  final String id;
  final String scamType;
  final String destinationSpaceId;
  final String destinationSpaceName;
  final String? platformUsed;
  final String? approximateDate;
  final String description;
  final String status;
  final bool isPinned;
  final bool isSubscribed;
  final String createdAt;

  const ScamAlertItem({
    required this.id,
    required this.scamType,
    required this.destinationSpaceId,
    required this.destinationSpaceName,
    this.platformUsed,
    this.approximateDate,
    required this.description,
    required this.status,
    required this.isPinned,
    this.isSubscribed = false,
    required this.createdAt,
  });

  factory ScamAlertItem.fromJson(Map<String, dynamic> j) => ScamAlertItem(
        id: j['id'] as String? ?? '',
        scamType: j['scam_type'] as String? ?? 'other',
        destinationSpaceId: j['destination_space_id'] as String? ?? '',
        destinationSpaceName: j['destination_space_name'] as String? ?? '',
        platformUsed: j['platform_used'] as String?,
        approximateDate: j['approximate_date'] as String?,
        description: j['description'] as String? ?? '',
        status: j['status'] as String? ?? '',
        isPinned: j['is_pinned'] as bool? ?? false,
        isSubscribed: j['is_subscribed'] as bool? ?? false,
        createdAt: j['created_at'] as String? ?? '',
      );
}

class ScamAlertsService {
  final JcApiClient _client;
  ScamAlertsService(this._client);

  /// Confirmed reports only — the public board. ?dest + ?scam_type filters
  /// mirror the web surface.
  Future<List<ScamAlertItem>> list({String? dest, String? scamType, int limit = 30, int offset = 0}) async {
    var path = '/scam-alerts?limit=$limit&offset=$offset';
    if (dest != null && dest.isNotEmpty) path += '&dest=$dest';
    if (scamType != null && scamType.isNotEmpty) path += '&scam_type=$scamType';
    return _client.get<List<ScamAlertItem>>(
      path,
      parse: (json) => (json as List)
          .map((e) => ScamAlertItem.fromJson(e as Map<String, dynamic>))
          .toList(),
    );
  }

  Future<ScamAlertItem> get(String id) => _client.get<ScamAlertItem>(
        '/scam-alerts/$id',
        parse: (json) => ScamAlertItem.fromJson(json as Map<String, dynamic>),
      );

  /// POST /scam-alerts — authenticated OR anonymous (the backend gates the
  /// anonymous path on scam_alert.anonymous_reports). Returns the new id.
  Future<String> submit(Map<String, dynamic> body) => _client.post<String>(
        '/scam-alerts',
        body: body,
        parse: (json) => (json as Map<String, dynamic>)['id'] as String? ?? '',
      );

  Future<void> subscribe(String id) => _client.post<void>(
        '/scam-alerts/$id/subscribe',
        parse: (_) {},
      );

  Future<void> unsubscribe(String id) => _client.delete('/scam-alerts/$id/subscribe');

  /// GET /scam-alerts/schema — canonical scam_type/platform option lists
  /// (never hardcode the enums; Rule 16).
  Future<List<Map<String, String>>> schemaOptions(String kind) async {
    final result = await _client.get<Map<String, dynamic>>(
      '/scam-alerts/schema',
      parse: (json) => json as Map<String, dynamic>,
    );
    final list = (result[kind] as List<dynamic>? ?? [])
        .cast<Map<String, dynamic>>();
    return list
        .map((e) => {'value': e['value'] as String? ?? '', 'label': e['label'] as String? ?? ''})
        .toList();
  }
}
