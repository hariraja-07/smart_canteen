import 'package:flutter/material.dart';

/// A future rendered as a widget, with the loading, error, and retry handling
/// written once instead of copied into every page.
///
/// Retry re-runs the loader rather than caching the failed future, so a second
/// attempt is a real request and not the same error shown again.
class AsyncView<T> extends StatefulWidget {
  final Future<T> Function() load;
  final Widget Function(BuildContext context, T data) builder;
  final String? emptyMessage;
  final bool Function(T data)? isEmpty;

  /// Adds pull-to-refresh, which only makes sense for a list the user watches
  /// change underneath them.
  final bool refreshable;

  const AsyncView({
    super.key,
    required this.load,
    required this.builder,
    this.emptyMessage,
    this.isEmpty,
    this.refreshable = false,
  });

  @override
  State<AsyncView<T>> createState() => _AsyncViewState<T>();
}

class _AsyncViewState<T> extends State<AsyncView<T>> {
  late Future<T> _future;

  @override
  void initState() {
    super.initState();
    _future = widget.load();
  }

  void _reload() {
    // The block body matters: `setState(() => _future = ...)` returns the
    // Future it just assigned, and setState rejects a callback that returns
    // one because it assumes async work is happening inside the rebuild.
    setState(() {
      _future = widget.load();
    });
  }

  @override
  Widget build(BuildContext context) {
    return FutureBuilder<T>(
      future: _future,
      builder: (context, snapshot) {
        if (snapshot.connectionState != ConnectionState.done) {
          return const Center(child: CircularProgressIndicator());
        }
        if (snapshot.hasError) {
          return _Message(
            text: 'Error: ${snapshot.error}',
            action: FilledButton(
              onPressed: _reload,
              child: const Text('Retry'),
            ),
          );
        }
        final data = snapshot.data as T;
        if (widget.isEmpty?.call(data) ?? false) {
          return _Message(text: widget.emptyMessage ?? 'Nothing here yet');
        }
        final built = widget.builder(context, data);
        if (!widget.refreshable) return built;
        return RefreshIndicator(
          onRefresh: () async {
            setState(() {
              _future = widget.load();
            });
            await _future;
          },
          child: built,
        );
      },
    );
  }
}

class _Message extends StatelessWidget {
  final String text;
  final Widget? action;

  const _Message({required this.text, this.action});

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(text, textAlign: TextAlign.center),
            if (action != null) ...[const SizedBox(height: 12), action!],
          ],
        ),
      ),
    );
  }
}

/// The server sends RFC 3339 timestamps. Formatting them properly wants a
/// package and a locale, neither of which earns its keep for a canteen queue,
/// so this keeps just the date.
String shortDate(String iso) {
  if (iso.length < 10) return iso;
  return iso.substring(0, 10);
}
