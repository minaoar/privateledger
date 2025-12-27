# PrivateLedger

**Personal Expenditure Insight - Privacy-First Finance Tracking**

A local-only personal finance application that consolidates bank transactions from OFX files, providing categorization and spending insights while keeping all data on your machine.

## Features

- **Privacy-First**: All data stays local. No cloud, no APIs, no tracking.
- **OFX Import**: Import transactions from Canadian banks (TD, RBC, CIBC) via OFX files
- **Smart Categorization**: Pattern-based auto-categorization with manual override support
- **Insights Dashboard**: Monthly spending analysis and trend visualization
- **Multi-Account Support**: Track multiple bank accounts and credit cards
- **Single Binary**: No dependencies, no installation required

## Quick Start

### Prerequisites

- Go 1.23 or later (for building from source)

### Building

```bash
# Clone the repository
git clone <repository-url>
cd PrivateLedger

# Download dependencies
go mod tidy

# Build the binary
make build

# Or build for all platforms
make build-all
```

### Running

```bash
# Run directly
./privateledger

# Or use make
make run
```

On first run, the application will:
1. Create `config.json` with default settings
2. Create `privateledger.db` SQLite database
3. Open your browser to `http://localhost:8080`

## Configuration

Edit `config.json` to customize settings:

```json
{
  "version": 1,
  "server": {
    "port": 8080,
    "auto_open_browser": true
  },
  "start_of_month": 1
}
```

- `port`: HTTP server port
- `auto_open_browser`: Automatically open browser on startup
- `start_of_month`: Day of month when your "month" starts (1-28, useful for aligning with pay cycles)

## Usage

1. **Create Accounts**: Add your bank accounts and credit cards
2. **Set Up Categories**: Define spending categories and matching patterns
3. **Import Transactions**: Upload OFX files from your bank
4. **Review & Categorize**: Review auto-categorized transactions, adjust as needed
5. **Analyze Spending**: View insights and trends on the dashboard

## Project Structure

See [DESIGN.md](DESIGN.md) for detailed architecture and design decisions.

## Development

```bash
# Run tests
make test

# Clean build artifacts
make clean
```

## Tech Stack

- **Go** - Backend and CLI
- **Gin** - Web framework
- **SQLite** (modernc.org/sqlite) - Database (pure Go, no CGO)
- **ofxgo** - OFX file parser
- **Bootstrap 5 + HTMX** - Frontend

## License

[Add your license here]

## Contributing

[Add contribution guidelines here]
