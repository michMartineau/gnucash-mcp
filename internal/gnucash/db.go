package gnucash

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps a read-only SQLite connection to a GnuCash database.
type DB struct {
	db *sql.DB
}

// NewDB opens a GnuCash SQLite database in read-only mode.
func NewDB(filepath string) (*DB, error) {
	dsn := fmt.Sprintf("file:%s?mode=ro", filepath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &DB{db: db}, nil
}

// Close closes the database connection.
func (d *DB) Close() error {
	return d.db.Close()
}

// GetAllAccounts returns all accounts from the database.
func (d *DB) GetAllAccounts(ctx context.Context) ([]Account, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT c.guid, c.name, c.account_type,
			   COALESCE(c.parent_guid, ''),
			   COALESCE(c.description, ''),
			   c.hidden, c.placeholder
		FROM accounts c inner join accounts p on c.parent_guid = p.guid
		WHERE c.parent_guid IS NOT NULL AND p.name != 'Template Root'
		ORDER BY c.name;
	`)
	if err != nil {
		return nil, fmt.Errorf("query accounts: %w", err)
	}
	defer rows.Close()

	var accounts []Account
	for rows.Next() {
		var a Account
		var hidden, placeholder int
		if err := rows.Scan(&a.GUID, &a.Name, &a.AccountType, &a.ParentGUID, &a.Description, &hidden, &placeholder); err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		a.Hidden = hidden != 0
		a.Placeholder = placeholder != 0
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

// FindAccountsByName returns accounts matching a case-insensitive name pattern.
func (d *DB) FindAccountsByName(ctx context.Context, name string) ([]Account, error) {
	pattern := "%" + strings.ToLower(name) + "%"
	rows, err := d.db.QueryContext(ctx, `
		SELECT guid, name, account_type,
		       COALESCE(parent_guid, ''),
		       COALESCE(description, ''),
		       hidden, placeholder
		FROM accounts
		WHERE LOWER(name) LIKE ?
		ORDER BY name
	`, pattern)
	if err != nil {
		return nil, fmt.Errorf("query accounts by name: %w", err)
	}
	defer rows.Close()

	var accounts []Account
	for rows.Next() {
		var a Account
		var hidden, placeholder int
		if err := rows.Scan(&a.GUID, &a.Name, &a.AccountType, &a.ParentGUID, &a.Description, &hidden, &placeholder); err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		a.Hidden = hidden != 0
		a.Placeholder = placeholder != 0
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

// GetSplitsForAccount returns splits for an account, optionally filtered by date range.
// Splits are returned with their parent transaction data joined.
func (d *DB) GetSplitsForAccount(ctx context.Context, accountGUID string, startDate, endDate string, limit int) ([]Transaction, error) {
	query := `
		SELECT t.guid, t.post_date, t.description,
		       s.guid, s.memo, s.value_num, s.value_denom,
		       s2.account_guid, COALESCE(a2.name, ''), s2.value_num, s2.value_denom, COALESCE(s2.memo, '')
		FROM splits s
		JOIN transactions t ON s.tx_guid = t.guid
		JOIN splits s2 ON s2.tx_guid = t.guid AND s2.guid != s.guid
		JOIN accounts a2 ON s2.account_guid = a2.guid
		WHERE s.account_guid = ?
	`
	args := []any{accountGUID}

	if startDate != "" {
		query += " AND t.post_date >= ?"
		args = append(args, startDate+" 00:00:00")
	}
	if endDate != "" {
		query += " AND t.post_date <= ?"
		args = append(args, endDate+" 23:59:59")
	}
	query += " ORDER BY t.post_date DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query splits: %w", err)
	}
	defer rows.Close()

	txMap := make(map[string]*Transaction)
	var txOrder []string
	for rows.Next() {
		var txGUID, postDateStr, desc string
		var splitGUID, memo string
		var valueNum, valueDenom int64
		var counterAccGUID, counterAccName string
		var counterNum, counterDenom int64
		var counterMemo string

		if err := rows.Scan(&txGUID, &postDateStr, &desc,
			&splitGUID, &memo, &valueNum, &valueDenom,
			&counterAccGUID, &counterAccName, &counterNum, &counterDenom, &counterMemo); err != nil {
			return nil, fmt.Errorf("scan split: %w", err)
		}

		tx, exists := txMap[txGUID]
		if !exists {
			postDate, _ := parseDate(postDateStr)
			tx = &Transaction{
				GUID:        txGUID,
				PostDate:    postDate,
				Description: desc,
				Splits: []Split{{
					GUID:        splitGUID,
					TxGUID:      txGUID,
					AccountGUID: accountGUID,
					Memo:        memo,
					ValueNum:    valueNum,
					ValueDenom:  valueDenom,
				}},
			}
			txMap[txGUID] = tx
			txOrder = append(txOrder, txGUID)
		}
		// Add counterpart split
		tx.Splits = append(tx.Splits, Split{
			TxGUID:      txGUID,
			AccountGUID: counterAccGUID,
			AccountName: counterAccName,
			Memo:        counterMemo,
			ValueNum:    counterNum,
			ValueDenom:  counterDenom,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var transactions []Transaction
	for _, guid := range txOrder {
		transactions = append(transactions, *txMap[guid])
	}
	return transactions, nil
}

// GetBalanceForAccount returns the sum of all splits for an account up to the given date.
func (d *DB) GetBalanceForAccount(ctx context.Context, accountGUID string, endDate string) (int64, int64, error) {
	query := `
		SELECT COALESCE(SUM(s.value_num), 0), COALESCE(MAX(s.value_denom), 100)
		FROM splits s
		JOIN transactions t ON s.tx_guid = t.guid
		WHERE s.account_guid = ?
	`
	args := []any{accountGUID}
	if endDate != "" {
		query += " AND t.post_date <= ?"
		args = append(args, endDate+" 23:59:59")
	}

	var num, denom int64
	err := d.db.QueryRowContext(ctx, query, args...).Scan(&num, &denom)
	if err != nil {
		return 0, 0, fmt.Errorf("query balance: %w", err)
	}
	return num, denom, nil
}

// SearchTransactions searches transaction descriptions and split memos.
func (d *DB) SearchTransactions(ctx context.Context, query string, limit int) ([]Transaction, error) {
	pattern := "%" + strings.ToLower(query) + "%"
	sqlQuery := `
		SELECT DISTINCT t.guid, t.post_date, t.description
		FROM transactions t
		LEFT JOIN splits s ON s.tx_guid = t.guid
		WHERE LOWER(t.description) LIKE ? OR LOWER(s.memo) LIKE ?
		ORDER BY t.post_date DESC
		LIMIT ?
	`
	rows, err := d.db.QueryContext(ctx, sqlQuery, pattern, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search transactions: %w", err)
	}
	defer rows.Close()

	var txGUIDs []string
	txMap := make(map[string]*Transaction)
	for rows.Next() {
		var guid, postDateStr, desc string
		if err := rows.Scan(&guid, &postDateStr, &desc); err != nil {
			return nil, fmt.Errorf("scan transaction: %w", err)
		}
		postDate, _ := parseDate(postDateStr)
		tx := &Transaction{GUID: guid, PostDate: postDate, Description: desc}
		txMap[guid] = tx
		txGUIDs = append(txGUIDs, guid)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Load splits for each transaction
	for _, guid := range txGUIDs {
		splits, err := d.getSplitsForTransaction(ctx, guid)
		if err != nil {
			return nil, err
		}
		txMap[guid].Splits = splits
	}

	var transactions []Transaction
	for _, guid := range txGUIDs {
		transactions = append(transactions, *txMap[guid])
	}
	return transactions, nil
}

func (d *DB) getSplitsForTransaction(ctx context.Context, txGUID string) ([]Split, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT s.guid, s.tx_guid, s.account_guid, COALESCE(a.name, ''),
		       COALESCE(s.memo, ''), s.value_num, s.value_denom
		FROM splits s
		JOIN accounts a ON s.account_guid = a.guid
		WHERE s.tx_guid = ?
	`, txGUID)
	if err != nil {
		return nil, fmt.Errorf("query splits for tx: %w", err)
	}
	defer rows.Close()

	var splits []Split
	for rows.Next() {
		var s Split
		if err := rows.Scan(&s.GUID, &s.TxGUID, &s.AccountGUID, &s.AccountName,
			&s.Memo, &s.ValueNum, &s.ValueDenom); err != nil {
			return nil, fmt.Errorf("scan split: %w", err)
		}
		splits = append(splits, s)
	}
	return splits, rows.Err()
}

