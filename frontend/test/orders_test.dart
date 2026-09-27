import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:frontend/core/api.dart';
import 'package:frontend/app.dart';
import 'package:frontend/core/models.dart';
import 'package:frontend/core/session.dart';

Session signedIn({String role = Role.student, int balance = 110}) {
  final s = Session(api: api);
  api.token = 'jwt';
  s.debugSetUser(
    User(
      id: 17,
      name: 'Ravi',
      email: 'ravi@x.test',
      role: role,
      coinBalance: balance,
    ),
  );
  return s;
}

/// Routes by path so one mock can serve the whole app.
MockClient routes(Map<String, Object> byPath) {
  return MockClient((r) async {
    final body = byPath[r.url.path];
    if (body == null) return http.Response('{"error":"no such route"}', 404);
    return http.Response(body is String ? body : jsonEncode(body), 200);
  });
}

Map<String, Object> order({
  int id = 5,
  int total = 70,
  String status = OrderStatus.pending,
  String customer = 'Ravi',
}) => {
  'id': id,
  'user_id': 17,
  'customer': customer,
  'total': total,
  'status': status,
  'items': [
    {
      'menu_item_id': 34,
      'name': 'Masala Chai',
      'qty': 2,
      'unit_price': 10,
      'line_total': 20,
    },
    {
      'menu_item_id': 35,
      'name': 'Masala Dosa',
      'qty': 1,
      'unit_price': 50,
      'line_total': 50,
    },
  ],
  'created_at': '2026-03-04T10:15:00Z',
  'updated_at': '2026-03-04T10:15:00Z',
};

/// The client the app under test talks through. Each test assigns the
/// transport it needs before pumping, so nothing is shared between them
/// and no tearDown is left over to undo one test leaking into the next.
late ApiClient api;

