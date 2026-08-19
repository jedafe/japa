import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/auth/providers/auth_provider.dart';

class RegisterScreen extends ConsumerWidget {
  const RegisterScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final username = TextEditingController();
    final email = TextEditingController();
    final password = TextEditingController();
    final state = ref.watch(authFormProvider);
    final notifier = ref.read(authFormProvider.notifier);

    ref.listen(isAuthenticatedProvider, (prev, next) {
      if (next == true && prev != true) {
        context.go('/');
      }
    });

    return Scaffold(
      appBar: AppBar(title: const Text('Register')),
      body: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            JcTextField(label: 'Username', controller: username),
            const SizedBox(height: 16),
            JcTextField(label: 'Email', controller: email),
            const SizedBox(height: 16),
            JcTextField(label: 'Password', controller: password, obscureText: true),
            if (state.error != null) Text(state.error!, style: const TextStyle(color: Colors.red)),
            const SizedBox(height: 16),
            JcButton(
              label: state.isLoading ? 'Loading...' : 'Register',
              onPressed: state.isLoading
                  ? null
                  : () => notifier.register(username.text, email.text, password.text),
            ),
            TextButton(
              onPressed: () => context.push('/login'),
              child: const Text('Back to Login'),
            ),
          ],
        ),
      ),
    );
  }
}
