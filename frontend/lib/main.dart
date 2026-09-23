import 'package:flutter/material.dart';

void main() {
  runApp(const SmartCanteenApp());
}

class SmartCanteenApp extends StatelessWidget {
  const SmartCanteenApp({super.key});

  @override
  Widget build(BuildContext context) {
    return const MaterialApp(
      title: 'Smart Canteen',
      home: Scaffold(
        body: Center(child: Text('Hello World')),
      ),
    );
  }
}