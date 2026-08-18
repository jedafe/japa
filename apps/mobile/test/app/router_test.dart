import 'package:flutter_test/flutter_test.dart';
import 'package:go_router/go_router.dart';
import 'package:mocktail/mocktail.dart';
import 'package:japaconnect_mobile/app/auth_guard.dart';

class MockGoRouterState extends Mock implements GoRouterState {}

void main() {
  test('redirects unauthenticated users to /login', () {
    final state = MockGoRouterState();
    when(() => state.matchedLocation).thenReturn('/');
    expect(AuthGuard.redirect(false, state), '/login');
  });
}
