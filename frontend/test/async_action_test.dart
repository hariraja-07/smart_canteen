import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:frontend/core/api.dart';
import 'package:frontend/core/async_action.dart';
import 'package:frontend/core/failure_text.dart';
import 'package:frontend/core/models.dart';
import 'package:frontend/core/session.dart';

/// [runMutation] decides what a failed request does to the user: it reports the
/// message, or signs them out when the session has ended. Both paths end the
/// interaction, so both are asserted here directly rather than through
/// whichever screen happened to exercise them first.

Session _signedIn() {
  final s = Session(api: ApiClient());
  s.debugSetUser(
    const User(
      id: 17,
      name: 'Ravi',
      email: 'ravi@x.test',
      role: Role.student,
      coinBalance: 50,
    ),
  );
  return s;
}

/// Runs [action] from a button and hands the result to [onResult], so a test can
/// check the return value and the screen in the same interaction.
class _Runner extends StatelessWidget {
  final Future<String> Function() action;
  final ValueChanged<String?> onResult;

  const _Runner({required this.action, required this.onResult});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Center(
        child: FilledButton(
          onPressed: () async {
            final result = await runMutation(context, action);
            onResult(result);
          },
          child: const Text('go'),
        ),
      ),
    );
  }
}

/// A minimal tree carrying only a session, since the behaviour under test is
/// the failure handling and needs no menu, cart or routes around it.
Future<Session> _pump(
  WidgetTester tester,
  Future<String> Function() action, {
  ValueChanged<String?>? onResult,
}) async {
  final session = _signedIn();
  // MaterialApp goes outside the runner on purpose. runMutation shows messages
  // through the ScaffoldMessenger, so the context it is handed has to sit below
  // the MaterialApp that installs one, exactly as it does for a real page.
  await tester.pumpWidget(
    SessionScope(
      session: session,
      child: MaterialApp(
        home: _Runner(action: action, onResult: onResult ?? (_) {}),
      ),
    ),
  );
  return session;
}

void main() {
  testWidgets('a successful action returns its value and shows nothing', (
    tester,
  ) async {
    String? received = 'untouched';
    await _pump(
      tester,
      () async => 'placed',
      onResult: (value) => received = value,
    );

    await tester.tap(find.text('go'));
    await tester.pumpAndSettle();

    expect(received, 'placed');
    expect(find.byType(SnackBar), findsNothing);
  });

  testWidgets('a server fault shows the retry wording and returns null', (
    tester,
  ) async {
    String? received = 'untouched';
    await _pump(
      tester,
      () async => throw ApiException(500, 'internal error'),
      onResult: (value) => received = value,
    );

    await tester.tap(find.text('go'));
    await tester.pumpAndSettle();

    expect(received, isNull);
    expect(find.byType(SnackBar), findsOneWidget);
    expect(
      find.text('The canteen server had a problem. Try again in a moment.'),
      findsOneWidget,
    );
  });

  // The case where a message is the wrong answer: the token is gone, so every
  // other screen would fail too, and "try again" sends someone back to a button
  // that cannot work.
  testWidgets('an expired session signs out instead of showing a message', (
    tester,
  ) async {
    final session = await _pump(
      tester,
      () async => throw ApiException(401, 'expired'),
    );
    expect(session.isSignedIn, isTrue);

    await tester.tap(find.text('go'));
    await tester.pumpAndSettle();

    expect(session.isSignedIn, isFalse);
    expect(find.byType(SnackBar), findsNothing);
  });

  testWidgets('an unreachable server is described as such', (tester) async {
    await _pump(tester, () async => throw const Socketish());

    await tester.tap(find.text('go'));
    await tester.pumpAndSettle();

    expect(find.byType(SnackBar), findsOneWidget);
    expect(
      find.textContaining('Could not reach the canteen server'),
      findsOneWidget,
    );
  });

  // Guards against a caller keeping its own copy of the wording, which is what
  // the five copies this replaced all did.
  testWidgets('the wording comes from failureMessage', (tester) async {
    await _pump(tester, () async => throw ApiException(402, 'no coins'));

    await tester.tap(find.text('go'));
    await tester.pumpAndSettle();

    expect(
      find.text(failureMessage(ApiException(402, 'no coins'))),
      findsOneWidget,
    );
  });
}

/// Stands in for the transport failing: a socket error is an Exception that is
/// not an ApiException, which is the path that decides for itself what to say.
class Socketish implements Exception {
  const Socketish();

  @override
  String toString() => 'connection refused';
}
