# PrivateLedger

**Privacy-First Personal Expense Tracker**

PrivateLedger is a **local-only personal finance application** that helps you understand your monthly spending across multiple bank accounts and credit cards—**without giving any third-party access to your financial data**.

Many personal finance apps require direct access to your bank accounts or uploading statements to external servers. For privacy-conscious users, this creates an uncomfortable trade-off between convenience and control. PrivateLedger takes a different approach.

Instead of connecting to your bank, PrivateLedger works entirely with **transaction files you download yourself**. Most banks in the **US, Canada, and the UK** allow exporting transaction history in **OFX (Microsoft Money)** or **QFX (Intuit Quicken)** formats. PrivateLedger imports these files locally, consolidates transactions across accounts, and generates meaningful spending insights—while keeping **all data on your own machine**.

No cloud.  
No bank credentials.  
No data leaving your system.

PrivateLedger is designed for users who want **full ownership of their financial data**, transparency in how categorization works, and the flexibility to extend or audit the code themselves.

---

## Features

- **Privacy-First by Design**  
  All data stays local. No cloud storage, no APIs, no telemetry.

- **OFX / QFX Import**  
  Seamlessly import transaction history from multiple banks and credit card providers using standard OFX/QFX files, which are supported by the majority of banks in the US, Canada, and the UK.

- **Multi-Account Consolidation**  
  Combine transactions from multiple chequing accounts, savings accounts, and credit cards.

- **Smart Categorization**  
  Automatically categorizes transactions using pattern-based logic, with the flexibility for manual overrides and custom rules.

- **Insights Dashboard**  
  Monthly spending breakdowns, category trends, and expenditure analysis.

- **Local Web Application**  
  Runs entirely on your machine via a local web UI.

- **Simple Distribution**  
  Single binary, no external dependencies, no installation hassles.

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

## Project Architecture & Design Decisions

This project has been developed heavily with Claude Code.  
For architectural decisions, trade-offs, and design rationale, see: [CLAUDE.md](CLAUDE.md#architecture)

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
- **ofxgo** - OFX/QFX file parser
- **Bootstrap 5 + HTMX** - Frontend

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for full license text.

## Contributing

Contributions are welcome and appreciated 🎉

To keep development focused and aligned with the project goals:

- Please open an issue first to discuss:
  - New features
  - Significant behavior changes
  - Architectural or data model changes
- For small fixes (typos, minor UI tweaks, bug fixes), feel free to open a pull request directly.

When submitting a pull request, please keep changes focused and well-scoped. Include context in the description (what & why).

If you're unsure whether something fits, opening an issue to discuss it first is always the best approach.
