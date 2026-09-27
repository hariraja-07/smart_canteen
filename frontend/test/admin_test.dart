import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:frontend/core/api.dart';
import 'package:frontend/app.dart';
import 'package:frontend/core/models.dart';
import 'package:frontend/core/session.dart';

Session signedIn({String role = Role.admin}) {
  final s = Session(api: api);
  api.token = 'jwt';
  s.debugSetUser(
    User(
      id: 15,
      name: 'Anita',
      email: 'admin@x.test',
      role: role,
      coinBalance: 0,
    ),
  );
  return s;
}

String userJson(int id, String name, String role, int balance) => jsonEncode({
  'id': id,
  'name': name,
  'email': '${name.toLowerCase()}@x.test',
  'role': role,
  'coin_balance': balance,
});

/// Roster plus a record of every exchange.
class Roster {
  List<Map<String, dynamic>> users;
  final List<Map<String, dynamic>> exchanges = [];
  int? rejectWith;

  Roster(this.users);

  MockClient client() => MockClient((r) async {
    if (r.url.path == '/api/menu') return http.Response('[]', 200);
    if (r.url.path == '/api/users/15/coins') {
      return http.Response('[]', 200);
    }
    if (r.url.path == '/api/admin/users') {
      return http.Response(jsonEncode(users), 200);
    }
    if (r.url.path.startsWith('/api/admin/users/') &&
        r.url.path.endsWith('/coins') &&
        r.method == 'POST') {
      final body = jsonDecode(r.body) as Map<String, dynamic>;
      exchanges.add({...body, 'id': int.parse(r.url.path.split('/')[4])});
      final reject = rejectWith;
      if (reject != null) {
        return http.Response(
          '{"error":"amount must be a positive whole number of coins"}',
          reject,
        );
      }
      final id = int.parse(r.url.path.split('/')[4]);
      final user = users.firstWhere((u) => u['id'] == id);
      user['coin_balance'] =
          (user['coin_balance'] as int) + (body['amount'] as int);
      return http.Response(jsonEncode(user), 201);
    }
    return http.Response('[]', 200);
  });
}

Future<void> openAdmin(WidgetTester tester) async {
  await tester.tap(find.text('Admin'));
  await tester.pumpAndSettle();
}

/// The client the app under test talks through. Each test assigns the
/// transport it needs before pumping, so nothing is shared between them
/// and no tearDown is left over to undo one test leaking into the next.
late ApiClient api;

