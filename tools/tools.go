package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/michelgermain/gnucash-mcp/internal/gnucash"
)

// RegisterTools adds all GnuCash MCP tools to the server.
func RegisterTools(s *server.MCPServer, svc *gnucash.Service) {
	registerListAccounts(s, svc)
	registerGetBalance(s, svc)
	registerGetTransactions(s, svc)
	registerSpendingByCategory(s, svc)
	registerIncomeVsExpenses(s, svc)
	registerSearchTransactions(s, svc)
}

func registerListAccounts(s *server.MCPServer, svc *gnucash.Service) {
	tool := mcp.NewTool("list_accounts",
		mcp.WithDescription("List all accounts with their hierarchy and types. Returns a tree structure of the chart of accounts."),
		mcp.WithString("account_type",
			mcp.Description("Filter by account type: ASSET, BANK, CASH, CREDIT, EQUITY, EXPENSE, INCOME, LIABILITY"),
		),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		accountType := mcp.ParseString(request, "account_type", "")
		result, err := svc.ListAccounts(ctx, accountType)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	})
}

func registerGetBalance(s *server.MCPServer, svc *gnucash.Service) {
	tool := mcp.NewTool("get_balance",
		mcp.WithDescription("Get the current balance for a specific account. Returns the sum of all transactions up to the given date."),
		mcp.WithString("account_name",
			mcp.Required(),
			mcp.Description("Account name (case-insensitive, partial match supported)"),
		),
		mcp.WithString("date",
			mcp.Description("Balance as of this date (YYYY-MM-DD). Defaults to today."),
		),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := request.RequireString("account_name")
		if err != nil {
			return mcp.NewToolResultError("account_name is required"), nil
		}
		date := mcp.ParseString(request, "date", "")
		result, err := svc.GetBalance(ctx, name, date)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	})
}

func registerGetTransactions(s *server.MCPServer, svc *gnucash.Service) {
	tool := mcp.NewTool("get_transactions",
		mcp.WithDescription("Retrieve transactions for an account within a date range. Shows date, amount, description, and counterpart account for each transaction."),
		mcp.WithString("account_name",
			mcp.Required(),
			mcp.Description("Account name (case-insensitive, partial match supported)"),
		),
		mcp.WithString("start_date",
			mcp.Description("Start date (YYYY-MM-DD)"),
		),
		mcp.WithString("end_date",
			mcp.Description("End date (YYYY-MM-DD)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of transactions to return (default: 50)"),
		),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := request.RequireString("account_name")
		if err != nil {
			return mcp.NewToolResultError("account_name is required"), nil
		}
		startDate := mcp.ParseString(request, "start_date", "")
		endDate := mcp.ParseString(request, "end_date", "")
		limit := mcp.ParseInt(request, "limit", 50)
		result, err := svc.GetTransactions(ctx, name, startDate, endDate, limit)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	})
}

func registerSpendingByCategory(s *server.MCPServer, svc *gnucash.Service) {
	tool := mcp.NewTool("spending_by_category",
		mcp.WithDescription("Aggregate expenses by category (expense accounts). Shows total amount and transaction count per category, sorted by highest spending."),
		mcp.WithString("start_date",
			mcp.Description("Start date (YYYY-MM-DD). Defaults to start of current month."),
		),
		mcp.WithString("end_date",
			mcp.Description("End date (YYYY-MM-DD). Defaults to today."),
		),
		mcp.WithString("parent_account",
			mcp.Description("Filter by parent expense account name"),
		),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		startDate := mcp.ParseString(request, "start_date", "")
		endDate := mcp.ParseString(request, "end_date", "")
		parentAccount := mcp.ParseString(request, "parent_account", "")
		result, err := svc.SpendingByCategory(ctx, startDate, endDate, parentAccount)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	})
}

func registerIncomeVsExpenses(s *server.MCPServer, svc *gnucash.Service) {
	tool := mcp.NewTool("income_vs_expenses",
		mcp.WithDescription("Monthly comparison of income and expenses. Shows per-month breakdown with income total, expense total, and net amount."),
		mcp.WithNumber("months",
			mcp.Description("Number of months to include (default: 6)"),
		),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		months := mcp.ParseInt(request, "months", 6)
		result, err := svc.IncomeVsExpenses(ctx, months)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	})
}

func registerSearchTransactions(s *server.MCPServer, svc *gnucash.Service) {
	tool := mcp.NewTool("search_transactions",
		mcp.WithDescription("Full-text search in transaction descriptions and split memos. Returns matching transactions with all their splits."),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Search term to match against transaction descriptions and memos"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of results (default: 20)"),
		),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := request.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError("query is required"), nil
		}
		limit := mcp.ParseInt(request, "limit", 20)
		result, err := svc.SearchTransactions(ctx, query, limit)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	})
}
