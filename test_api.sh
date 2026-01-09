#!/bin/bash
# Simple API test script for PrivateLedger

BASE_URL="http://localhost:8844"

echo "======================================"
echo "PrivateLedger API Test Script"
echo "======================================"
echo ""

# Test 1: Root endpoint
echo "1. Testing root endpoint..."
curl -s "${BASE_URL}/" | jq '.'
echo ""

# Test 2: Create an account
echo "2. Creating a test account..."
ACCOUNT_RESPONSE=$(curl -s -X POST "${BASE_URL}/api/accounts" \
  -H "Content-Type: application/json" \
  -d '{"name":"Test Account"}')
echo "$ACCOUNT_RESPONSE" | jq '.'
ACCOUNT_ID=$(echo "$ACCOUNT_RESPONSE" | jq -r '.account_id')
echo "Created account ID: $ACCOUNT_ID"
echo ""

# Test 3: List accounts
echo "3. Listing all accounts..."
curl -s "${BASE_URL}/api/accounts" | jq '.'
echo ""

# Test 4: Create a category
echo "4. Creating a test category with patterns..."
CATEGORY_RESPONSE=$(curl -s -X POST "${BASE_URL}/api/categories" \
  -H "Content-Type: application/json" \
  -d '{"name":"Groceries","color":"#4CAF50","patterns":["WALMART","LOBLAWS","SOBEYS"]}')
echo "$CATEGORY_RESPONSE" | jq '.'
CATEGORY_ID=$(echo "$CATEGORY_RESPONSE" | jq -r '.category_id')
echo "Created category ID: $CATEGORY_ID"
echo ""

# Test 5: List categories
echo "5. Listing all categories with patterns..."
curl -s "${BASE_URL}/api/categories" | jq '.'
echo ""

# Test 6: Get dashboard stats
echo "6. Getting dashboard stats..."
curl -s "${BASE_URL}/api/insights/dashboard" | jq '.'
echo ""

# Test 7: Get current period
echo "7. Getting current period..."
curl -s "${BASE_URL}/api/insights/current-period" | jq '.'
echo ""

# Test 8: List transactions (should be empty)
echo "8. Listing transactions (should be empty)..."
curl -s "${BASE_URL}/api/transactions" | jq '.'
echo ""

echo "======================================"
echo "Tests completed!"
echo "======================================"
echo ""
echo "To import OFX files, use:"
echo "curl -X POST ${BASE_URL}/api/import \\"
echo "  -F \"file=@statement.ofx\" \\"
echo "  -F \"account_id=${ACCOUNT_ID}\""
echo ""
