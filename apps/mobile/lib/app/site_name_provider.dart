import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';

/// Loads the configured site name (general.site_name) once per session for
/// the MaterialApp title. Never hardcode the brand — Rule: siteSettings.
final siteNameProvider = FutureProvider<String>((ref) async {
  const fallback = 'Japa.to';
  try {
    final client = ref.watch(jcApiClientProvider);
    final res = await client.get<Map<String, dynamic>>(
      '/site/settings/public',
      parse: (json) => json as Map<String, dynamic>,
    );
    final settings = (res['data'] ?? res) as Map<String, dynamic>;
    final name = settings['general.site_name'];
    return name is String && name.isNotEmpty ? name : fallback;
  } catch (_) {
    return fallback;
  }
});
