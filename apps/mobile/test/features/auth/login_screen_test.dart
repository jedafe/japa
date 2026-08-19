import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_mobile/features/auth/screens/login_screen.dart';
import 'package:japaconnect_mobile/features/auth/screens/register_screen.dart';
import 'package:japaconnect_mobile/features/auth/screens/totp_screen.dart';

void main() {
  testWidgets('LoginScreen shows login fields', (tester) async {
    await tester.pumpWidget(const ProviderScope(child: MaterialApp(home: LoginScreen())));
    expect(find.byType(TextField), findsNWidgets(2));
    expect(find.text('auth.login'), findsOneWidget);
  });

  testWidgets('RegisterScreen shows register fields', (tester) async {
    await tester.pumpWidget(const ProviderScope(child: MaterialApp(home: RegisterScreen())));
    expect(find.byType(TextField), findsNWidgets(3));
    expect(find.text('auth.register'), findsNWidgets(2));
  });

  testWidgets('TOTPScreen shows TOTP field', (tester) async {
    await tester.pumpWidget(const ProviderScope(child: MaterialApp(home: TOTPScreen())));
    expect(find.byType(TextField), findsOneWidget);
    expect(find.text('auth.verify'), findsOneWidget);
  });
}
