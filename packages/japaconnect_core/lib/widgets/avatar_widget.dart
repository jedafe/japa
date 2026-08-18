import 'package:cached_network_image/cached_network_image.dart';
import 'package:flutter/material.dart';

class AvatarWidget extends StatelessWidget {
  final String? url;
  final double size;

  const AvatarWidget({super.key, this.url, this.size = 40});

  @override
  Widget build(BuildContext context) {
    return ClipOval(
      child: SizedBox(
        width: size,
        height: size,
        child: url == null
            ? const ColoredBox(color: Colors.grey)
            : CachedNetworkImage(
                imageUrl: url!,
                fit: BoxFit.cover,
                placeholder: (_, __) => const ColoredBox(color: Colors.grey),
                errorWidget: (_, __, ___) => const Icon(Icons.person),
              ),
      ),
    );
  }
}
