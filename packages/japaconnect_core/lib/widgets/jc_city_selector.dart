import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:japaconnect_core/geo/providers.dart';
import 'package:japaconnect_core/widgets/jc_geo_selector.dart';

/// A searchable dropdown that consumes the cities for a given state id from
/// `GET /api/v1/geo/states/{id}/cities`.
///
/// Returns the selected city id.
class JcCitySelector extends ConsumerWidget {
  final String stateId;
  final String? selectedId;
  final ValueChanged<String?> onChanged;
  final String? label;

  const JcCitySelector({
    super.key,
    required this.stateId,
    this.selectedId,
    required this.onChanged,
    this.label,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(citiesProvider(stateId));

    return JcGeoSelector(
      state: state,
      selectedId: selectedId,
      onChanged: onChanged,
      label: label,
      onRetry: () => ref.invalidate(citiesProvider(stateId)),
    );
  }
}
