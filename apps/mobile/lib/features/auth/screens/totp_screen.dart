import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/auth/providers/auth_provider.dart';

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

    ref.listen(isAuthenticatedProvider, (prev, next) {
      if (next == true && prev != true) {
        context.go('/');
      }
    });

    return Scaffold(
      appBar: AppBar(title: const Text('Two-Factor Authentication')),
      body: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            JcTextField(label: 'TOTP Code', controller: _code),
            if (state.error != null) Text(state.error!, style: const TextStyle(color: Colors.red)),
            const SizedBox(height: 16),
            JcButton(
              label: state.isLoading ? 'Verifying...' : 'Verify',
              onPressed: state.isLoading ? null : () => notifier.verifyTOTP(_code.text),
            ),
          ],
        ),
      ),
    );
  }
}
