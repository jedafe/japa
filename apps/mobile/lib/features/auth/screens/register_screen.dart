import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/auth/providers/auth_provider.dart';

class RegisterScreen extends ConsumerStatefulWidget {
  const RegisterScreen({super.key});

  @override
  ConsumerState<RegisterScreen> createState() => _RegisterScreenState();
}

class _RegisterScreenState extends ConsumerState<RegisterScreen> {
  late final TextEditingController _username;
  late final TextEditingController _email;
  late final TextEditingController _password;

  @override
  void initState() {
    super.initState();
    _username = TextEditingController();
    _email = TextEditingController();
    _password = TextEditingController();
  }

  @override
  void dispose() {
    _username.dispose();
    _email.dispose();
    _password.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
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
            JcTextField(label: 'Username', controller: _username),
            const SizedBox(height: 16),
            JcTextField(label: 'Email', controller: _email),
            const SizedBox(height: 16),
            JcTextField(label: 'Password', controller: _password, obscureText: true),
            if (state.error != null) Text(state.error!, style: const TextStyle(color: Colors.red)),
            const SizedBox(height: 16),
            JcButton(
              label: state.isLoading ? 'Loading...' : 'Register',
              onPressed: state.isLoading
                  ? null
                  : () => notifier.register(_username.text, _email.text, _password.text),
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
