# GnuCash MCP Server

A [Model Context Protocol](https://modelcontextprotocol.io/) (MCP) server that provides read-only access to GnuCash financial data stored in SQLite format. This enables AI assistants like Claude to query and analyze your personal finance data directly.

## Features

- **Read-only access** — your financial data is never modified
- **6 tools** for exploring accounts, balances, transactions, and spending patterns
- **Pure Go** — no CGO required, single static binary
- **Stdio transport** — works with Claude Desktop and any MCP-compatible client

## Prerequisites

- Go 1.21+
- A GnuCash file saved in **SQLite format** (File → Save As → SQLite3)

## Build

```bash
git clone https://github.com/michelgermain/gnucash-mcp.git
cd gnucash-mcp
go build -o gnucash-mcp .
```

## Configuration

### Claude Desktop

Add to your Claude Desktop config (`~/.config/claude/claude_desktop_config.json` on macOS/Linux):

```json
{
  "mcpServers": {
    "gnucash": {
      "command": "/absolute/path/to/gnucash-mcp",
      "env": {
        "GNUCASH_FILE": "/path/to/your/file.gnucash"
      }
    }
  }
}
```

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `GNUCASH_FILE` | Yes | Absolute path to your GnuCash SQLite file |

## Tools

### `list_accounts`

List all accounts with their hierarchy and types.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `account_type` | string | No | Filter by type: `ASSET`, `BANK`, `CASH`, `CREDIT`, `EQUITY`, `EXPENSE`, `INCOME`, `LIABILITY` |

### `get_balance`

Get the current balance for a specific account.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `account_name` | string | Yes | Account name (case-insensitive, partial match) |
| `date` | string | No | Balance as of date (`YYYY-MM-DD`), defaults to today |

### `get_transactions`

Retrieve transactions for an account within a date range.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `account_name` | string | Yes | Account name (case-insensitive, partial match) |
| `start_date` | string | No | Start date (`YYYY-MM-DD`) |
| `end_date` | string | No | End date (`YYYY-MM-DD`) |
| `limit` | number | No | Max results (default: 50) |

### `spending_by_category`

Aggregate expenses by category, sorted by highest spending.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `start_date` | string | No | Start date (`YYYY-MM-DD`), defaults to start of current month |
| `end_date` | string | No | End date (`YYYY-MM-DD`), defaults to today |
| `parent_account` | string | No | Filter by parent expense account name |

### `income_vs_expenses`

Monthly comparison of income and expenses.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `months` | number | No | Number of months to include (default: 6) |

### `search_transactions`

Full-text search in transaction descriptions and split memos.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | Yes | Search term |
| `limit` | number | No | Max results (default: 20) |

## Project Structure

```
gnucash-mcp/
├── main.go                 # Entry point, MCP server setup
├── internal/
│   └── gnucash/
│       ├── models.go       # Data structures (Account, Transaction, Split)
│       ├── db.go           # SQLite connection and queries
│       └── service.go      # Business logic and formatting
└── tools/
    └── tools.go            # MCP tool definitions and handlers
```

## Example Queries

Once configured, you can ask Claude things like:

- *"What's my checking account balance?"*
- *"Show me my spending by category for last month"*
- *"Search for all transactions mentioning 'Amazon'"*
- *"Compare my income vs expenses over the last 6 months"*
- *"List all my expense accounts"*

## Security

- The database is opened in **read-only mode** (`?mode=ro`) at the SQLite driver level
- No write operations are implemented
- The file path is provided via environment variable and never logged

## License

MIT
