import 'package:flutter/material.dart';

import 'core/cart.dart';
import 'core/session.dart';
import 'features/auth/login_page.dart';
import 'features/shell.dart';

class SmartCanteenApp extends StatefulWidget {
  final Session session;

  /// Optional so a test that does not care about the cart need not build one.
  /// When supplied it is used as-is, which is how a test starts with a
  /// pre-filled cart.
  final Cart? cart;

  const SmartCanteenApp({super.key, required this.session, this.cart});

  @override
  State<SmartCanteenApp> createState() => _SmartCanteenAppState();
}

class _SmartCanteenAppState extends State<SmartCanteenApp> {
  /// Built once and kept, because a cart created inside build would be a new
  /// empty cart on every rebuild and the user's items would vanish under them.
  late final Cart _cart = widget.cart ?? Cart();

  @override
  Widget build(BuildContext context) {
    return SessionScope(
      session: widget.session,
      child: CartScope(
        cart: _cart,
        child: MaterialApp(
          title: 'Smart Canteen',
          theme: ThemeData(
            colorSchemeSeed: const Color(0xFF2E7D32),
            useMaterial3: true,
          ),
          // Rebuilding on the session means signing out swaps the whole app
          // back to the login screen without anyone having to remember to
          // navigate there, which is how a signed-out user ends up staring at
          // a stale dashboard.
          home: ListenableBuilder(
            listenable: widget.session,
            builder: (context, _) =>
                widget.session.isSignedIn ? const Shell() : const LoginPage(),
          ),
        ),
      ),
    );
  }
}
