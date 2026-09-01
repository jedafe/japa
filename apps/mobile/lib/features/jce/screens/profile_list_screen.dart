import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/jce/providers/jce_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class JceProfileListScreen extends ConsumerWidget {
  final bool selecting;

  const JceProfileListScreen({super.key, this.selecting = false});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final t = ref.watch(tProvider);
    final state = ref.watch(jceProfilesProvider);
    final selectedId = ref.watch(selectedProfileIdProvider);

    return Scaffold(
      appBar: AppBar(
        title: Text(t('jce.profile.title')),
        actions: [
          IconButton(
            icon: const Icon(Icons.add),
            onPressed: () => _showProfileSheet(context, ref, null),
          ),
        ],
      ),
      body: state.when(
        data: (profiles) {
          if (profiles.isEmpty) {
            return EmptyState(message: t('jce.profile.empty'));
          }
          return ListView.builder(
            itemCount: profiles.length,
            itemBuilder: (_, i) {
              final p = profiles[i];
              return ListTile(
                leading: selectedId == p.id
                    ? const Icon(Icons.check_circle)
                    : const Icon(Icons.person_outline),
                title: Text(p.title),
                subtitle: p.isDefault ? Text(t('jce.profile.default_label')) : null,
                trailing: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    if (p.isDefault) const Icon(Icons.star),
                    _ProfileActions(
                      profile: p,
                      onEdit: () => _showProfileSheet(context, ref, p),
                    ),
                  ],
                ),
                onTap: () {
                  ref.read(selectedProfileIdProvider.notifier).select(p.id);
                  if (selecting && context.canPop()) {
                    context.pop();
                  }
                },
              );
            },
          );
        },
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
          error: e,
          onRetry: () => ref.invalidate(jceProfilesProvider),
        ),
      ),
    );
  }

  void _showProfileSheet(BuildContext context, WidgetRef ref, ComparisonProfile? profile) {
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      builder: (_) => _ProfileSheet(profile: profile),
    );
  }
}

class _ProfileActions extends ConsumerWidget {
  final ComparisonProfile profile;
  final VoidCallback onEdit;

  const _ProfileActions({
    required this.profile,
    required this.onEdit,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final t = ref.watch(tProvider);
    final service = ref.watch(jceServiceProvider);

    return PopupMenuButton<String>(
      icon: const Icon(Icons.more_vert),
      onSelected: (value) async {
        switch (value) {
          case 'edit':
            onEdit();
            break;
          case 'default':
            try {
              await service.setDefaultProfile(profile.id);
              ref.invalidate(jceProfilesProvider);
            } catch (e) {
              if (context.mounted) {
                ScaffoldMessenger.of(context).showSnackBar(
                  SnackBar(content: Text(t('jce.profile.error_default_failed', {'error': e.toString()}))),
                );
              }
            }
            break;
          case 'delete':
            final confirmed = await showDialog<bool>(
              context: context,
              builder: (_) => AlertDialog(
                title: Text(t('jce.profile.delete_title')),
                content: Text(t('jce.profile.delete_confirm')),
                actions: [
                  TextButton(
                    onPressed: () => Navigator.of(context).pop(false),
                    child: Text(t('jce.common.no')),
                  ),
                  TextButton(
                    onPressed: () => Navigator.of(context).pop(true),
                    child: Text(t('jce.common.yes')),
                  ),
                ],
              ),
            );
            if (confirmed != true || !context.mounted) return;
            try {
              await service.deleteProfile(profile.id);
              ref.invalidate(jceProfilesProvider);
            } catch (e) {
              if (context.mounted) {
                ScaffoldMessenger.of(context).showSnackBar(
                  SnackBar(content: Text(t('jce.profile.error_delete_failed', {'error': e.toString()}))),
                );
              }
            }
            break;
        }
      },
      itemBuilder: (_) => [
        PopupMenuItem(value: 'edit', child: Text(t('jce.profile.action_edit'))),
        if (!profile.isDefault)
          PopupMenuItem(value: 'default', child: Text(t('jce.profile.action_set_default'))),
        PopupMenuItem(value: 'delete', child: Text(t('jce.profile.action_delete'))),
      ],
    );
  }
}

class _ProfileSheet extends ConsumerStatefulWidget {
  final ComparisonProfile? profile;

