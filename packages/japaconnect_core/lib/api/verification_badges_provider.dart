import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/api/client.dart';
import 'package:japaconnect_core/models/verification_type.dart';

/// Fetches and caches the managed verification badge taxonomy from
/// GET /verification-requests/types (JEOS §07). Keyed as 'scope:value' so
/// widgets can resolve custom artwork + labels for a given badge type.
final verificationBadgesProvider =
    FutureProvider<Map<String, VerificationType>>((ref) async {
  final client = ref.watch(jcApiClientProvider);
  final res = await client.get<Map<String, dynamic>>(
    '/verification-requests/types',
    parse: (d) => d as Map<String, dynamic>,
  );
  final list = (res['types'] ?? res['data'] ?? []) as List<dynamic>;
  final map = <String, VerificationType>{};
  for (final raw in list) {
    final t = VerificationType.fromJson(raw as Map<String, dynamic>);
    map['${t.scope}:${t.value}'] = t;
  }
  return map;
});

/// Resolves a badge's custom artwork URL for 'scope:type' from the cached
/// taxonomy, or null when the type has no artwork (generic icon fallback).
String? verificationBadgeUrl(
  AsyncValue<Map<String, VerificationType>> badges,
  String scope,
  String? type,
) {
  if (type == null || type.isEmpty) return null;
  return badges.value?['$scope:$type']?.badgeUrl;
}

/// Resolves a badge's managed label for 'scope:type' — the accessibility
/// label for badge artwork. Null when the type is unknown.
String? verificationBadgeLabel(
  AsyncValue<Map<String, VerificationType>> badges,
  String scope,
  String? type,
) {
  if (type == null || type.isEmpty) return null;
  return badges.value?['$scope:$type']?.label;
}
