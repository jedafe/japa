import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:japaconnect_core/geo/models.dart';
import 'package:japaconnect_core/geo/providers.dart';
import 'package:japaconnect_core/widgets/jc_error_widget.dart';
import 'package:japaconnect_core/widgets/loading_shimmer.dart';

/// A searchable dropdown that consumes the canonical 250 primary geo space
/// countries from `GET /api/v1/geo/countries`.
///
/// Returns the selected country's id. Use the `onChanged` callback to resolve
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

    return state.when(
      data: (countries) {
        final entries = countries
            .map(
              (c) => DropdownMenuEntry<String>(
                value: c.id,
                label: c.name,
              ),
            )
            .toList();

        return DropdownMenu<String>(
          initialSelection: selectedId,
          label: Text(label ?? 'Country'),
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
        onRetry: () => ref.invalidate(countriesProvider),
      ),
    );
  }
}
