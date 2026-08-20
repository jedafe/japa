import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:japaconnect_core/geo/providers.dart';
import 'package:japaconnect_core/widgets/jc_geo_selector.dart';

/// A searchable dropdown that consumes the canonical 250 primary geo space
/// countries from `GET /api/v1/geo/countries`.
///
/// Returns the selected country id. Use the `onChanged` callback to resolve
/// the full [GeoSpace] from the parent provider if you need `geo_code`/`name`.
class JcCountrySelector extends ConsumerWidget {
  final String? selectedId;
  final ValueChanged<String?> onChanged;
  final String? label;

  const JcCountrySelector({
    super.key,
    this.selectedId,
    required this.onChanged,
    this.label,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(countriesProvider);

    return JcGeoSelector(
      state: state,
      selectedId: selectedId,
      onChanged: onChanged,
      label: label,
      onRetry: () => ref.invalidate(countriesProvider),
    );
  }
}
