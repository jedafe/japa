import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';

final authFormProvider =
    NotifierProvider<AuthFormNotifier, AuthFormState>(() => AuthFormNotifier());

class AuthFormState {
  final bool isLoading;
  final String? error;
  final String? challengeToken;

  const AuthFormState({this.isLoading = false, this.error, this.challengeToken});

  AuthFormState copyWith({bool? isLoading, String? error, String? challengeToken}) {
    return AuthFormState(
      isLoading: isLoading ?? this.isLoading,
      error: error ?? this.error,
      challengeToken: challengeToken ?? this.challengeToken,
    );
  }
}

class AuthFormNotifier extends Notifier<AuthFormState> {
  @override
  AuthFormState build() => const AuthFormState();

  AuthService get _auth => ref.read(authServiceProvider);

  void _setAuthenticated() {
    ref.read(isAuthenticatedProvider.notifier).set(true);
  }

  Future<void> login(String username, String password) async {
    state = state.copyWith(isLoading: true, error: null);
    try {
      await _auth.login(username, password, onAuthenticated: _setAuthenticated);
      state = state.copyWith(isLoading: false);
    } on JcApiError catch (e) {
      if (e.code == 'totp_required') {
        state = state.copyWith(isLoading: false, challengeToken: e.message);
      } else {
        ref.read(isAuthenticatedProvider.notifier).set(false);
        state = state.copyWith(isLoading: false, error: e.message);
      }
    }
  }

  Future<void> register(String username, String email, String password) async {
    state = state.copyWith(isLoading: true, error: null);
    try {
      await _auth.register(username, email, password, onAuthenticated: _setAuthenticated);
      state = state.copyWith(isLoading: false);
    } on JcApiError catch (e) {
      ref.read(isAuthenticatedProvider.notifier).set(false);
      state = state.copyWith(isLoading: false, error: e.message);
    }
  }

  Future<void> verifyTOTP(String code) async {
    state = state.copyWith(isLoading: true, error: null);
    try {
      final token = state.challengeToken;
      if (token == null) {
        state = state.copyWith(isLoading: false, error: 'No challenge token');
        return;
      }
      await _auth.verifyTOTP(token, code, onAuthenticated: _setAuthenticated);
      state = state.copyWith(isLoading: false);
    } on JcApiError catch (e) {
      ref.read(isAuthenticatedProvider.notifier).set(false);
      state = state.copyWith(isLoading: false, error: e.message);
    }
  }
}
