import 'package:japaconnect_core/models/content.dart';
import 'package:japaconnect_core/widgets/entity_card.dart';

class FeedCard extends EntityCard {
  FeedCard({super.key, required Content item, super.onTap})
      : super(item: item.toCardMap());
}
