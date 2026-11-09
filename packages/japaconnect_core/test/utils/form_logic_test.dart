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

    test('union of intervals chains across hidden steps', () {
      final fields = [
        mk('j1', 's1', '{"jump":{"when":{"any":[{"op":"eq","value":"a"}]},"step":"s4"}}'),
        mk('j2', 's2', '{"jump":{"when":{"any":[{"op":"eq","value":"b"}]},"step":"s5"}}'),
      ];
      final h = hiddenStepsByJump(fields, order, {'j1': 'a', 'j2': 'b'});
      expect(h, {'s2', 's3', 's4'});
    });
  });
}