void main() {
  setUp(() => api = ApiClient());

  group('orders tab', () {
    testWidgets('lists orders with totals and status', (tester) async {
      api = ApiClient(
        httpClient: routes({
          '/api/menu': <Object>[],
          '/api/orders': [order()],
        }),
      );
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));

      await tester.tap(find.text('Orders'));
      await tester.pumpAndSettle();

      expect(find.text('Order #5'), findsOneWidget);
      expect(find.text('70 coins'), findsOneWidget);
      expect(find.text('Pending'), findsOneWidget);
      expect(find.text('2 x Masala Chai  20'), findsOneWidget);
      expect(find.text('2026-03-04'), findsOneWidget);
    });

    testWidgets('a student does not see the customer column', (tester) async {
      // A student only ever receives their own orders, so their own name on the
      // card is noise. The server enforces the scoping; this is only the UI
      // agreeing with it.
      api = ApiClient(
        httpClient: routes({
          '/api/menu': <Object>[],
          '/api/orders': [order()],
        }),
      );
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));
      await tester.tap(find.text('Orders'));
      await tester.pumpAndSettle();

      expect(find.text('Ravi'), findsNothing);
    });

    testWidgets('the kitchen sees whose order it is', (tester) async {
      api = ApiClient(
        httpClient: routes({
          '/api/menu': <Object>[],
          '/api/orders': [order(customer: 'Priya')],
        }),
      );
      await tester.pumpWidget(
        SmartCanteenApp(session: signedIn(role: Role.canteenManagement)),
      );
      await tester.tap(find.text('Orders'));
      await tester.pumpAndSettle();

      expect(find.text('Priya'), findsOneWidget);
    });

    testWidgets('an empty queue says so instead of showing nothing', (
      tester,
    ) async {
      api = ApiClient(
        httpClient: routes({
          '/api/menu': <Object>[],
          '/api/orders': <Object>[],
        }),
      );
      await tester.pumpWidget(
        SmartCanteenApp(session: signedIn(role: Role.canteenManagement)),
      );
      await tester.tap(find.text('Orders'));
      await tester.pumpAndSettle();

      expect(find.text('No orders in the queue'), findsOneWidget);
    });

    testWidgets('a student is told they have not ordered, in their own words', (
      tester,
    ) async {
      api = ApiClient(
        httpClient: routes({
          '/api/menu': <Object>[],
          '/api/orders': <Object>[],
        }),
      );
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));
      await tester.tap(find.text('Orders'));
      await tester.pumpAndSettle();

      expect(find.text('You have not ordered yet'), findsOneWidget);
    });

    testWidgets('a failure offers a retry that refetches', (tester) async {
      var calls = 0;
      api = ApiClient(
        httpClient: MockClient((r) async {
          if (r.url.path == '/api/menu') return http.Response('[]', 200);
          calls++;
          if (calls == 1) return http.Response('{"error":"boom"}', 500);
          return http.Response(jsonEncode([order()]), 200);
        }),
      );
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));
      await tester.tap(find.text('Orders'));
      await tester.pumpAndSettle();

      expect(find.textContaining('Error:'), findsOneWidget);
      await tester.tap(find.text('Retry'));
      await tester.pumpAndSettle();

      // A real second request, not the same failure replayed.
      expect(calls, 2);
      expect(find.text('Order #5'), findsOneWidget);
    });

    testWidgets('every request carries the token', (tester) async {
      final seen = <String?>[];
      api = ApiClient(
        httpClient: MockClient((r) async {
          seen.add(r.headers['Authorization']);
          if (r.url.path == '/api/menu') return http.Response('[]', 200);
          return http.Response('[]', 200);
        }),
      );
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));
      await tester.tap(find.text('Orders'));
      await tester.pumpAndSettle();

      // The menu is public, but the orders call is not, and it must be signed.
      expect(seen.contains('Bearer jwt'), isTrue);
    });
  });

  group('coins tab', () {
    final history = [
      {
        'id': 12,
        'amount': -70,
        'kind': 'order_payment',
        'reason': 'order #5',
        'actor_id': null,
        'order_id': 5,
        'created_at': '2026-03-04T10:15:00Z',
      },
      {
        'id': 11,
        'amount': 100,
        'kind': 'exchange_in',
        'reason': 'cash at counter',
        'actor_id': 15,
        'order_id': null,
        'created_at': '2026-03-01T09:00:00Z',
      },
    ];

    testWidgets('shows each movement with its sign', (tester) async {
      api = ApiClient(
        httpClient: routes({
          '/api/menu': <Object>[],
          '/api/users/17/coins': history,
        }),
      );
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));

      await tester.tap(find.text('Coins'));
      await tester.pumpAndSettle();

      expect(find.text('Coins bought'), findsOneWidget);
      expect(find.text('Order paid'), findsOneWidget);
      expect(find.text('+100'), findsOneWidget);
      expect(find.text('-70'), findsOneWidget);
    });

    testWidgets('reads the signed-in user history, not a fixed id', (
      tester,
    ) async {
      // A hardcoded id here would show one user another user's ledger.
      final paths = <String>[];
      api = ApiClient(
        httpClient: MockClient((r) async {
          paths.add(r.url.path);
          if (r.url.path == '/api/menu') return http.Response('[]', 200);
          return http.Response('[]', 200);
        }),
      );
      final s = Session(api: api);
      api.token = 'jwt';
      s.debugSetUser(
        const User(
          id: 99,
          name: 'Priya',
          email: 'p@x.test',
          role: Role.student,
          coinBalance: 60,
        ),
      );
      await tester.pumpWidget(SmartCanteenApp(session: s));
      await tester.tap(find.text('Coins'));
      await tester.pumpAndSettle();

      expect(paths, contains('/api/users/99/coins'));
      expect(paths, isNot(contains('/api/users/17/coins')));
    });

    testWidgets('no activity yet reads as an empty history', (tester) async {
      api = ApiClient(
        httpClient: routes({
          '/api/menu': <Object>[],
          '/api/users/17/coins': <Object>[],
        }),
      );
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));
      await tester.tap(find.text('Coins'));
      await tester.pumpAndSettle();

      expect(find.text('No coin activity yet'), findsOneWidget);
    });
  });

  group('shell', () {
    testWidgets('the app bar balance is the signed-in balance', (tester) async {
      api = ApiClient(httpClient: routes({'/api/menu': <Object>[]}));
      await tester.pumpWidget(SmartCanteenApp(session: signedIn(balance: 110)));
      await tester.pumpAndSettle();

      expect(find.text('110 coins'), findsOneWidget);
    });

    testWidgets('three tabs are offered and each renders', (tester) async {
      api = ApiClient(
        httpClient: routes({
          '/api/menu': <Object>[],
          '/api/orders': <Object>[],
          '/api/users/17/coins': <Object>[],
        }),
      );
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));
      await tester.pumpAndSettle();

      expect(find.byType(NavigationBar), findsOneWidget);
      for (final label in ['Menu', 'Orders', 'Coins']) {
        expect(find.text(label), findsWidgets, reason: 'missing $label tab');
      }
    });
  });
}
