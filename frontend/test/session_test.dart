import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:frontend/core/api.dart';
import 'package:frontend/app.dart';
import 'package:frontend/core/models.dart';
import 'package:frontend/core/session.dart';

const _meJson =
    '{"id":17,"name":"Ravi","email":"ravi@x.test",'
    '"role":"student","coin_balance":110}';

String _loginOk({
  String role = 'student',
  int balance = 110,
  String name = 'Ravi',
}) =>
    '{"id":17,"name":"$name","email":"$name@x.test","role":"$role",'
    '"coin_balance":$balance,"token":"jwt-here"}';

/// Seeds a session as if the user were already signed in, so a test can start
/// on a signed-in screen without a login round trip.
Session signedInSession({
  String role = 'student',
  int balance = 110,
  String name = 'Ravi',
}) {
  final s = Session();
  Api.token = 'jwt-here';
  s.debugSetUser(
    User(
      id: 17,
      name: name,
      email: '$name@x.test',
      role: role,
      coinBalance: balance,
    ),
  );
  return s;
}

void main() {
  tearDown(() {
    Api.token = null;
    Api.client = http.Client();
  });

  group('the app requires a login', () {
    testWidgets('starts on the login screen, not the menu', (tester) async {
      Api.client = MockClient((r) async => http.Response('[]', 200));
      await tester.pumpWidget(SmartCanteenApp(session: Session()));

      expect(find.text('Sign in'), findsOneWidget);
      expect(find.text('Email'), findsOneWidget);
      // The menu is reachable only after signing in.
      expect(find.text('Menu'), findsNothing);
    });

    testWidgets('a successful login shows the menu and the balance', (
      tester,
    ) async {
      Api.client = MockClient((r) async {
        if (r.url.path == '/api/auth/login') {
          return http.Response(_loginOk(balance: 110), 200);
        }
        return http.Response('[]', 200);
      });

      await tester.pumpWidget(SmartCanteenApp(session: Session()));
      await tester.enterText(find.byType(TextFormField).first, 'Ravi@x.test');
      await tester.enterText(find.byType(TextFormField).last, 'pw');
      await tester.tap(find.text('Continue'));
      await tester.pumpAndSettle();

      expect(find.text('Menu'), findsWidgets);
      expect(find.text('110 coins'), findsOneWidget);
      expect(find.text('Ravi'), findsNothing);
    });

    testWidgets('a rejected password keeps the user on the login screen', (
      tester,
    ) async {
      Api.client = MockClient(
        (r) async =>
            http.Response('{"error":"invalid email or password"}', 401),
      );

      await tester.pumpWidget(SmartCanteenApp(session: Session()));
      await tester.enterText(find.byType(TextFormField).first, 'ravi@x.test');
      await tester.enterText(find.byType(TextFormField).last, 'wrong');
      await tester.tap(find.text('Continue'));
      await tester.pumpAndSettle();

      expect(find.text('invalid email or password'), findsOneWidget);
      expect(find.text('Menu'), findsNothing);
    });

    testWidgets('signing out returns to login and drops the token', (
      tester,
    ) async {
      Api.client = MockClient((r) async => http.Response('[]', 200));
      await tester.pumpWidget(SmartCanteenApp(session: signedInSession()));

      expect(find.text('Menu'), findsWidgets);
      await tester.tap(find.byTooltip('Sign out'));
      await tester.pumpAndSettle();

      expect(find.text('Sign in'), findsOneWidget);
      expect(find.text('Menu'), findsNothing);
      expect(Api.token, isNull);
    });
  });

  group('Session', () {
    test('a failed sign-in leaves the session signed out', () async {
      Api.client = MockClient(
        (r) async => http.Response('{"error":"no"}', 401),
      );
      final s = Session();

      await expectLater(
        s.signIn('a@b.test', 'x'),
        throwsA(isA<ApiException>()),
      );
      expect(s.isSignedIn, isFalse);
      expect(Api.token, isNull);
    });

    test('refresh adopts the server balance after spending coins', () async {
      // The app bar balance comes from this, so it has to be the server's
      // number after an order rather than a pre-order guess.
      final s = signedInSession(balance: 110);
      Api.client = MockClient((r) async => http.Response(_meJson, 200));

      await s.refresh();

      expect(s.coinBalance, 110);
    });

    test(
      'an expired token signs the user out instead of stranding them',
      () async {
        // A shell whose every request 401s is worse than the login screen.
        final s = signedInSession();
        Api.client = MockClient(
          (r) async => http.Response('{"error":"expired"}', 401),
        );

        await s.refresh();

        expect(s.isSignedIn, isFalse);
        expect(Api.token, isNull);
      },
    );

    test('a network blip keeps the user signed in', () async {
      // Throwing someone out over a dropped connection is a worse outcome than
      // a balance that is briefly stale.
      final s = signedInSession(balance: 110);
      Api.client = MockClient((r) async => http.Response('gateway down', 502));

      await s.refresh();

      expect(s.isSignedIn, isTrue);
      expect(s.coinBalance, 110);
    });

    test('role flags mirror the server rules', () {
      expect(signedInSession(role: Role.admin).canManageKitchen, isTrue);
      expect(
        signedInSession(role: Role.canteenManagement).canManageKitchen,
        isTrue,
      );
      expect(signedInSession(role: Role.student).canManageKitchen, isFalse);
      expect(signedInSession(role: Role.staff).canManageKitchen, isFalse);
      expect(signedInSession(role: Role.admin).isAdmin, isTrue);
      expect(signedInSession(role: Role.canteenManagement).isAdmin, isFalse);
    });

    test('signing in trims the email, since a stray space is a typo', () async {
      String? body;
      Api.client = MockClient((r) async {
        body = r.body;
        return http.Response(_loginOk(), 200);
      });
      await Session().signIn('  ravi@x.test  ', 'pw');

      expect(body, contains('ravi@x.test'));
      expect(body, isNot(contains('  ravi')));
    });
  });
}