void main() {
  setUp(() => api = ApiClient());

  group('who gets the admin tab', () {
    testWidgets('the admin gets it', (tester) async {
      api = ApiClient(httpClient: Roster([]).client());
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));
      await tester.pumpAndSettle();

      expect(find.text('Admin'), findsWidgets);
    });

    testWidgets('nobody else does', (tester) async {
      // The canteen manages the kitchen but does not mint coins, and the two
      // routes this screen uses are admin-only on the server.
      for (final role in [Role.canteenManagement, Role.student, Role.staff]) {
        api = ApiClient(httpClient: Roster([]).client());
        await tester.pumpWidget(SmartCanteenApp(session: signedIn(role: role)));
        await tester.pumpAndSettle();
        expect(find.text('Admin'), findsNothing, reason: 'shown for $role');
      }
    });
  });

  group('the roster', () {
    testWidgets('lists each account with its balance', (tester) async {
      api = ApiClient(
        httpClient: Roster([
          {
            'id': 17,
            'name': 'Ravi',
            'email': 'ravi@x.test',
            'role': 'student',
            'coin_balance': 110,
          },
          {
            'id': 18,
            'name': 'Priya',
            'email': 'priya@x.test',
            'role': 'student',
            'coin_balance': 60,
          },
        ]).client(),
      );
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));
      await tester.pumpAndSettle();
      await openAdmin(tester);

      expect(find.text('Ravi'), findsOneWidget);
      expect(find.text('Priya'), findsOneWidget);
      expect(find.text('ravi@x.test  student'), findsOneWidget);
      expect(find.text('110'), findsOneWidget);
      expect(find.text('60'), findsOneWidget);
    });

    testWidgets('no accounts reads as an empty roster', (tester) async {
      api = ApiClient(httpClient: Roster([]).client());
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));
      await tester.pumpAndSettle();
      await openAdmin(tester);

      expect(find.text('No accounts yet'), findsOneWidget);
    });
  });

  group('giving coins', () {
    testWidgets('sends the amount and the reason, then updates the balance', (
      tester,
    ) async {
      final roster = Roster([
        {
          'id': 17,
          'name': 'Ravi',
          'email': 'ravi@x.test',
          'role': 'student',
          'coin_balance': 10,
        },
      ]);
      api = ApiClient(httpClient: roster.client());
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));
      await tester.pumpAndSettle();
      await openAdmin(tester);

      await tester.tap(find.text('Add coins'));
      await tester.pumpAndSettle();
      await tester.enterText(find.byType(TextFormField).first, '100');
      await tester.enterText(
        find.byType(TextFormField).last,
        'cash at counter',
      );
      await tester.tap(find.text('Give coins'));
      await tester.pumpAndSettle();

      expect(roster.exchanges.single['id'], 17);
      expect(roster.exchanges.single['amount'], 100);
      expect(roster.exchanges.single['reason'], 'cash at counter');
      expect(find.text('Ravi now has 110 coins'), findsOneWidget);
      expect(find.text('110'), findsOneWidget);
    });

    testWidgets('a reason is required, because the ledger explains the coins', (
      tester,
    ) async {
      final roster = Roster([
        {
          'id': 17,
          'name': 'Ravi',
          'email': 'ravi@x.test',
          'role': 'student',
          'coin_balance': 10,
        },
      ]);
      api = ApiClient(httpClient: roster.client());
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));
      await tester.pumpAndSettle();
      await openAdmin(tester);

      await tester.tap(find.text('Add coins'));
      await tester.pumpAndSettle();
      await tester.enterText(find.byType(TextFormField).first, '100');
      // No reason, only whitespace.
      await tester.enterText(find.byType(TextFormField).last, '   ');
      await tester.tap(find.text('Give coins'));
      await tester.pumpAndSettle();

      expect(find.text('A reason is required'), findsOneWidget);
      // Nothing was sent, so no coins were minted on a guess.
      expect(roster.exchanges, isEmpty);
    });

    testWidgets('zero and empty amounts are refused before sending', (
      tester,
    ) async {
      final roster = Roster([
        {
          'id': 17,
          'name': 'Ravi',
          'email': 'ravi@x.test',
          'role': 'student',
          'coin_balance': 10,
        },
      ]);
      api = ApiClient(httpClient: roster.client());
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));
      await tester.pumpAndSettle();
      await openAdmin(tester);

      await tester.tap(find.text('Add coins'));
      await tester.pumpAndSettle();
      // The field takes digits only, so a decimal cannot even be typed.
      await tester.enterText(find.byType(TextFormField).first, '0');
      await tester.enterText(find.byType(TextFormField).last, 'cash');
      await tester.tap(find.text('Give coins'));
      await tester.pumpAndSettle();

      expect(find.text('Must be at least 1 coin'), findsOneWidget);
      expect(roster.exchanges, isEmpty);
    });

    testWidgets('dismissing the dialog exchanges nothing', (tester) async {
      final roster = Roster([
        {
          'id': 17,
          'name': 'Ravi',
          'email': 'ravi@x.test',
          'role': 'student',
          'coin_balance': 10,
        },
      ]);
      api = ApiClient(httpClient: roster.client());
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));
      await tester.pumpAndSettle();
      await openAdmin(tester);

      await tester.tap(find.text('Add coins'));
      await tester.pumpAndSettle();
      await tester.enterText(find.byType(TextFormField).first, '100');
      await tester.enterText(find.byType(TextFormField).last, 'cash');
      await tester.tap(find.text('Cancel'));
      await tester.pumpAndSettle();

      expect(roster.exchanges, isEmpty);
      expect(find.text('Add coins'), findsOneWidget);
    });

    testWidgets('a rejection is shown and nothing is credited locally', (
      tester,
    ) async {
      final roster = Roster([
        {
          'id': 17,
          'name': 'Ravi',
          'email': 'ravi@x.test',
          'role': 'student',
          'coin_balance': 10,
        },
      ])..rejectWith = 400;
      api = ApiClient(httpClient: roster.client());
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));
      await tester.pumpAndSettle();
      await openAdmin(tester);

      await tester.tap(find.text('Add coins'));
      await tester.pumpAndSettle();
      await tester.enterText(find.byType(TextFormField).first, '100');
      await tester.enterText(find.byType(TextFormField).last, 'cash');
      await tester.tap(find.text('Give coins'));
      await tester.pumpAndSettle();

      expect(find.textContaining('positive whole number'), findsOneWidget);
      // Still 10, because the server refused and the client did not invent a
      // balance.
      expect(find.text('10'), findsOneWidget);
    });

    testWidgets('an expired token signs the admin out', (tester) async {
      final roster = Roster([
        {
          'id': 17,
          'name': 'Ravi',
          'email': 'ravi@x.test',
          'role': 'student',
          'coin_balance': 10,
        },
      ])..rejectWith = 401;
      api = ApiClient(httpClient: roster.client());
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));
      await tester.pumpAndSettle();
      await openAdmin(tester);

      await tester.tap(find.text('Add coins'));
      await tester.pumpAndSettle();
      await tester.enterText(find.byType(TextFormField).first, '100');
      await tester.enterText(find.byType(TextFormField).last, 'cash');
      await tester.tap(find.text('Give coins'));
      await tester.pumpAndSettle();

      expect(find.text('Sign in'), findsOneWidget);
    });
  });

  group('fetchUsers', () {
    test('parses a roster', () async {
      api = ApiClient(
        httpClient: MockClient(
          (r) async => http.Response(
            '[{"id":17,"name":"Ravi","email":"r@x.test","role":"student","coin_balance":5}]',
            200,
          ),
        ),
      );
      final users = await api.fetchUsers();
      expect(users.single.name, 'Ravi');
      expect(users.single.coinBalance, 5);
    });

    test('a roster with no password field still parses', () async {
      // The server omits the hash, so a client that required it would break.
      api = ApiClient(
        httpClient: MockClient(
          (r) async => http.Response(
            '[{"id":17,"name":"Ravi","email":"r@x.test","role":"student","coin_balance":5}]',
            200,
          ),
        ),
      );
      expect((await api.fetchUsers()).single.role, 'student');
    });
  });
}
