import 'package:flutter_test/flutter_test.dart';
import 'package:frontend/api.dart';
import 'package:frontend/failure_text.dart';

void main() {
  group('needsSignOut', () {
    test('is true only for 401', () {
      expect(needsSignOut(ApiException(401, 'expired')), isTrue);
      expect(needsSignOut(ApiException(403, 'nope')), isFalse);
      expect(needsSignOut(ApiException(500, 'boom')), isFalse);
      expect(needsSignOut(ApiException(0, 'offline')), isFalse);
    });

    test('a 500 is not terminal, so a blip must not sign the user out', () {
      // The failure that this guards against is subtle and user-hostile: treating
      // a server error as terminal logs a signed-in user out over something
      // they did not cause and cannot fix.
      expect(needsSignOut(ApiException(503, 'unavailable')), isFalse);
    });
  });

  group('failureMessage', () {
    test('tells the user what to do when out of coins', () {
      // The one failure with a real remedy, so it must not fall through to the
      // server's wording, which for 402 is just a statement of fact.
      expect(
        failureMessage(ApiException(402, 'insufficient balance')),
        'Not enough coins. Visit the canteen counter to top up.',
      );
    });

    test('suggests retrying for a server fault', () {
      for (final status in [500, 502, 503]) {
        expect(
          failureMessage(ApiException(status, 'internal error')),
          'The canteen server had a problem. Try again in a moment.',
          reason: 'status $status is our fault and worth repeating',
        );
      }
    });

    test('passes the server wording through when we have nothing to add', () {
      expect(
        failureMessage(ApiException(400, 'Item unavailable')),
        'Item unavailable',
      );
      expect(
        failureMessage(ApiException(409, 'cannot move a completed order')),
        'cannot move a completed order',
      );
    });

    test('describes an unreachable server', () {
      final message = failureMessage(ApiException.networkFailure('timeout'));
      expect(message, contains('Could not reach the canteen server'));
      expect(message, contains('timeout'));
    });

    test('network failure is not mistaken for a status', () {
      // It carries no status, so it must not read as retryable-by-status or
      // trip any other comparison that assumes a real HTTP code.
      final failure = ApiException.networkFailure('boom');
      expect(failure.isRetryable, isFalse);
      expect(failure.isUnauthorized, isFalse);
      expect(failure.isPaymentRequired, isFalse);
    });
  });
}
