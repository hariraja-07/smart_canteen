#!/usr/bin/env bash
# Start/stop the backend detached for local testing.
#   ./dev-server.sh start [binary]
#   ./dev-server.sh stop
set -u

BIN="${2:-/tmp/sc-backend3}"
PIDFILE=/tmp/sc-backend.pid
LOG=/tmp/sc.log

export DATABASE_URL="${DATABASE_URL:-postgres://smartcanteen:smartcanteen_dev@localhost:5432/smart_canteen}"
export JWT_SECRET="${JWT_SECRET:-dev-secret-at-least-32-characters-long}"

case "${1:-start}" in
  start)
    if [ -f "$PIDFILE" ] && kill -0 "$(cat $PIDFILE)" 2>/dev/null; then
      echo "already running pid $(cat $PIDFILE)"; exit 0
    fi
    setsid "$BIN" >"$LOG" 2>&1 </dev/null &
    echo $! > "$PIDFILE"
    for _ in $(seq 1 25); do
      if curl -fsS --max-time 2 http://localhost:8080/health >/dev/null 2>&1; then
        echo "started pid $(cat $PIDFILE)"; exit 0
      fi
      sleep 0.4
    done
    echo "failed to become healthy; last log lines:"; tail -5 "$LOG"; exit 1
    ;;
  stop)
    if [ -f "$PIDFILE" ]; then kill "$(cat $PIDFILE)" 2>/dev/null; rm -f "$PIDFILE"; echo stopped
    else echo "no pidfile"; fi
    ;;
  *) echo "usage: $0 {start|stop} [binary]"; exit 2 ;;
esac
