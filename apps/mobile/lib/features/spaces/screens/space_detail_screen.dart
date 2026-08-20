import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/spaces/providers/spaces_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class SpaceDetailScreen extends ConsumerStatefulWidget {
  final String slug;
  const SpaceDetailScreen({super.key, required this.slug});

  @override
  ConsumerState<SpaceDetailScreen> createState() => _SpaceDetailScreenState();
}

class _SpaceDetailScreenState extends ConsumerState<SpaceDetailScreen> {
  bool _loading = false;

  Future<void> _join() async {
    setState(() => _loading = true);
    try {
      await ref.read(spacesServiceProvider).join(widget.slug);
      if (mounted) {
        ref.invalidate(spaceDetailProvider(widget.slug));
      }
    } catch (e) {
      if (mounted) {
        final t = ref.read(tProvider);
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(t('errors.failed_join', {'error': e.toString()}))),
        );
      }
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _leave() async {
    setState(() => _loading = true);
    try {
      await ref.read(spacesServiceProvider).leave(widget.slug);
      if (mounted) {
        ref.invalidate(spaceDetailProvider(widget.slug));
      }
    } catch (e) {
      if (mounted) {
        final t = ref.read(tProvider);
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(t('errors.failed_leave', {'error': e.toString()}))),
        );
      }
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(spaceDetailProvider(widget.slug));
    final t = ref.watch(tProvider);
    return Scaffold(
      appBar: AppBar(title: Text(t('spaces.space'))),
      body: state.when(
        data: (space) => Column(
          children: [
            ListTile(title: Text(space.name ?? '')),
            Row(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                JcButton(
                  label: t('spaces.join'),
                  onPressed: _loading ? null : _join,
                ),
                const SizedBox(width: 16),
                JcButton(
                  label: t('spaces.leave'),
                  onPressed: _loading ? null : _leave,
                ),
              ],
            ),
          ],
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
          error: e,
          onRetry: () => ref.invalidate(spaceDetailProvider(widget.slug)),
        ),
      ),
    );
  }
}