  const _ProfileSheet({this.profile});

  @override
  ConsumerState<_ProfileSheet> createState() => _ProfileSheetState();
}

class _ProfileSheetState extends ConsumerState<_ProfileSheet> {
  late final _titleController = TextEditingController(text: widget.profile?.title ?? '');
  late final _sourceController = TextEditingController(text: widget.profile?.sourceSpaceId ?? '');
  late final _targetsController = TextEditingController(
    text: widget.profile?.targetSpaceIds.join(', ') ?? '',
  );
  late bool _isDefault = widget.profile?.isDefault ?? false;
  late bool _isTemplate = widget.profile?.isTemplate ?? false;
  bool _saving = false;
  String? _error;

  static final _uuidPattern = RegExp(
    r'^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$',
  );

  @override
  void dispose() {
    _titleController.dispose();
    _sourceController.dispose();
    _targetsController.dispose();
    super.dispose();
  }

  bool _isUuid(String value) => _uuidPattern.hasMatch(value);

  Future<void> _save() async {
    final t = ref.read(tProvider);
    final title = _titleController.text.trim();
    if (title.isEmpty) {
      setState(() => _error = t('jce.profile.error_title_required'));
      return;
    }
    final source = _sourceController.text.trim();
    if (source.isNotEmpty && !_isUuid(source)) {
      setState(() => _error = t('jce.profile.error_source_invalid'));
      return;
    }
    final targets = _targetsController.text
        .split(',')
        .map((s) => s.trim())
        .where((s) => s.isNotEmpty)
        .toList();
    if (targets.isEmpty) {
      setState(() => _error = t('jce.profile.error_targets_required'));
      return;
    }
    for (final id in targets) {
      if (!_isUuid(id)) {
        setState(() => _error = t('jce.profile.error_target_invalid', {'id': id}));
        return;
      }
    }

    setState(() {
      _saving = true;
      _error = null;
    });

    try {
      final service = ref.read(jceServiceProvider);
      final input = ComparisonProfileInput(
        title: title,
        sourceSpaceId: source.isEmpty ? null : source,
        targetSpaceIds: targets,
        isDefault: _isDefault,
        isTemplate: _isTemplate,
      );
      if (widget.profile == null) {
        final created = await service.createProfile(input);
        if (created == null) {
          setState(() => _error = t('jce.profile.error_create_failed'));
          return;
        }
      } else {
        await service.updateProfile(widget.profile!.id, input);
      }
      ref.invalidate(jceProfilesProvider);
      if (mounted) Navigator.of(context).pop();
    } catch (e) {
      setState(() => _error = e.toString());
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final t = ref.watch(tProvider);
    final isEdit = widget.profile != null;

    return Padding(
      padding: EdgeInsets.only(
        bottom: MediaQuery.of(context).viewInsets.bottom,
        left: 16,
        right: 16,
        top: 16,
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(
            isEdit ? t('jce.profile.edit') : t('jce.profile.create'),
            style: JcTypography.heading,
          ),
          const SizedBox(height: 16),
          JcTextField(label: t('jce.profile.title_label'), controller: _titleController),
          const SizedBox(height: 12),
          JcTextField(label: t('jce.profile.source_space'), controller: _sourceController),
          const SizedBox(height: 12),
          JcTextField(label: t('jce.profile.target_spaces'), controller: _targetsController),
          const SizedBox(height: 8),
          SwitchListTile(
            title: Text(t('jce.profile.is_default')),
            value: _isDefault,
            onChanged: (v) => setState(() => _isDefault = v),
          ),
          SwitchListTile(
            title: Text(t('jce.profile.is_template')),
            value: _isTemplate,
            onChanged: (v) => setState(() => _isTemplate = v),
          ),
          if (_error != null)
            Padding(
              padding: const EdgeInsets.symmetric(vertical: 8),
              child: Text(_error!, style: TextStyle(color: JcColors.error)),
            ),
          const SizedBox(height: 16),
          if (_saving)
            const Center(child: CircularProgressIndicator())
          else
            JcButton(
              label: isEdit ? t('jce.profile.save') : t('jce.profile.create'),
              onPressed: _save,
            ),
          const SizedBox(height: 16),
        ],
      ),
    );
  }
}
