import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/auth/providers/auth_provider.dart';

class LoginScreen extends ConsumerWidget {
  const LoginScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final username = TextEditingController();
    final password = TextEditingController();
    final state = ref.watch(authFormProvider);
    final notifier = ref.read(authFormProvider.notifier);

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
            JcTextField(label: 'Username', controller: username),
            const SizedBox(height: 16),
            JcTextField(label: 'Password', controller: password, obscureText: true),
            if (state.error != null) Text(state.error!, style: const TextStyle(color: Colors.red)),
            const SizedBox(height: 16),
            JcButton(
              label: state.isLoading ? 'Loading...' : 'Login',
              onPressed: state.isLoading ? null : () => notifier.login(username.text, password.text),
            ),
            TextButton(
              onPressed: () => context.push('/register'),
              child: const Text('Register'),
            ),
          ],
        ),
      ),
    );
  }
}
