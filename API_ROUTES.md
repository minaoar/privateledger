# PrivateLedger API Routes

Complete API documentation for PrivateLedger endpoints.

## Base URL

```
http://localhost:8080
```

---

## Root

### GET /
Get API information and available endpoints.

**Response:**
```json
{
  "name": "PrivateLedger API",
  "version": "0.1.0",
  "endpoints": {
    "accounts": "/api/accounts",
    "transactions": "/api/transactions",
    "categories": "/api/categories",
    "import": "/api/import",
    "insights": "/api/insights"
  }
}
```

---

## Accounts

### GET /api/accounts
List all accounts.

**Response:**
```json
[
  {
    "account_id": 1,
    "name": "TD Credit Card",
    "created_at": "2025-01-15T10:30:00Z"
  }
]
```

### GET /api/accounts/:id
Get a single account by ID.

**Response:**
```json
{
  "account_id": 1,
  "name": "TD Credit Card",
  "created_at": "2025-01-15T10:30:00Z"
}
```

### POST /api/accounts
Create a new account.

**Request:**
```json
{
  "name": "RBC Chequing"
}
```

**Response:** `201 Created`
```json
{
  "account_id": 2,
  "name": "RBC Chequing",
  "created_at": "2025-01-15T10:35:00Z"
}
```

### PUT /api/accounts/:id
Update an account's name.

**Request:**
```json
{
  "name": "RBC Chequing Updated"
}
```

**Response:**
```json
{
  "account_id": 2,
  "name": "RBC Chequing Updated",
  "created_at": "2025-01-15T10:35:00Z"
}
```

### DELETE /api/accounts/:id
Delete an account (cascades to transactions).

**Response:**
```json
{
  "message": "Account deleted successfully"
}
```

---

## Transactions

### GET /api/transactions
List transactions with optional filters.

**Query Parameters:**
- `account_id` (int) - Filter by account
- `category_id` (int) - Filter by category
- `uncategorized` (bool) - Only uncategorized (true/false)
- `start_date` (string) - Date range start (YYYY-MM-DD)
- `end_date` (string) - Date range end (YYYY-MM-DD)
- `limit` (int) - Max results
- `offset` (int) - Pagination offset

**Example:**
```
GET /api/transactions?account_id=1&uncategorized=true&limit=50
```

**Response:**
```json
[
  {
    "transaction_id": 1,
    "account_id": 1,
    "trn_type": "DEBIT",
    "fit_id": "25326171322410010",
    "date_posted": "2025-01-15T00:00:00Z",
    "amount": -45.99,
    "transaction_details": "STARBUCKS - DOWNTOWN",
    "transaction_type": 1,
    "category_id": 3,
    "category_source": 1,
    "created_at": "2025-01-15T10:40:00Z"
  }
]
```

### GET /api/transactions/:id
Get a single transaction by ID.

**Response:** Same as list item above.

### PATCH /api/transactions/:id/category
Update a transaction's category (manual assignment).

**Request:**
```json
{
  "category_id": 5
}
```

Or to uncategorize:
```json
{
  "category_id": null
}
```

**Response:** Updated transaction object.

### DELETE /api/transactions/:id
Delete a transaction.

**Response:**
```json
{
  "message": "Transaction deleted successfully"
}
```

---

## Categories

### GET /api/categories
List all categories with their patterns.

**Response:**
```json
[
  {
    "category_id": 1,
    "name": "Groceries",
    "color": "#4CAF50",
    "created_at": "2025-01-15T09:00:00Z",
    "patterns": [
      {
        "category_pattern_id": 1,
        "pattern_name": "WALMART",
        "category_id": 1,
        "created_at": "2025-01-15T09:00:00Z"
      },
      {
        "category_pattern_id": 2,
        "pattern_name": "LOBLAWS",
        "category_id": 1,
        "created_at": "2025-01-15T09:01:00Z"
      }
    ]
  }
]
```

### GET /api/categories/:id
Get a single category with patterns.

**Response:** Same as list item above.

### POST /api/categories
Create a new category with optional initial patterns.

**Request:**
```json
{
  "name": "Dining",
  "color": "#FF5722",
  "patterns": ["RESTAURANT", "STARBUCKS", "TIM HORTONS"]
}
```

**Response:** `201 Created` - Category with patterns object.

### PUT /api/categories/:id
Update a category's name and/or color.

**Request:**
```json
{
  "name": "Restaurants",
  "color": "#E91E63"
}
```

**Response:** Updated category object.

### DELETE /api/categories/:id
Delete a category (cascades to patterns, clears from transactions).

