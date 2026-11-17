#!/bin/sh
# Demo server for UI tests: tools/demo.sh start|stop  (127.0.0.1:8765, token "test")
cd "$(dirname "$0")/.."
case "$1" in
start)
  go build -o /tmp/larik-demo-bin . || exit 1
  rm -rf /tmp/larik-demo-data; mkdir -p /tmp/larik-demo-data
  LARIK_DATA=/tmp/larik-demo-data nohup /tmp/larik-demo-bin --port 8765 --token test --stay > /tmp/larik-demo.log 2>&1 &
  echo $! > /tmp/larik-demo.pid
  for i in $(seq 1 50); do curl -s -o /dev/null http://127.0.0.1:8765/api/health && break; sleep 0.1; done
  echo started ;;
stop)
  [ -f /tmp/larik-demo.pid ] && kill "$(cat /tmp/larik-demo.pid)" 2>/dev/null; rm -f /tmp/larik-demo.pid; echo stopped ;;
esac