// GetExpenseSplits returns all splits for expense accounts in a date range,
// grouped by account.
func (d *DB) GetExpenseSplits(ctx context.Context, startDate, endDate string, parentAccountGUID string) (map[string][]Split, map[string]string, error) {
	query := `
		SELECT s.value_num, s.value_denom, a.guid, a.name, a.parent_guid
		FROM splits s
		JOIN transactions t ON s.tx_guid = t.guid
		JOIN accounts a ON s.account_guid = a.guid
		WHERE a.account_type = 'EXPENSE'
		  AND t.post_date >= ?
		  AND t.post_date <= ?
	`
	args := []any{startDate + " 00:00:00", endDate + " 23:59:59"}

	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("query expense splits: %w", err)
	}
	defer rows.Close()

	// accountGUID -> []Split
	byAccount := make(map[string][]Split)
	// accountGUID -> accountName
	names := make(map[string]string)
	// accountGUID -> parentGUID
	parents := make(map[string]string)

	for rows.Next() {
		var s Split
		var accGUID, accName, parentGUID string
		if err := rows.Scan(&s.ValueNum, &s.ValueDenom, &accGUID, &accName, &parentGUID); err != nil {
			return nil, nil, fmt.Errorf("scan expense split: %w", err)
		}
		s.AccountGUID = accGUID
		s.AccountName = accName
		byAccount[accGUID] = append(byAccount[accGUID], s)
		names[accGUID] = accName
		parents[accGUID] = parentGUID
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	// Filter by parent account if specified
	if parentAccountGUID != "" {
		filtered := make(map[string][]Split)
		for guid, splits := range byAccount {
			if parents[guid] == parentAccountGUID {
				filtered[guid] = splits
			}
		}
		byAccount = filtered
	}

	return byAccount, names, nil
}

