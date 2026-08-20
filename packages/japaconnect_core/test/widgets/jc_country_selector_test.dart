import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';

void main() {
  testWidgets('JcCountrySelector loads and reports selection',
      (tester) async {
    String? selected;

    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          countriesProvider.overrideWith((ref) async => [
                GeoSpace(
                  id: '1',
                  name: 'Nigeria',
                  slug: 'nigeria',
                  level: 'country',
                  geoCode: 'NG',
                ),
                GeoSpace(
                  id: '2',
                  name: 'United Kingdom',
                  slug: 'united-kingdom',
                  level: 'country',
                  geoCode: 'GB',
                ),
              ]),
        ],
        child: MaterialApp(
          home: Scaffold(
            body: JcCountrySelector(
              selectedId: null,
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

    expect(find.text('Nigeria'), findsOneWidget);
    expect(find.text('United Kingdom'), findsOneWidget);

    await tester.tap(find.text('United Kingdom'));
    await tester.pumpAndSettle();

    expect(selected, '2');
  });
}
