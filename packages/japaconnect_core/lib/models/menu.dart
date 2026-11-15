/// Menu item as returned by GET /menus/{slug} (ADR-054). Visibility,
/// permission_node, feature_flag and ADR-128 intent shaping are already
/// resolved server-side — clients render `items` as received.
class JcMenuItem {
  final String id;
  final String label;
  final String url;
  final String? icon;
  final int sortOrder;
  final String visibility;
  final String target;
  final List<JcMenuItem> children;

  const JcMenuItem({
    required this.id,
    required this.label,
    required this.url,
    this.icon,
    this.sortOrder = 0,
    this.visibility = 'all',
    this.target = '_self',
    this.children = const [],
  });

  factory JcMenuItem.fromJson(Map<String, dynamic> json) => JcMenuItem(
        id: json['id'] as String? ?? '',
        label: json['label'] as String? ?? '',
        url: json['url'] as String? ?? '',
        icon: json['icon'] as String?,
        sortOrder: json['sort_order'] as int? ?? 0,
        visibility: json['visibility'] as String? ?? 'all',
        target: json['target'] as String? ?? '_self',
        children: [
          for (final c in (json['children'] as List? ?? const []))
            JcMenuItem.fromJson(c as Map<String, dynamic>),
        ],
      );
}

/// Resolved menu response for GET /menus/{slug}.
class JcMenu {
  final String id;
  final String slug;
  final String name;
  final String placement;
  final List<JcMenuItem> items;

  const JcMenu({
    required this.id,
    required this.slug,
    required this.name,
    required this.placement,
    required this.items,
  });

  factory JcMenu.fromJson(Map<String, dynamic> json) => JcMenu(
        id: json['id'] as String? ?? '',
        slug: json['slug'] as String? ?? '',
        name: json['name'] as String? ?? '',
        placement: json['placement'] as String? ?? '',
        items: [
          for (final i in (json['items'] as List? ?? const []))
            JcMenuItem.fromJson(i as Map<String, dynamic>),
        ],
      );
}
