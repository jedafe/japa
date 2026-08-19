import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/spaces/screens/space_list_screen.dart';
import 'package:japaconnect_mobile/features/spaces/screens/space_detail_screen.dart';

void main() {
  testWidgets('space list screen shows loading shimmer', (tester) async {
    await tester.pumpWidget(
        const ProviderScope(child: MaterialApp(home: SpaceListScreen())));
    expect(find.byType(LoadingShimmer), findsOneWidget);
  });

  testWidgets('space detail screen shows loading shimmer', (tester) async {
    await tester.pumpWidget(const ProviderScope(
        child: MaterialApp(home: SpaceDetailScreen(slug: 'sample'))));
    expect(find.byType(LoadingShimmer), findsOneWidget);
  });
}
