import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:japaconnect_core/geo/models.dart';
import 'package:japaconnect_core/widgets/jc_error_widget.dart';
import 'package:japaconnect_core/widgets/loading_shimmer.dart';

/// A searchable dropdown for any list of [GeoSpace] values.
///
/// The parent widget is responsible for watching the right provider and
/// passing the resulting [AsyncValue]. This keeps the widget reusable for
/// countries, states and cities.
class JcGeoSelector extends StatelessWidget {
  final AsyncValue<List<GeoSpace>> state;
  final String? selectedId;
  final ValueChanged<String?> onChanged;
  final String? label;
  final VoidCallback? onRetry;

  const JcGeoSelector({
    super.key,
    required this.state,
    this.selectedId,
    required this.onChanged,
    this.label,
    this.onRetry,
  });

  @override
  Widget build(BuildContext context) {
    return state.when(
      data: (items) {
        final entries = items
            .map(
              (c) => DropdownMenuEntry<String>(
                value: c.id,
                label: c.name,
              ),
            )
            .toList();

        return DropdownMenu<String>(
          initialSelection: selectedId,
          label: Text(label ?? 'Select'),
          dropdownMenuEntries: entries,
          onSelected: onChanged,
          enableSearch: true,
          requestFocusOnTap: true,
          expandedInsets: EdgeInsets.zero,
        );
      },
      loading: () => const SizedBox(
        height: 56,
        child: LoadingShimmer(),
      ),
      error: (e, _) => JcErrorWidget(
        error: e,
        onRetry: onRetry,
      ),
    );
  }
}
