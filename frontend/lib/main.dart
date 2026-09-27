import 'package:flutter/material.dart';

import 'login_page.dart';
import 'session.dart';
import 'shell.dart';

void main() {
  runApp(SmartCanteenApp(session: Session()));
}

class SmartCanteenApp extends StatelessWidget {
  final Session session;

  /// The session is injectable so a test can start signed in, or signed out,
  /// without going through a network round trip to find out.
  const SmartCanteenApp({super.key, required this.session});

  @override
  Widget build(BuildContext context) {
    return SessionScope(
      session: session,
      child: MaterialApp(
        title: 'Smart Canteen',
        theme: ThemeData(
          colorSchemeSeed: const Color(0xFF2E7D32),
          useMaterial3: true,
        ),
        // Rebuilding on the session means signing out swaps the whole app back
        // to the login screen without anyone having to remember to navigate
        // there, which is how a signed-out user ends up staring at a stale
        // dashboard.
        home: ListenableBuilder(
          listenable: session,
          builder: (context, _) =>
              session.isSignedIn ? const Shell() : const LoginPage(),
        ),
      ),
    );
  }
}
