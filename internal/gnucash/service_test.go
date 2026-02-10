package gnucash

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// setupTestDB creates an in-memory GnuCash database with seed data.
func setupTestDB(t *testing.T) *DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	schema := `
		CREATE TABLE accounts (
			guid TEXT PRIMARY KEY,
			name TEXT,
			account_type TEXT,
			parent_guid TEXT,
			description TEXT,
			commodity_guid TEXT,
			hidden INTEGER DEFAULT 0,
			placeholder INTEGER DEFAULT 0
		);
		CREATE TABLE transactions (
			guid TEXT PRIMARY KEY,
			currency_guid TEXT,
			post_date TEXT,
			enter_date TEXT,
			description TEXT
		);
		CREATE TABLE splits (
			guid TEXT PRIMARY KEY,
			tx_guid TEXT,
			account_guid TEXT,
			memo TEXT,
			value_num INTEGER,
			value_denom INTEGER,
			quantity_num INTEGER,
			quantity_denom INTEGER
		);

		-- Root account
		INSERT INTO accounts VALUES ('root', 'Root Account', 'ROOT', NULL, '', '', 0, 0);

		-- Top-level accounts
		INSERT INTO accounts VALUES ('assets',   'Assets',   'ASSET',   'root', '', '', 0, 0);
		INSERT INTO accounts VALUES ('expenses', 'Expenses', 'EXPENSE', 'root', '', '', 0, 0);
		INSERT INTO accounts VALUES ('income',   'Income',   'INCOME',  'root', '', '', 0, 0);

		-- Leaf accounts
		INSERT INTO accounts VALUES ('checking',   'Checking',   'BANK',    'assets',   'Main checking account', '', 0, 0);
		INSERT INTO accounts VALUES ('groceries',  'Groceries',  'EXPENSE', 'expenses', '', '', 0, 0);
		INSERT INTO accounts VALUES ('restaurant', 'Restaurant', 'EXPENSE', 'expenses', '', '', 0, 0);
		INSERT INTO accounts VALUES ('salary',     'Salary',     'INCOME',  'income',   '', '', 0, 0);

		-- Transaction 1: salary deposit of 3000.00 EUR on Jan 15
		INSERT INTO transactions VALUES ('tx1', 'eur', '2025-01-15 00:00:00', '2025-01-15 00:00:00', 'January salary');
		INSERT INTO splits VALUES ('sp1a', 'tx1', 'checking',  '', 300000, 100, 300000, 100);
		INSERT INTO splits VALUES ('sp1b', 'tx1', 'salary',    '', -300000, 100, -300000, 100);

		-- Transaction 2: groceries 85.50 EUR on Jan 20
		INSERT INTO transactions VALUES ('tx2', 'eur', '2025-01-20 00:00:00', '2025-01-20 00:00:00', 'Supermarket');
		INSERT INTO splits VALUES ('sp2a', 'tx2', 'checking',  '', -8550, 100, -8550, 100);
		INSERT INTO splits VALUES ('sp2b', 'tx2', 'groceries', '', 8550, 100, 8550, 100);

		-- Transaction 3: groceries 42.00 EUR on Feb 5
		INSERT INTO transactions VALUES ('tx3', 'eur', '2025-02-05 00:00:00', '2025-02-05 00:00:00', 'Market');
		INSERT INTO splits VALUES ('sp3a', 'tx3', 'checking',  '', -4200, 100, -4200, 100);
		INSERT INTO splits VALUES ('sp3b', 'tx3', 'groceries', '', 4200, 100, 4200, 100);

		-- Transaction 4: restaurant 25.00 EUR on Jan 25
		INSERT INTO transactions VALUES ('tx4', 'eur', '2025-01-25 00:00:00', '2025-01-25 00:00:00', 'Pizza place');
		INSERT INTO splits VALUES ('sp4a', 'tx4', 'checking',   '', -2500, 100, -2500, 100);
		INSERT INTO splits VALUES ('sp4b', 'tx4', 'restaurant', '', 2500, 100, 2500, 100);

		-- Transaction 5: salary deposit of 3000.00 EUR on Feb 15
		INSERT INTO transactions VALUES ('tx5', 'eur', '2025-02-15 00:00:00', '2025-02-15 00:00:00', 'February salary');
		INSERT INTO splits VALUES ('sp5a', 'tx5', 'checking',  '', 300000, 100, 300000, 100);
		INSERT INTO splits VALUES ('sp5b', 'tx5', 'salary',    '', -300000, 100, -300000, 100);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	return &DB{db: db}
}

func TestGetBalance(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	tests := []struct {
		name    string
		account string
		date    string
		wantSub string // substring expected in the result
	}{
		{
			// 3000 - 85.50 - 42 - 25 + 3000 = 5847.50
			name:    "checking balance all time",
			account: "Checking",
			date:    "",
			wantSub: "5847.50 EUR",
		},
		{
			// 3000 - 85.50 - 25 = 2889.50
			name:    "checking balance as of end of January",
			account: "Checking",
			date:    "2025-01-31",
			wantSub: "2889.50 EUR",
		},
		{
			name:    "groceries total",
			account: "Groceries",
			date:    "",
			wantSub: "127.50 EUR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := svc.GetBalance(ctx, tt.account, tt.date)
			if err != nil {
				t.Fatalf("GetBalance(%q, %q) returned error: %v", tt.account, tt.date, err)
			}
			if !strings.Contains(result, tt.wantSub) {
				t.Errorf("GetBalance(%q, %q) = %q, want substring %q", tt.account, tt.date, result, tt.wantSub)
			}
		})
	}
}

func TestGetBalance_AccountNotFound(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	_, err := svc.GetBalance(ctx, "Nonexistent", "")
	if err == nil {
		t.Fatal("expected error for nonexistent account, got nil")
	}
}

func TestGetBalance_AmbiguousAccount(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// "e" matches Expenses, Checking, Groceries, Salary, etc.
	_, err := svc.GetBalance(ctx, "e", "")
	if err == nil {
		t.Fatal("expected error for ambiguous account name, got nil")
	}
	if !strings.Contains(err.Error(), "multiple accounts match") {
		t.Errorf("expected 'multiple accounts match' error, got: %v", err)
	}
}

// --- ListAccounts ---

func TestListAccounts(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	result, err := svc.ListAccounts(ctx, "")
	if err != nil {
		t.Fatalf("ListAccounts() returned error: %v", err)
	}

	// Should contain all non-root accounts with full paths
	for _, want := range []string{"Assets:Checking", "Expenses:Groceries", "Expenses:Restaurant", "Income:Salary"} {
		if !strings.Contains(result, want) {
			t.Errorf("ListAccounts() missing %q in:\n%s", want, result)
		}
	}
}

func TestListAccounts_FilterByType(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	result, err := svc.ListAccounts(ctx, "EXPENSE")
	if err != nil {
		t.Fatalf("ListAccounts(EXPENSE) returned error: %v", err)
	}

	if !strings.Contains(result, "Groceries") {
		t.Errorf("expected Groceries in EXPENSE list, got:\n%s", result)
	}
	if strings.Contains(result, "Checking") {
		t.Errorf("BANK account Checking should not appear in EXPENSE filter, got:\n%s", result)
	}
}

// --- GetTransactions ---

func TestGetTransactions(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	result, err := svc.GetTransactions(ctx, "Checking", "2025-01-01", "2025-01-31", 50)
	if err != nil {
		t.Fatalf("GetTransactions() returned error: %v", err)
	}

	// 3 transactions in January: salary, supermarket, pizza
	for _, want := range []string{"January salary", "Supermarket", "Pizza place"} {
		if !strings.Contains(result, want) {
			t.Errorf("GetTransactions() missing %q in:\n%s", want, result)
		}
	}
	// February transaction should be excluded
	if strings.Contains(result, "Market") && !strings.Contains(result, "Supermarket") {
		t.Errorf("GetTransactions() should not include Feb transaction 'Market'")
	}
}

func TestGetTransactions_Limit(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	result, err := svc.GetTransactions(ctx, "Checking", "", "", 2)
	if err != nil {
		t.Fatalf("GetTransactions(limit=2) returned error: %v", err)
	}

	if !strings.Contains(result, "Showing 2 transactions") {
		t.Errorf("expected 2 transactions with limit=2, got:\n%s", result)
	}
}

func TestGetTransactions_NoResults(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	result, err := svc.GetTransactions(ctx, "Checking", "2020-01-01", "2020-12-31", 50)
	if err != nil {
		t.Fatalf("GetTransactions() returned error: %v", err)
	}

	if !strings.Contains(result, "No transactions found") {
		t.Errorf("expected 'No transactions found', got:\n%s", result)
	}
}

// --- SpendingByCategory ---

func TestSpendingByCategory(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	result, err := svc.SpendingByCategory(ctx, "2025-01-01", "2025-02-28", "")
	if err != nil {
		t.Fatalf("SpendingByCategory() returned error: %v", err)
	}

	// Groceries: 85.50 + 42.00 = 127.50, Restaurant: 25.00
	if !strings.Contains(result, "Groceries") {
		t.Errorf("expected Groceries category, got:\n%s", result)
	}
	if !strings.Contains(result, "127.50") {
		t.Errorf("expected 127.50 for Groceries, got:\n%s", result)
	}
	if !strings.Contains(result, "Restaurant") {
		t.Errorf("expected Restaurant category, got:\n%s", result)
	}
	if !strings.Contains(result, "25.00") {
		t.Errorf("expected 25.00 for Restaurant, got:\n%s", result)
	}
	// Grand total: 127.50 + 25.00 = 152.50
	if !strings.Contains(result, "152.50") {
		t.Errorf("expected grand total 152.50, got:\n%s", result)
	}
}

func TestSpendingByCategory_FilterByParent(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Filter by "Expenses" parent — both Groceries and Restaurant are direct children
	result, err := svc.SpendingByCategory(ctx, "2025-01-01", "2025-02-28", "Expenses")
	if err != nil {
		t.Fatalf("SpendingByCategory(parent=Expenses) returned error: %v", err)
	}

	if !strings.Contains(result, "Groceries") || !strings.Contains(result, "Restaurant") {
		t.Errorf("expected both categories under Expenses, got:\n%s", result)
	}
}

func TestSpendingByCategory_NoExpenses(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	result, err := svc.SpendingByCategory(ctx, "2020-01-01", "2020-12-31", "")
	if err != nil {
		t.Fatalf("SpendingByCategory() returned error: %v", err)
	}

	if !strings.Contains(result, "No expenses found") {
		t.Errorf("expected 'No expenses found', got:\n%s", result)
	}
}

// --- IncomeVsExpenses ---

func TestIncomeVsExpenses(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Use enough months to cover our fixture data (Jan-Feb 2025)
	result, err := svc.IncomeVsExpenses(ctx, 24)
	if err != nil {
		t.Fatalf("IncomeVsExpenses() returned error: %v", err)
	}

	// January: income 3000, expenses 85.50 + 25.00 = 110.50
	if !strings.Contains(result, "2025-01") {
		t.Errorf("expected 2025-01 in output, got:\n%s", result)
	}
	// February: income 3000, expenses 42.00
	if !strings.Contains(result, "2025-02") {
		t.Errorf("expected 2025-02 in output, got:\n%s", result)
	}
	// Should have column headers
	if !strings.Contains(result, "Income") || !strings.Contains(result, "Expenses") || !strings.Contains(result, "Net") {
		t.Errorf("expected column headers, got:\n%s", result)
	}
}

// --- SearchTransactions ---

func TestSearchTransactions(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	result, err := svc.SearchTransactions(ctx, "salary", 20)
	if err != nil {
		t.Fatalf("SearchTransactions() returned error: %v", err)
	}

	if !strings.Contains(result, "January salary") {
		t.Errorf("expected 'January salary' in results, got:\n%s", result)
	}
	if !strings.Contains(result, "February salary") {
		t.Errorf("expected 'February salary' in results, got:\n%s", result)
	}
	// Each result should show splits with account names
	if !strings.Contains(result, "Checking") || !strings.Contains(result, "Salary") {
		t.Errorf("expected split details with account names, got:\n%s", result)
	}
}

func TestSearchTransactions_NoMatch(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	result, err := svc.SearchTransactions(ctx, "nonexistent_xyz", 20)
	if err != nil {
		t.Fatalf("SearchTransactions() returned error: %v", err)
	}

	if !strings.Contains(result, "No transactions found") {
		t.Errorf("expected 'No transactions found', got:\n%s", result)
	}
}

func TestSearchTransactions_Limit(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// "a" matches most descriptions — limit to 1
	result, err := svc.SearchTransactions(ctx, "a", 1)
	if err != nil {
		t.Fatalf("SearchTransactions(limit=1) returned error: %v", err)
	}

	if !strings.Contains(result, "1 found") {
		t.Errorf("expected '1 found' with limit=1, got:\n%s", result)
	}
}

// --- ResolveAccount via full path ---

func TestGetBalance_FullPath(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Use colon-separated full path to resolve unambiguously
	result, err := svc.GetBalance(ctx, "Expenses:Groceries", "")
	if err != nil {
		t.Fatalf("GetBalance with full path returned error: %v", err)
	}

	if !strings.Contains(result, "127.50 EUR") {
		t.Errorf("expected 127.50 EUR, got:\n%s", result)
	}
}
