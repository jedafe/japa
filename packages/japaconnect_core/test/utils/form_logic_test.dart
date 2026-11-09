import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:japaconnect_core/japaconnect_core.dart';

/// Parity vectors — same cases as internal/formlogic/logic_test.go (Go) and
/// packages/formlogic/src/index.test.ts (TS). Conditions arrive as decoded
/// JSON maps, so literals are written through jsonDecode for realism.
Map<String, dynamic> dec(String s) => jsonDecode(s) as Map<String, dynamic>;

void main() {
  group('evalClause ops', () {
    final answers = <String, dynamic>{
      'purpose': 'study',
      'pets': ['dog', 'cat'],
      'score': 80.0,
      'flag': true,
      'empty': '',
      'emptyarr': <dynamic>[],
    };
    bool run(String clauseJson) =>
        evalCondition(<String, dynamic>{'all': [dec(clauseJson)]}, answers, 'purpose');

    test('eq', () {
      expect(run('{"field":"purpose","op":"eq","value":"study"}'), isTrue);
      expect(run('{"field":"purpose","op":"eq","value":"work"}'), isFalse);
      expect(run('{"field":"score","op":"eq","value":"80"}'), isTrue);
      expect(run('{"field":"flag","op":"eq","value":"true"}'), isTrue);
    });
    test('neq', () {
      expect(run('{"field":"purpose","op":"neq","value":"work"}'), isTrue);
      expect(run('{"field":"purpose","op":"neq","value":"study"}'), isFalse);
      expect(run('{"field":"nope","op":"neq","value":"x"}'), isFalse);
    });
    test('in', () {
      expect(run('{"field":"purpose","op":"in","value":["work","study"]}'), isTrue);
      expect(run('{"field":"purpose","op":"in","value":["work"]}'), isFalse);
    });
    test('contains', () {
      expect(run('{"field":"pets","op":"contains","value":"dog"}'), isTrue);
      expect(run('{"field":"pets","op":"contains","value":"fish"}'), isFalse);
      expect(run('{"field":"purpose","op":"contains","value":"x"}'), isFalse);
    });
    test('answered / not_answered', () {
      expect(run('{"field":"purpose","op":"answered"}'), isTrue);
      expect(run('{"field":"empty","op":"answered"}'), isFalse);
      expect(run('{"field":"emptyarr","op":"answered"}'), isFalse);
      expect(run('{"field":"nope","op":"answered"}'), isFalse);
      expect(run('{"field":"nope","op":"not_answered"}'), isTrue);
      expect(run('{"field":"purpose","op":"not_answered"}'), isFalse);
    });
    test('omitted field defaults to selfKey', () {
      expect(run('{"op":"eq","value":"study"}'), isTrue);
      expect(run('{"op":"eq","value":"work"}'), isFalse);
    });
  });

  group('evalCondition all/any', () {
    final answers = <String, dynamic>{'a': 'x', 'b': 'y'};
    test('all passes', () {
      expect(
        evalCondition(dec('{"all":[{"field":"a","op":"eq","value":"x"},{"field":"b","op":"eq","value":"y"}]}'), answers),
        isTrue,
      );
    });
    test('all&&any: failed any-group → false', () {
      expect(
        evalCondition(dec('{"all":[{"field":"a","op":"eq","value":"x"}],"any":[{"field":"b","op":"eq","value":"z"}]}'), answers),
        isFalse,
      );
    });
    test('null condition → true (fail-open)', () {
      expect(evalCondition(null, answers), isTrue);
    });
    test('malformed clause → false (Go/TS parity)', () {
      // A non-map clause is a broken rule element — it must fail like the
      // zero-value Clause{} in Go/TS, not silently pass.
      expect(evalCondition(dec('{"all":[123]}'), answers), isFalse);
      expect(evalCondition(dec('{"any":[123]}'), answers), isFalse);
      expect(evalCondition(dec('{"all":[null]}'), answers), isFalse);
    });
    test('non-string clause field falls back to selfKey (Go unmarshal-drop parity)', () {
      // Go drops a non-string field on unmarshal → "" → selfKey.
      expect(
        evalCondition(dec('{"all":[{"field":123,"op":"eq","value":1}]}'), {'self': 1}, 'self'),
        isTrue,
      );
    });
    test('non-string field key does not throw', () {
      // key:123 — Go's typed field is a string by construction; the JSON
      // shape check treats a non-string key as "".
      final out = visibleFields(
        [
          {'key': 123, 'config': <String, dynamic>{}},
        ],
        {},
      );
      expect(out, hasLength(1));
    });
  });

  group('fieldVisible', () {
    test('absent/malformed config → visible', () {
      expect(fieldVisible(null, {}), isTrue);
      expect(fieldVisible(<String, dynamic>{}, {}), isTrue);
      expect(fieldVisible(dec('{"visible_if":"nonsense"}'), {}), isTrue);
    });
    test('failing rule hides, passing rule shows', () {
      final cfg = dec('{"visible_if":{"all":[{"field":"a","op":"eq","value":1}]}}');
      expect(fieldVisible(cfg, {'a': 'nope'}), isFalse);
      expect(fieldVisible(cfg, {'a': 1.0}), isTrue);
    });
  });

  group('hiddenStepsByJump', () {
    const order = ['s1', 's2', 's3', 's4', 's5'];
    Map<String, dynamic> mk(String key, String step, String cfgJson) => <String, dynamic>{
          'key': key,
          'config': <String, dynamic>{'dimension': step, ...dec(cfgJson)},
        };

    test('forward jump hides open interval', () {
      final fields = [
        mk('f1', 's1', '{"jump":{"when":{"any":[{"op":"eq","value":"go"}]},"step":"s4"}}'),
        mk('f2', 's2', '{}'),
      ];
      final h = hiddenStepsByJump(fields, order, {'f1': 'go'});
      expect(h, {'s2', 's3'});
      expect(hiddenStepsByJump(fields, order, {'f1': 'stop'}), isEmpty);
    });

    test('backward/self/unknown targets ignored', () {
      final fields = [
        mk('f3', 's3', '{"jump":{"when":{"any":[{"op":"eq","value":1}]},"step":"s1"}}'),
        mk('f4', 's3', '{"jump":{"when":{"any":[{"op":"eq","value":1}]},"step":"s3"}}'),
        mk('f5', 's3', '{"jump":{"when":{"any":[{"op":"eq","value":1}]},"step":"ghost"}}'),
      ];
      expect(hiddenStepsByJump(fields, order, {'f3': 1.0, 'f4': 1.0, 'f5': 1.0}), isEmpty);
    });

    test('hidden firing field cannot jump', () {
      final fields = [
        mk('f6', 's1',
            '{"visible_if":{"all":[{"field":"gate","op":"eq","value":0}]},"jump":{"when":{"any":[{"op":"eq","value":1}]},"step":"s3"}}'),
      ];
      expect(hiddenStepsByJump(fields, order, {'f6': 1.0}), isEmpty);
    });

    test('field in a jump-hidden step cannot fire', () {
      final fields = [
        mk('j1', 's1', '{"jump":{"when":{"any":[{"op":"eq","value":"a"}]},"step":"s4"}}'),
        mk('j2', 's2', '{"jump":{"when":{"any":[{"op":"eq","value":"b"}]},"step":"s5"}}'),
        mk('j3', 's4', '{"jump":{"when":{"any":[{"op":"eq","value":"c"}]},"step":"s5"}}'),
      ];
      // j2's step s2 is hidden by j1's jump → suppressed; s4 stays visible.
      final h = hiddenStepsByJump(fields, order, {'j1': 'a', 'j2': 'b', 'j3': 'c'});
      expect(h, {'s2', 's3'});
    });

    test('union of intervals from visible sources', () {
      final fields = [
        mk('j1', 's1', '{"jump":{"when":{"any":[{"op":"eq","value":"a"}]},"step":"s3"}}'),
        mk('j2', 's2', '{"jump":{"when":{"any":[{"op":"eq","value":"b"}]},"step":"s5"}}'),
        mk('j3', 's3', '{"jump":{"when":{"any":[{"op":"eq","value":"c"}]},"step":"s5"}}'),
      ];
      final h = hiddenStepsByJump(fields, order, {'j1': 'a', 'j2': 'b', 'j3': 'c'});
      expect(h, {'s2', 's4'});
    });

    test('seeded stepHidden suppresses its fields\' jumps', () {
      final fields = [
        mk('j1', 's1', '{"jump":{"when":{"any":[{"op":"eq","value":"a"}]},"step":"s3"}}'),
        mk('j3', 's3', '{"jump":{"when":{"any":[{"op":"eq","value":"c"}]},"step":"s5"}}'),
      ];
      final h = hiddenStepsByJump(fields, order, {'j1': 'a', 'j3': 'c'}, stepHidden: {'s3'});
      expect(h, {'s2', 's3'}); // j3 suppressed by the seed; s3 stays in the set
    });

    test('when-less jump fires iff the owning field is answered', () {
      final fields = [mk('u1', 's1', '{"jump":{"step":"s3"}}')];
      expect(hiddenStepsByJump(fields, order, {'u1': 'x'}), {'s2'});
      expect(hiddenStepsByJump(fields, order, {}), isEmpty);
      expect(hiddenStepsByJump(fields, order, {'u1': ''}), isEmpty);
    });
  });
}
