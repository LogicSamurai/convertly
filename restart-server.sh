#!/bin/bash
# Restart the Convertly server with updated code

echo "Stopping existing server..."
pkill -f "go run main.go" || true
sleep 2

echo "Starting server..."
cd /home/guru/workspace/github.com/SyntaxSamurai/convertly
PORT=3000 go run main.go > /tmp/convertly-server.log 2>&1 &

echo "Waiting for server to start..."
sleep 3

echo "Testing server..."
curl -s http://localhost:3000/ping

echo ""
echo "Server restarted successfully!"
echo "Visit http://localhost:3000 to test"
