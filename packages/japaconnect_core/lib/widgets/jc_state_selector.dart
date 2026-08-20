import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:japaconnect_core/geo/providers.dart';
import 'package:japaconnect_core/widgets/jc_geo_selector.dart';

/// A searchable dropdown that consumes the states/provinces for a given
/// country geo_code from `GET /api/v1/geo/countries/{code}/states`.
///
/// Returns the selected state id.
class JcStateSelector extends ConsumerWidget {
  final String countryCode;
  final String? selectedId;
  final ValueChanged<String?> onChanged;
  final String? label;

  const JcStateSelector({
    super.key,
    required this.countryCode,
    this.selectedId,
    required this.onChanged,
    this.label,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(statesProvider(countryCode));

    return JcGeoSelector(
      state: state,
      selectedId: selectedId,
      onChanged: onChanged,
      label: label,
      onRetry: () => ref.invalidate(statesProvider(countryCode)),
    );
  }
}
