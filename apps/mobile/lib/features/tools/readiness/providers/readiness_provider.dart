import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/tools/readiness/services/readiness_service.dart';

final readinessServiceProvider = Provider<ReadinessService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return ReadinessService(client);
});

/// Active assessment form + scoring metadata.
final readinessProvider =
    FutureProvider.autoDispose<Map<String, dynamic>>((ref) async {
  final service = ref.watch(readinessServiceProvider);
  return service.getAssessment();
});

/// Authenticated user's assessment history. Fails gracefully for guests.
final readinessHistoryProvider =
    FutureProvider.autoDispose<List<dynamic>>((ref) async {
  final service = ref.watch(readinessServiceProvider);
  final auth = ref.watch(isAuthenticatedProvider);
  if (!auth) return [];
  return service.getHistory();
});

/// Journey option lists (purposes, stages, visa_routes) from the backend.
final readinessJourneyOptionsProvider =
    FutureProvider.autoDispose<Map<String, dynamic>>((ref) async {
  final service = ref.watch(readinessServiceProvider);
  return service.getJourneyOptions();
});

/// Country option list from the canonical Spaces directory, reordered so
/// the configured origin/destination markets (ADR-064, public settings)
/// come first. Every country remains selectable — pin-first, not restricted.
final readinessCountryOptionsProvider =
    FutureProvider.autoDispose<List<dynamic>>((ref) async {
  final service = ref.watch(readinessServiceProvider);
  final countries = await service.getCountries();

  Map<String, dynamic> settings;
  try {
    settings = await service.getPublicSettings();
  } catch (_) {
    settings = const {};
  }

  final markets = <String>{
    ..._parseCountryCsv(settings['geo_personalization.origin_markets']),
    ..._parseCountryCsv(settings['geo_personalization.destination_markets']),
  };
  if (markets.isEmpty) return countries;

  String codeOf(dynamic c) =>
      (c['geo_code'] ?? '').toString().toUpperCase();

  final pinned = <dynamic>[
    for (final c in countries)
      if (markets.contains(codeOf(c))) c,
  ];
  pinned.sort((a, b) => codeOf(a).compareTo(codeOf(b)));

  final rest = <dynamic>[
    for (final c in countries)
      if (!markets.contains(codeOf(c))) c,
  ];

  return [...pinned, ...rest];
});

List<String> _parseCountryCsv(dynamic raw) {
  if (raw is! String) return const [];
  return raw
      .split(',')
      .map((s) => s.trim().toUpperCase())
      .where((s) => s.length == 2)
      .toList();
}
