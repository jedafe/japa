import 'package:go_router/go_router.dart';

class AuthGuard {
  static String? redirect(bool loggedIn, GoRouterState state) {
    final onLogin = state.matchedLocation == '/login';
    if (!loggedIn && !onLogin) return '/login';
    if (loggedIn && onLogin) return '/';
    return null;
  }
}
