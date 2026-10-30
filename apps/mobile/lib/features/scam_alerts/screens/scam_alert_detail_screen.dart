// Ref: japaconnect-engineering/sections/81-scam-alert-board.md
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_mobile/features/scam_alerts/providers/scam_alerts_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class ScamAlertDetailScreen extends ConsumerStatefulWidget {
  final String id;
  const ScamAlertDetailScreen({super.key, required this.id});

  @override
  ConsumerState<ScamAlertDetailScreen> createState() => _ScamAlertDetailState();
}

class _ScamAlertDetailState extends ConsumerState<ScamAlertDetailScreen> {
  bool _subscribed = false;
  bool _busy = false;
  bool _seeded = false;

  Future<void> _toggle() async {
    final t = ref.read(tProvider);
    setState(() => _busy = true);
    try {
      if (_subscribed) {
        await ref.read(scamAlertsServiceProvider).unsubscribe(widget.id);
      } else {
        await ref.read(scamAlertsServiceProvider).subscribe(widget.id);
      }
      if (mounted) setState(() => _subscribed = !_subscribed);
    } catch (_) {
      if (mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text(t('scam_alerts.form.sign_in'))));
      }
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final t = ref.watch(tProvider);
    final alert = ref.watch(scamAlertDetailProvider(widget.id));
    return Scaffold(
      appBar: AppBar(title: Text(t('scam_alerts.title'))),
      body: alert.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(child: Text(t('common.retry'))),
        data: (a) {
          if (!_seeded) {
            _subscribed = a.isSubscribed;
            _seeded = true;
          }
          return ListView(
            padding: const EdgeInsets.all(16),
            children: [
              Align(
                alignment: Alignment.centerRight,
                child: TextButton.icon(
                  onPressed: _busy ? null : _toggle,
                  icon: Icon(
                    _subscribed ? Icons.notifications_off : Icons.notifications_active,
                    size: 18,
                  ),
                  label: Text(_subscribed ? t('scam_alerts.unsubscribe') : t('scam_alerts.subscribe')),
                ),
              ),
              if (a.isPinned)
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
                  margin: const EdgeInsets.only(bottom: 12),
                  decoration: BoxDecoration(
                    color: Colors.red.withValues(alpha: 0.1),
                    borderRadius: BorderRadius.circular(8),
                  ),
                  child: Text(
                    t('scam_alerts.pinned'),
                    style: const TextStyle(color: Colors.red, fontWeight: FontWeight.w600),
                  ),
                ),
              Text(
                '${a.scamType.replaceAll('_', ' ')} · ${a.destinationSpaceName}',
                style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 16),
              ),
              const SizedBox(height: 12),
              Text(a.description, style: const TextStyle(fontSize: 15, height: 1.5)),
              if (a.platformUsed != null) ...[
                const SizedBox(height: 16),
                Text(
                  '${t('scam_alerts.platform')}: ${a.platformUsed}',
                  style: const TextStyle(color: Colors.grey, fontSize: 13),
                ),
              ],
            ],
          );
        },
      ),
    );
  }
}
