#!/bin/bash
echo "Restarting Convertly server..."
pkill -f "go run main.go" 2>/dev/null
sleep 2
cd /home/guru/workspace/github.com/SyntaxSamurai/convertly
PORT=3000 go run main.go > /tmp/convertly-server.log 2>&1 &
sleep 3
echo "Server restarted!"
echo "Visit http://localhost:3000"
