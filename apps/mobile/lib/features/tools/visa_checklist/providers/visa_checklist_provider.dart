import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/tools/visa_checklist/services/visa_checklist_service.dart';

final visaChecklistServiceProvider = Provider<VisaChecklistService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return VisaChecklistService(client);
});

/// GET /checklist/selector — origins + destinations (+ nested visa types).
final visaChecklistSelectorProvider =
    FutureProvider.autoDispose<ChecklistSelector>((ref) async {
      final service = ref.watch(visaChecklistServiceProvider);
      return service.getSelector();
    });

/// 3-step selection state. Origin narrows the item set; destination +
/// visa type resolve the guide_id.
class VisaChecklistSelection {
  final ChecklistCountryOption? origin;
  final ChecklistCountryOption? destination;
  final ChecklistVisaTypeOption? visaType;

  const VisaChecklistSelection({this.origin, this.destination, this.visaType});

  VisaChecklistSelection copyWith({
    ChecklistCountryOption? Function()? origin,
    ChecklistCountryOption? Function()? destination,
    ChecklistVisaTypeOption? Function()? visaType,
  }) => VisaChecklistSelection(
    origin: origin != null ? origin() : this.origin,
    destination: destination != null ? destination() : this.destination,
    visaType: visaType != null ? visaType() : this.visaType,
  );
}

class VisaChecklistSelectionNotifier extends Notifier<VisaChecklistSelection> {
  @override
  VisaChecklistSelection build() => const VisaChecklistSelection();

  /// Origin changes only affect item filtering — the picked destination +
  /// visa type (i.e. the guide) are origin-independent and stay selected.
  void selectOrigin(ChecklistCountryOption? o) {
    state = state.copyWith(origin: () => o);
  }

  void selectDestination(ChecklistCountryOption? d) {
    state = state.copyWith(destination: () => d, visaType: () => null);
  }

  void selectVisaType(ChecklistVisaTypeOption? v) {
    state = state.copyWith(visaType: () => v);
  }

  void clear() {
    state = const VisaChecklistSelection();
  }
}

final visaChecklistSelectionProvider =
    NotifierProvider.autoDispose<
      VisaChecklistSelectionNotifier,
      VisaChecklistSelection
    >(VisaChecklistSelectionNotifier.new);

/// Loaded guide for the picked visa type, with authenticated progress
/// merged in. Re-fetches when origin or visa type changes.
final visaChecklistGuideProvider = FutureProvider.autoDispose<ChecklistGuide?>((
  ref,
) async {
  final sel = ref.watch(visaChecklistSelectionProvider);
  final vt = sel.visaType;
  if (vt == null) return null;

  final service = ref.watch(visaChecklistServiceProvider);
  final originSlug = sel.origin?.slug;

  final guide = await service.getGuide(vt.guideId, origin: originSlug);

  // Merge saved progress for signed-in users (§70: GET /progress). A
  // failed progress load still renders the checklist unticked rather
  // than failing the whole screen.
  final authed = ref.watch(isAuthenticatedProvider);
  if (!authed) return guide;

  List<ChecklistProgressItem> progress;
  try {
    progress = await service.getProgress(vt.guideId, origin: originSlug);
  } catch (_) {
    return guide;
  }
  if (progress.isEmpty) return guide;

  final byKey = {for (final p in progress) p.itemKey: p};
  return ChecklistGuide(
    guideId: guide.guideId,
    title: guide.title,
    items: [
      for (final it in guide.items)
        byKey.containsKey(it.itemKey)
            ? it.copyWith(
                completed: byKey[it.itemKey]!.completed,
                notes: byKey[it.itemKey]!.notes.isNotEmpty
                    ? byKey[it.itemKey]!.notes
                    : it.notes,
              )
            : it,
    ],
  );
});
