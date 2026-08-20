import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/auth/providers/auth_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class LoginScreen extends ConsumerStatefulWidget {
  const LoginScreen({super.key});

  @override
  ConsumerState<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends ConsumerState<LoginScreen> {
  late final TextEditingController _username;
  late final TextEditingController _password;

  @override
  void initState() {
    super.initState();
    _username = TextEditingController();
    _password = TextEditingController();
  }

  @override
  void dispose() {
    _username.dispose();
    _password.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(authFormProvider);
    final notifier = ref.read(authFormProvider.notifier);
    final t = ref.watch(tProvider);

    ref.listen(authFormProvider, (prev, next) {
      if (next.challengeToken != null && next.challengeToken != prev?.challengeToken) {
        context.push('/totp');
      }
    });
    ref.listen(isAuthenticatedProvider, (prev, next) {
      if (next == true && prev != true) {
        context.go('/');
      }
    });

    return Scaffold(
      body: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            JcTextField(label: t('auth.email'), controller: _username),
            const SizedBox(height: 16),
            JcTextField(label: t('auth.password'), controller: _password, obscureText: true),
            if (state.error != null) Text(state.error!, style: const TextStyle(color: Colors.red)),
            const SizedBox(height: 16),
            JcButton(
              label: state.isLoading ? t('auth.loading') : t('auth.login'),
              onPressed: state.isLoading ? null : () => notifier.login(_username.text, _password.text),
            ),
            TextButton(
              onPressed: () => context.push('/register'),
              child: Text(t('auth.register')),
            ),
          ],
        ),
      ),
    );
  }
}
