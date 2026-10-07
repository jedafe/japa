import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/jce/screens/comparison_list_screen.dart';
import 'package:japaconnect_mobile/features/jce/screens/comparison_detail_screen.dart';
import 'package:japaconnect_mobile/features/jce/providers/jce_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

import 'fake_jc_api_client.dart';

void main() {
  late AppLocalizations testL10n;

  setUpAll(() {
    final raw = File('lib/i18n/en.json').readAsStringSync();
    testL10n = AppLocalizations(Map<String, String>.from(jsonDecode(raw)));
  });

  testWidgets('jce screens show loading shimmer initially', (tester) async {
    for (final screen in [
      const JceComparisonListScreen(),
      const JceComparisonDetailScreen(id: 'sample'),
    ]) {
      await tester.pumpWidget(
        ProviderScope(child: MaterialApp(home: screen)),
      );
      expect(find.byType(LoadingShimmer), findsOneWidget);
    }
  });

  testWidgets('comparison list renders items and page size options from API meta', (tester) async {
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          tProvider.overrideWith((_) => testL10n.t),
          jcApiClientProvider.overrideWith((_) => FakeJcApiClient()),
        ],
        child: const MaterialApp(home: JceComparisonListScreen()),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('Lagos vs London'), findsOneWidget);
    expect(find.text('Toronto vs Berlin'), findsOneWidget);
    expect(find.byIcon(Icons.list), findsOneWidget);
    expect(find.byIcon(Icons.sort), findsOneWidget);
  });

  testWidgets('comparison detail renders title and bodyHtml fallback', (tester) async {
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          tProvider.overrideWith((_) => testL10n.t),
          jcApiClientProvider.overrideWith((_) => FakeJcApiClient()),
        ],
        child: const MaterialApp(home: JceComparisonDetailScreen(id: 'c1')),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('Lagos vs London'), findsWidgets);
    expect(find.text('Body HTML'), findsOneWidget);
  });

  test('jceCapabilitiesProvider computes refresh capabilities from fake client', () async {
    final container = ProviderContainer(
      overrides: [
        jcApiClientProvider.overrideWith((_) => FakeJcApiClient()),
      ],
    );
    addTearDown(container.dispose);
    final caps = await container.read(jceCapabilitiesProvider.future);
    expect(caps.canViewFreshness, isTrue);
    expect(caps.canRefresh, isTrue);
    expect(caps.canViewIngestionJobs, isTrue);
  });
}
