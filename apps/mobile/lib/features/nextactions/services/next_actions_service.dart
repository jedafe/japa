import 'package:japaconnect_core/japaconnect_core.dart';

/// §122 Next-Action Suggestion — render-ready item from
/// GET /api/v1/next-actions.
class NextAction {
  final String id;
  final String ruleSlug;
  final String title;
  final String body;
  final String ctaLabel;
  final String ctaUrl;
  final String icon;
  final List<String> channels;
  final int priority;
  final int snoozeDefaultHours;

  const NextAction({
    required this.id,
    required this.ruleSlug,
    required this.title,
    required this.body,
    required this.ctaLabel,
    required this.ctaUrl,
    required this.icon,
    required this.channels,
    required this.priority,
    required this.snoozeDefaultHours,
  });

  factory NextAction.fromJson(Map<String, dynamic> json) => NextAction(
        id: json['id'] as String? ?? '',
        ruleSlug: json['rule_slug'] as String? ?? '',
        title: json['title'] as String? ?? '',
        body: json['body'] as String? ?? '',
        ctaLabel: json['cta_label'] as String? ?? '',
        ctaUrl: json['cta_url'] as String? ?? '',
        icon: json['icon'] as String? ?? '',
        channels: (json['channels'] as List<dynamic>? ?? [])
            .whereType<String>()
            .toList(),
        priority: json['priority'] as int? ?? 0,
        snoozeDefaultHours: json['snooze_default_hours'] as int? ?? 0,
      );
}

class NextActionsService {
  final JcApiClient _client;

  NextActionsService(this._client);

  /// Pull the ranked suggestion list for a surface ('mobile' here — the
  /// audience.surfaces filter distinguishes mobile from the web shell).
  Future<List<NextAction>> getNextActions({String surface = 'mobile'}) async {
    return _client.get<List<NextAction>>(
      '/next-actions?surface=$surface',
      parse: (json) => (json as List<dynamic>)
          .map((e) => NextAction.fromJson(e as Map<String, dynamic>))
          .toList(),
    );
  }

  Future<void> dismiss(String id, {int? snoozeHours}) {
    return _client.post<void>(
      '/next-actions/$id/dismiss',
      body: snoozeHours != null ? {'snooze_hours': snoozeHours} : const {},
      parse: (_) {},
    );
  }

  Future<void> complete(String id) {
    return _client.post<void>('/next-actions/$id/complete',
        body: const {}, parse: (_) {});
  }

  Future<void> track(String id, String event) {
    return _client.post<void>(
      '/next-actions/$id/track',
      body: {'event': event},
      parse: (_) {},
    );
  }
}
