import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/jce/screens/comparison_list_screen.dart';
import 'package:japaconnect_mobile/features/jce/screens/comparison_detail_screen.dart';
import 'package:japaconnect_mobile/features/jce/screens/profile_list_screen.dart';
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
      const JceProfileListScreen(),
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

  testWidgets('comparison detail renders title, bodyHtml fallback, and score', (tester) async {
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
    expect(find.text('Score'), findsOneWidget);
    expect(find.text('Lagos: 85.0 / 100.0'), findsOneWidget);
    expect(find.byType(CircularProgressIndicator), findsNothing);
  });

  testWidgets('comparison detail handles empty profile list', (tester) async {
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          tProvider.overrideWith((_) => testL10n.t),
          jcApiClientProvider.overrideWith((_) => FakeJcApiClient(emptyProfiles: true)),
        ],
        child: const MaterialApp(home: JceComparisonDetailScreen(id: 'c1')),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('No profiles yet'), findsOneWidget);
  });

  test('jceCapabilitiesProvider computes view score from fake client', () async {
    final container = ProviderContainer(
      overrides: [
        jcApiClientProvider.overrideWith((_) => FakeJcApiClient()),
      ],
    );
    addTearDown(container.dispose);
    final caps = await container.read(jceCapabilitiesProvider.future);
    expect(caps.canViewScore, isTrue);
    expect(caps.canRecalculateScore, isTrue);
  });

  test('scoreComparisonProvider loads score from fake client', () async {
    final container = ProviderContainer(
      overrides: [
        jcApiClientProvider.overrideWith((_) => FakeJcApiClient()),
      ],
    );
    addTearDown(container.dispose);
    final score = await container.read(scoreComparisonProvider('c1').future);
    expect(score, isNotNull);
    expect(score!.scores.first.itemTitle, 'Lagos');
  });

  testWidgets('profile list renders profiles and default label', (tester) async {
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          tProvider.overrideWith((_) => testL10n.t),
          jcApiClientProvider.overrideWith((_) => FakeJcApiClient()),
        ],
        child: const MaterialApp(home: JceProfileListScreen()),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('Solo mover'), findsOneWidget);
    expect(find.text('Default'), findsOneWidget);
  });
}