**Response:**
```json
{
  "message": "Category deleted successfully"
}
```

### POST /api/categories/recategorize
Manually trigger re-categorization of all uncategorized transactions.

**Response:**
```json
{
  "processed_count": 150,
  "categorized_count": 87
}
```

---

## Patterns

### POST /api/categories/:id/patterns
Add a new pattern to a category.

**Request:**
```json
{
  "pattern_name": "MCDONALD'S"
}
```

**Response:** `201 Created`
```json
{
  "category_pattern_id": 10,
  "pattern_name": "MCDONALD'S",
  "category_id": 3,
  "created_at": "2025-01-15T11:00:00Z"
}
```

### DELETE /api/patterns/:id
Delete a pattern.

**Response:**
```json
{
  "message": "Pattern deleted successfully"
}
```

---

## Import

### POST /api/import
Import transactions from an OFX file.

**Content-Type:** `multipart/form-data`

**Form Fields:**
- `file` (file) - OFX file
- `account_id` (int) - Target account ID

**Example (curl):**
```bash
curl -X POST http://localhost:8080/api/import \
  -F "file=@statement.ofx" \
  -F "account_id=1"
```

**Response:**
```json
{
  "total_transactions": 120,
  "imported_count": 95,
  "duplicate_count": 25,
  "categorized_count": 68,
  "error_count": 0,
  "errors": [],
  "transactions": [...]
}
```

### POST /api/import/validate
Validate an OFX file without importing.

**Content-Type:** `multipart/form-data`

**Form Fields:**
- `file` (file) - OFX file

**Response:**
```json
{
  "valid": true,
  "message": "OFX file is valid"
}
```

---

## Insights

### GET /api/insights/dashboard
Get complete dashboard statistics.

**Response:**
```json
{
  "current_month": {
    "period": {
      "label": "2025-01",
      "start_date": "2025-01-19T00:00:00Z",
      "end_date": "2025-02-18T23:59:59Z"
    },
    "total_income": 3500.00,
    "total_expenses": 2145.67,
    "net_amount": 1354.33,
    "transaction_count": 89,
    "uncategorized_count": 12,
    "category_breakdown": [
      {
        "category_id": 1,
        "category_name": "Groceries",
        "category_color": "#4CAF50",
        "total_amount": 456.78,
        "count": 18
      }
    ]
  },
  "account_count": 3,
  "category_count": 8,
  "uncategorized_count": 45
}
```

### GET /api/insights/monthly
Get financial summary for a specific month.

**Query Parameters:**
- `year` (int) - Year (e.g., 2025)
- `month` (int) - Month 1-12 (e.g., 1 for January)

If no parameters provided, returns current month.

**Example:**
```
GET /api/insights/monthly?year=2025&month=1
```

**Response:** Same as `current_month` in dashboard response.

### GET /api/insights/trends
Get financial trends over multiple months.

**Query Parameters:**
- `months` (int) - Number of months to include (default: 6)

**Example:**
```
GET /api/insights/trends?months=6
```

**Response:**
```json
[
  {
    "period": {
      "label": "2024-08",
      "start_date": "2024-08-19T00:00:00Z",
      "end_date": "2024-09-18T23:59:59Z"
    },
    "total_income": 3200.00,
    "total_expenses": 2100.00,
    "net_amount": 1100.00
  },
  {
    "period": {
      "label": "2024-09",
      "start_date": "2024-09-19T00:00:00Z",
      "end_date": "2024-10-18T23:59:59Z"
    },
    "total_income": 3500.00,
    "total_expenses": 2300.00,
    "net_amount": 1200.00
  }
]
```

### GET /api/insights/current-period
Get the current month period based on config.

**Response:**
```json
{
  "label": "2025-01",
  "start_date": "2025-01-19T00:00:00Z",
  "end_date": "2025-02-18T23:59:59Z"
}
```

---

## Error Responses

All endpoints return consistent error responses:

**400 Bad Request:**
```json
{
  "error": "Invalid account_id"
}
```

**404 Not Found:**
```json
{
  "error": "Account not found"
}
```

**409 Conflict:**
```json
{
  "error": "Account with this name already exists"
}
```

**500 Internal Server Error:**
```json
{
  "error": "Failed to query database: ..."
}
```

---

## Notes

- All timestamps are in UTC
- Date filters use YYYY-MM-DD format
- Custom month boundaries respect `start_of_month` config setting
- Category source values: 0=none, 1=rule, 2=manual
- Transaction type values: 1=debit, 2=credit
- Background re-categorization is triggered automatically when patterns are added
