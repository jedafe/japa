import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';

import '../services/next_actions_service.dart';

final nextActionsServiceProvider = Provider<NextActionsService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return NextActionsService(client);
});

/// §122 — the ranked suggestion list for the mobile surface. Auto-disposes;
/// screens re-fetch on entry so cooldowns/caps stay server-side.
final nextActionsProvider =
    AsyncNotifierProvider.autoDispose<NextActionsNotifier, List<NextAction>>(
  NextActionsNotifier.new,
);

class NextActionsNotifier extends AsyncNotifier<List<NextAction>> {
  @override
  Future<List<NextAction>> build() {
    try {
      return ref.read(nextActionsServiceProvider).getNextActions();
    } catch (_) {
      return Future.value(const <NextAction>[]);
    }
  }

  Future<void> dismiss(NextAction action) async {
    try {
      await ref.read(nextActionsServiceProvider).dismiss(action.id);
    } catch (_) {}
    state = AsyncData((state.value ?? [])
        .where((a) => a.id != action.id)
        .toList());
  }

  /// "Not now" defers the suggestion — snooze for the rule's default
  /// duration (falling back to a day) rather than permanently dismissing.
  Future<void> snooze(NextAction action) async {
    try {
      await ref.read(nextActionsServiceProvider).dismiss(
        action.id,
        snoozeHours: action.snoozeDefaultHours > 0 ? action.snoozeDefaultHours : 24,
      );
    } catch (_) {}
    state = AsyncData((state.value ?? [])
        .where((a) => a.id != action.id)
        .toList());
  }

  Future<void> act(NextAction action) async {
    try {
      // CTA tap records 'clicked' only — completion belongs to the rule's
      // completion_signal or a real completion hook, not a link tap.
      await ref.read(nextActionsServiceProvider).track(action.id, 'clicked');
    } catch (_) {}
    state = AsyncData((state.value ?? [])
        .where((a) => a.id != action.id)
        .toList());
  }

}