// GetMonthlyIncomeExpenses returns monthly totals for income and expense accounts.
func (d *DB) GetMonthlyIncomeExpenses(ctx context.Context, startDate, endDate string) ([]struct {
	Month   string
	AccType string
	Total   int64
	Denom   int64
}, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT strftime('%Y-%m', t.post_date) as month,
		       a.account_type,
		       SUM(s.value_num) as total,
		       MAX(s.value_denom) as denom
		FROM splits s
		JOIN transactions t ON s.tx_guid = t.guid
		JOIN accounts a ON s.account_guid = a.guid
		WHERE a.account_type IN ('INCOME', 'EXPENSE')
		  AND t.post_date >= ?
		  AND t.post_date <= ?
		GROUP BY month, a.account_type
		ORDER BY month
	`, startDate+" 00:00:00", endDate+" 23:59:59")
	if err != nil {
		return nil, fmt.Errorf("query monthly totals: %w", err)
	}
	defer rows.Close()

	type row struct {
		Month   string
		AccType string
		Total   int64
		Denom   int64
	}
	var results []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.Month, &r.AccType, &r.Total, &r.Denom); err != nil {
			return nil, fmt.Errorf("scan monthly total: %w", err)
		}
		results = append(results, r)
	}

	type returnRow = struct {
		Month   string
		AccType string
		Total   int64
		Denom   int64
	}
	var ret []returnRow
	for _, r := range results {
		ret = append(ret, returnRow(r))
	}
	return ret, rows.Err()
}

func parseDate(s string) (time.Time, error) {
	// Try the actual DB format first
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		// Fallback to the format documented in CLAUDE.md
		t, err = time.Parse("20060102150405", s)
	}
	return t, err
}
