import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/auth/providers/auth_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

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
    final t = ref.watch(tProvider);

    ref.listen(isAuthenticatedProvider, (prev, next) {
      if (next == true && prev != true) {
        context.go('/');
      }
    });

    return Scaffold(
      appBar: AppBar(title: Text(t('auth.register'))),
      body: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            JcTextField(label: t('auth.username'), controller: _username, hint: t('auth.username_hint', '3–30 characters — letters, numbers, underscores.')),
            const SizedBox(height: 16),
            JcTextField(label: t('auth.email'), controller: _email, hint: t('auth.email_hint', 'Used for sign-in and account emails.')),
            const SizedBox(height: 16),
            JcTextField(label: t('auth.password'), controller: _password, obscureText: true, hint: t('auth.password_hint', 'At least 8 characters with upper & lower case letters and a number.')),
            if (state.error != null) Text(state.error!, style: const TextStyle(color: Colors.red)),
            const SizedBox(height: 16),
            JcButton(
              label: state.isLoading ? t('auth.loading') : t('auth.register'),
              onPressed: state.isLoading
                  ? null
                  : () => notifier.register(_username.text, _email.text, _password.text),
            ),
            TextButton(
              onPressed: () => context.push('/login'),
              child: Text(t('auth.back_to_login')),
            ),
          ],
        ),
      ),
    );
  }
}
