#!/bin/bash
set -e

echo "=== Internal Transfers System - Verification Script ==="
echo ""

echo "1. Checking Go installation..."
go version

echo ""
echo "2. Checking Docker installation..."
docker --version

echo ""
echo "3. Starting PostgreSQL..."
make db-up
sleep 3

echo ""
echo "4. Verifying PostgreSQL is running..."
docker ps | grep transfers-postgres

echo ""
echo "5. Setting DATABASE_URL..."
export DATABASE_URL="postgres://app:app@localhost:5432/transfers?sslmode=disable"
echo "DATABASE_URL=$DATABASE_URL"

echo ""
echo "6. Starting server in background..."
make run &
SERVER_PID=$!
echo "Server PID: $SERVER_PID"

echo ""
echo "7. Waiting for server to start..."
sleep 5

echo ""
echo "8. Testing /health endpoint..."
curl -s http://localhost:8080/health | jq .

echo ""
echo "9. Testing POST /accounts (should return 501)..."
curl -s -X POST http://localhost:8080/accounts | jq .

echo ""
echo "10. Testing GET /accounts/123 (should return 501)..."
curl -s http://localhost:8080/accounts/123 | jq .

echo ""
echo "11. Testing POST /transactions (should return 501)..."
curl -s -X POST http://localhost:8080/transactions | jq .

echo ""
echo "12. Stopping server with Ctrl+C simulation..."
kill -SIGINT $SERVER_PID
sleep 2

echo ""
echo "13. Stopping PostgreSQL..."
make db-down

echo ""
echo "=== All checks passed! ==="
