import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/feed/screens/discover_screen.dart';

void main() {
  testWidgets('DiscoverScreen shows loading shimmer initially', (tester) async {
    await tester.pumpWidget(const ProviderScope(child: MaterialApp(home: DiscoverScreen())));
    expect(find.byType(LoadingShimmer), findsOneWidget);
  });
}
