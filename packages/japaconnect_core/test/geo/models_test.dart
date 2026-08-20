import 'package:flutter_test/flutter_test.dart';
import 'package:japaconnect_core/japaconnect_core.dart';

void main() {
  group('GeoSpace', () {
    test('parses backend geo space response', () {
      final space = GeoSpace.fromJson({
        'id': '00000000-0000-0000-0000-000000000001',
        'name': 'Nigeria',
        'slug': 'nigeria',
        'level': 'country',
        'geo_code': 'NG',
        'description': 'The giant of Africa',
        'currency_code': 'NGN',
        'member_count': 10,
        'post_count': 20,
        'follower_count': 30,
      });

      expect(space.id, '00000000-0000-0000-0000-000000000001');
      expect(space.name, 'Nigeria');
      expect(space.slug, 'nigeria');
      expect(space.level, 'country');
      expect(space.geoCode, 'NG');
      expect(space.description, 'The giant of Africa');
      expect(space.currencyCode, 'NGN');
      expect(space.memberCount, 10);
      expect(space.postCount, 20);
      expect(space.followerCount, 30);
      expect(space.isCountry, true);
    });

    test('defaults numeric fields to zero', () {
      final space = GeoSpace.fromJson({
        'id': '1',
        'name': 'Lagos',
        'slug': 'lagos',
        'level': 'city',
      });

      expect(space.memberCount, 0);
      expect(space.postCount, 0);
      expect(space.followerCount, 0);
    });
  });
}
