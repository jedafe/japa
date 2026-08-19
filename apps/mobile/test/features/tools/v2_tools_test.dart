import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/tools/visa_checklist/screens/visa_checklist_screen.dart';
import 'package:japaconnect_mobile/features/tools/readiness/screens/readiness_screen.dart';
import 'package:japaconnect_mobile/features/tools/cost_of_living/screens/cost_of_living_screen.dart';
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

  testWidgets('cost of living screen shows form initially', (tester) async {
    await tester.pumpWidget(
        const ProviderScope(child: MaterialApp(home: CostOfLivingScreen())));
    expect(find.byType(JcTextField), findsNWidgets(2));
    expect(find.byType(JcButton), findsOneWidget);
  });
}
