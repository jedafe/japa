import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';

void main() {
  group('JcStateSelector', () {
    testWidgets('loads states for a country and reports selection',
        (tester) async {
      String? selected;

      await tester.pumpWidget(
        ProviderScope(
          overrides: [
            statesProvider.overrideWith((ref, code) async {
              if (code != 'NG') return [];
              return [
                GeoSpace(
                  id: 'lagos-state',
                  name: 'Lagos',
                  slug: 'lagos',
                  level: 'state',
                  geoCode: 'LA',
                ),
                GeoSpace(
                  id: 'abuja-state',
                  name: 'Abuja',
                  slug: 'abuja',
                  level: 'state',
                  geoCode: 'FC',
                ),
              ];
            }),
          ],
          child: MaterialApp(
            home: Scaffold(
              body: JcStateSelector(
                countryCode: 'NG',
                onChanged: (id) => selected = id,
              ),
            ),
          ),
        ),
      );

      await tester.pumpAndSettle();

      expect(find.byType(DropdownMenu<String>), findsOneWidget);

      await tester.tap(find.byType(DropdownMenu<String>));
      await tester.pumpAndSettle();

      expect(find.text('Lagos'), findsOneWidget);
      expect(find.text('Abuja'), findsOneWidget);

      await tester.tap(find.text('Abuja'));
      await tester.pumpAndSettle();

      expect(selected, 'abuja-state');
    });
  });

  group('JcCitySelector', () {
    testWidgets('loads cities for a state and reports selection',
        (tester) async {
      String? selected;

      await tester.pumpWidget(
        ProviderScope(
          overrides: [
            citiesProvider.overrideWith((ref, id) async {
              if (id != 'lagos-state') return [];
              return [
                GeoSpace(
                  id: 'ikeja',
                  name: 'Ikeja',
                  slug: 'ikeja',
                  level: 'city',
                ),
                GeoSpace(
                  id: 'lekki',
                  name: 'Lekki',
                  slug: 'lekki',
                  level: 'city',
                ),
              ];
            }),
          ],
          child: MaterialApp(
            home: Scaffold(
              body: JcCitySelector(
                stateId: 'lagos-state',
                onChanged: (id) => selected = id,
              ),
            ),
          ),
        ),
      );

      await tester.pumpAndSettle();

      expect(find.byType(DropdownMenu<String>), findsOneWidget);

      await tester.tap(find.byType(DropdownMenu<String>));
      await tester.pumpAndSettle();

      expect(find.text('Ikeja'), findsOneWidget);
      expect(find.text('Lekki'), findsOneWidget);

      await tester.tap(find.text('Lekki'));
      await tester.pumpAndSettle();

      expect(selected, 'lekki');
    });
  });
}
