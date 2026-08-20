import 'package:japaconnect_core/api/generated/lib/api.dart' show Space, Tag, User;

/// Extensions that produce the card-map shape expected by [EntityCard]
/// (`title`, `subtitle`, `avatar_url`). This lets generic list screens keep
/// using [EntityCard] while the service layer works with typed models.
extension SpaceCardMap on Space {
  Map<String, dynamic> toCardMap() => {
        'title': name ?? '',
        'subtitle': description ?? '',
        'avatar_url': null,
      };
}

extension UserCardMap on User {
  Map<String, dynamic> toCardMap() => {
        'title': displayName ?? username ?? '',
        'subtitle': bio ?? '',
        'avatar_url': avatarUrl,
      };
}

extension TagCardMap on Tag {
  Map<String, dynamic> toCardMap() => {
        'title': name ?? '',
        'subtitle': slug ?? '',
        'avatar_url': null,
      };
}
