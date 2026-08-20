import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/tools/cost_of_living/providers/cost_of_living_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class CostOfLivingScreen extends ConsumerStatefulWidget {
  const CostOfLivingScreen({super.key});

  @override
  ConsumerState<CostOfLivingScreen> createState() => _CostOfLivingScreenState();
}

class _CostOfLivingScreenState extends ConsumerState<CostOfLivingScreen> {
  String _originCountryId = '';
  String _destinationCountryId = '';
  String _originCityId = '';
  String _destinationCityId = '';

  String _countryCodeFor(String countryId, List<GeoSpace> countries) {
    for (final c in countries) {
      if (c.id == countryId) return c.geoCode ?? '';
    }
    return '';
  }

  Widget _buildCityList(
    AsyncValue<List<Map<String, dynamic>>> state,
    String selectedCityId,
    Function(String) onSelect,
    String emptyLabel,
    String countryCode,
  ) {
    return state.when(
      data: (cities) {
        if (cities.isEmpty) {
          return Center(child: Text(emptyLabel));
        }
        return ListView.builder(
          itemCount: cities.length,
          itemBuilder: (_, i) {
            final city = cities[i];
            final id = city['id'] as String? ?? '';
            final title = city['title'] as String? ?? '';
            final geoName = city['geo_space_name'] as String? ?? '';
            final isBaseline = city['is_baseline'] == true;
            final selected = id == selectedCityId;

            return ListTile(
              title: Text(title),
              subtitle: geoName.isNotEmpty ? Text(geoName) : null,
              selected: selected,
              selectedTileColor: Theme.of(context).colorScheme.primaryContainer,
              trailing: isBaseline
                  ? Chip(
                      label: Text(
                        ref.read(tProvider)('tools.baseline'),
                        style: const TextStyle(fontSize: 12),
                      ),
                    )
                  : null,
              onTap: () => onSelect(id),
            );
          },
        );
      },
      loading: () => const LoadingShimmer(),
      error: (e, _) => JcErrorWidget(
        error: e,
        onRetry: () =>
            ref.invalidate(costOfLivingCitiesProvider(countryCode)),
      ),
    );
  }

  Widget _buildCountrySection(
    String labelKey,
    String selectedCountryId,
    ValueChanged<String?> onCountryChanged,
    AsyncValue<List<Map<String, dynamic>>> citiesState,
    String selectedCityId,
    ValueChanged<String> onCitySelected,
  ) {
    final t = ref.watch(tProvider);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(
          t(labelKey),
          style: const TextStyle(fontSize: 14, fontWeight: FontWeight.w600),
        ),
        const SizedBox(height: 8),
        JcCountrySelector(
          selectedId: selectedCountryId,
          onChanged: onCountryChanged,
          label: t('tools.select_a_country'),
        ),
        const SizedBox(height: 8),
        if (selectedCountryId.isNotEmpty)
          SizedBox(
            height: 160,
            child: _buildCityList(
              citiesState,
              selectedCityId,
              onCitySelected,
              t('tools.no_cities_for_country'),
              _countryCodeFor(
                selectedCountryId,
                ref.watch(countriesProvider).asData?.value ?? [],
              ),
            ),
          ),
      ],
    );
  }

  Widget _buildComparison(Map<String, dynamic> matrix) {
    final t = ref.watch(tProvider);
    final cities = (matrix['cities'] as List<dynamic>? ?? [])
        .cast<Map<String, dynamic>>();
    final categories = (matrix['categories'] as List<dynamic>? ?? [])
        .cast<Map<String, dynamic>>();
    final overall = matrix['overall_index'] as Map<String, dynamic>? ?? {};

    if (cities.isEmpty) return const SizedBox.shrink();

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          t('tools.overall_index'),
          style: const TextStyle(fontSize: 16, fontWeight: FontWeight.bold),
        ),
        const SizedBox(height: 8),
        ...cities.map((city) {
          final id = city['id'] as String? ?? '';
          final title = city['title'] as String? ?? '';
          final index = overall[id] as num?;
          return ListTile(
            title: Text(title),
            trailing: Text(index != null
                ? '${index.toStringAsFixed(2)}x'
                : t('common.none')),
          );
        }),
        const SizedBox(height: 16),
        Text(
          t('tools.compare'),
          style: const TextStyle(fontSize: 16, fontWeight: FontWeight.bold),
        ),
        const SizedBox(height: 8),
        SingleChildScrollView(
          scrollDirection: Axis.horizontal,
          child: DataTable(
            columns: [
              DataColumn(label: Text(t('tools.category'))),
              ...cities.map((city) => DataColumn(
                    label: Text(city['title'] as String? ?? ''),
                  )),
            ],
            rows: categories.map((cat) {
              final label = cat['label'] as String? ?? '';
              final values = (cat['values'] as Map<String, dynamic>? ?? {})
                  .cast<String, num>();
              return DataRow(
                cells: [
                  DataCell(Text(label)),
                  ...cities.map((city) {
                    final id = city['id'] as String? ?? '';
                    final value = values[id];
                    return DataCell(Text(value != null
                        ? value.toStringAsFixed(0)
                        : t('common.none')));
                  }),
                ],
              );
            }).toList(),
          ),
        ),
      ],
    );
  }

  @override
  Widget build(BuildContext context) {
    final t = ref.watch(tProvider);
    final countries =
        ref.watch(countriesProvider).asData?.value ?? [];

    final originCountryCode =
        _countryCodeFor(_originCountryId, countries);
    final destinationCountryCode =
        _countryCodeFor(_destinationCountryId, countries);

    final originCities =
        ref.watch(costOfLivingCitiesProvider(originCountryCode));
    final destinationCities =
        ref.watch(costOfLivingCitiesProvider(destinationCountryCode));

    final comparisonArgs = (
      originCityId: _originCityId,
      destinationCityId: _destinationCityId,
    );
    final comparison =
        ref.watch(costOfLivingComparisonProvider(comparisonArgs));

    final canCompare =
        _originCityId.isNotEmpty && _destinationCityId.isNotEmpty;

    return Scaffold(
      appBar: AppBar(title: Text(t('tools.cost_of_living'))),
      body: SingleChildScrollView(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            _buildCountrySection(
              'tools.origin',
              _originCountryId,
              (id) => setState(() {
                _originCountryId = id ?? '';
                _originCityId = '';
              }),
              originCities,
              _originCityId,
              (id) => setState(() => _originCityId = id),
            ),
            const SizedBox(height: 24),
            _buildCountrySection(
              'tools.destination',
              _destinationCountryId,
              (id) => setState(() {
                _destinationCountryId = id ?? '';
                _destinationCityId = '';
              }),
              destinationCities,
              _destinationCityId,
              (id) => setState(() => _destinationCityId = id),
            ),
            const SizedBox(height: 24),
            JcButton(
              label: t('tools.compare'),
              onPressed: canCompare
                  ? () => ref.invalidate(
                        costOfLivingComparisonProvider(comparisonArgs),
                      )
                  : null,
            ),
            const SizedBox(height: 16),
            comparison.when(
              data: _buildComparison,
              loading: () =>
                  canCompare ? const LoadingShimmer() : const SizedBox.shrink(),
              error: (e, _) => JcErrorWidget(
                error: e,
                onRetry: () => ref.invalidate(
                  costOfLivingComparisonProvider(comparisonArgs),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
