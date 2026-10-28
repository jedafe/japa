import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/auth/providers/auth_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class TOTPScreen extends ConsumerStatefulWidget {
  const TOTPScreen({super.key});

  @override
  ConsumerState<TOTPScreen> createState() => _TOTPScreenState();
}

class _TOTPScreenState extends ConsumerState<TOTPScreen> {
  late final TextEditingController _code;

  @override
  void initState() {
    super.initState();
    _code = TextEditingController();
  }

  @override
  void dispose() {
    _code.dispose();
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
      appBar: AppBar(title: Text(t('auth.totp_title'))),
      body: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            JcTextField(label: t('auth.totp_code'), controller: _code, hint: t('auth.totp_code_hint', 'Open your authenticator app and enter the 6-digit code.')),
            if (state.error != null) Text(state.error!, style: const TextStyle(color: Colors.red)),
            const SizedBox(height: 16),
            JcButton(
              label: state.isLoading ? t('auth.verifying') : t('auth.verify'),
              onPressed: state.isLoading ? null : () => notifier.verifyTOTP(_code.text),
            ),
          ],
        ),
      ),
    );
  }
}
