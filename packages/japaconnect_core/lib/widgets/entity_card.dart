import 'package:flutter/material.dart';
import 'package:japaconnect_core/widgets/avatar_widget.dart';

class EntityCard extends StatelessWidget {
  final dynamic item;
  final VoidCallback? onTap;

  const EntityCard({super.key, required this.item, this.onTap});

  @override
  Widget build(BuildContext context) {
    final title = item['title'] ?? '';
    final subtitle = item['subtitle'] ?? '';
    final avatar = item['avatar_url'] as String?;

    return ListTile(
      leading: AvatarWidget(url: avatar, size: 40),
      title: Text(title),
      subtitle: Text(subtitle, maxLines: 2, overflow: TextOverflow.ellipsis),
      onTap: onTap,
    );
  }
}
