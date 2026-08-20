import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/tools/visa_checklist/screens/visa_checklist_screen.dart';
import 'package:japaconnect_mobile/features/tools/readiness/screens/readiness_screen.dart';
import 'package:japaconnect_mobile/features/tools/cost_of_living/screens/cost_of_living_screen.dart';
import 'package:japaconnect_mobile/features/tools/cost_of_living/providers/cost_of_living_provider.dart';
import 'package:japaconnect_mobile/features/tools/scholarships/screens/scholarships_screen.dart';
import 'package:japaconnect_mobile/features/tools/remittance/screens/remittance_screen.dart';

void main() {
  testWidgets('v2 tool screens show loading shimmer', (tester) async {
    for (final screen in [
      const VisaChecklistScreen(),
      const ReadinessScreen(),
      const ScholarshipsScreen(),
      const RemittanceScreen(),
    ]) {
      await tester.pumpWidget(
          ProviderScope(child: MaterialApp(home: screen)));
      expect(find.byType(LoadingShimmer), findsOneWidget);
    }
  });

  testWidgets('cost of living screen shows country selectors and compare',
      (tester) async {
    await tester.pumpWidget(ProviderScope(
      overrides: [
        countriesProvider.overrideWith((ref) async => [
              GeoSpace(
                id: '00000000-0000-0000-0000-000000000001',
                name: 'Nigeria',
                slug: 'nigeria',
                level: 'country',
                geoCode: 'NG',
              ),
              GeoSpace(
                id: '00000000-0000-0000-0000-000000000002',
                name: 'United Kingdom',
                slug: 'united-kingdom',
                level: 'country',
                geoCode: 'GB',
              ),
            ]),
        costOfLivingCitiesProvider.overrideWith((ref, _) async => []),
        costOfLivingComparisonProvider.overrideWith((ref, _) async => {}),
      ],
      child: const MaterialApp(home: CostOfLivingScreen()),
    ));

    await tester.pumpAndSettle();

    expect(find.byType(JcCountrySelector), findsNWidgets(2));
    expect(find.byType(JcButton), findsOneWidget);
  });
}
