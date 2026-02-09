package gnucash

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Service provides business logic for GnuCash data access.
type Service struct {
	db *DB
}

// NewService creates a new Service wrapping a database connection.
func NewService(db *DB) *Service {
	return &Service{db: db}
}

// ListAccounts returns accounts as a tree, optionally filtered by type.
func (s *Service) ListAccounts(ctx context.Context, accountType string) (string, error) {
	accounts, err := s.db.GetAllAccounts(ctx)
	if err != nil {
		return "", err
	}

	// Format output
	var sb strings.Builder
	for _, acc := range accounts {
		fmt.Fprintf(&sb, "%s\t%s%i\n", acc.FullName, acc.AccountType, acc.)
	}

	result := sb.String()
	if result == "" {
		return "No accounts found.", nil
	}
	return result, nil
}

// resolveAccount finds a single account by name. Returns an error if no match or ambiguous.
func (s *Service) resolveAccount(ctx context.Context, name string) (*Account, error) {
	accounts, err := s.db.FindAccountsByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, fmt.Errorf("no account found matching '%s'", name)
	}

	// Check for exact match first (case-insensitive)
	for i, a := range accounts {
		if strings.EqualFold(a.Name, name) {
			return &accounts[i], nil
		}
	}

	if len(accounts) > 1 {
		names := make([]string, len(accounts))
		for i, a := range accounts {
			names[i] = fmt.Sprintf("  - %s [%s]", a.Name, a.AccountType)
		}
		return nil, fmt.Errorf("multiple accounts match '%s':\n%s\nPlease be more specific.", name, strings.Join(names, "\n"))
	}

	return &accounts[0], nil
}

// GetBalance returns the balance for a named account as of a given date.
func (s *Service) GetBalance(ctx context.Context, accountName, date string) (string, error) {
	account, err := s.resolveAccount(ctx, accountName)
	if err != nil {
		return "", err
	}

	num, denom, err := s.db.GetBalanceForAccount(ctx, account.GUID, date)
	if err != nil {
		return "", err
	}

	balance := FormatDecimal(num, denom)

	dateLabel := "current"
	if date != "" {
		dateLabel = "as of " + date
	}

	return fmt.Sprintf("Account: %s [%s]\nBalance (%s): %s EUR", account.Name, account.AccountType, dateLabel, balance), nil
}

// GetTransactions returns transactions for a named account within a date range.
func (s *Service) GetTransactions(ctx context.Context, accountName, startDate, endDate string, limit int) (string, error) {
	account, err := s.resolveAccount(ctx, accountName)
	if err != nil {
		return "", err
	}

	if limit <= 0 {
		limit = 50
	}

	transactions, err := s.db.GetSplitsForAccount(ctx, account.GUID, startDate, endDate, limit)
	if err != nil {
		return "", err
	}

	if len(transactions) == 0 {
		return fmt.Sprintf("No transactions found for %s in the given period.", account.Name), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Transactions for %s [%s]", account.Name, account.AccountType)
	if startDate != "" || endDate != "" {
		sb.WriteString(" (")
		if startDate != "" {
			sb.WriteString("from " + startDate)
		}
		if endDate != "" {
			if startDate != "" {
				sb.WriteString(" ")
			}
			sb.WriteString("to " + endDate)
		}
		sb.WriteString(")")
	}
	fmt.Fprintf(&sb, "\nShowing %d transactions:\n\n", len(transactions))

	for _, tx := range transactions {
		// The first split is for the queried account
		amount := tx.Splits[0].FormatAmount()
		counterparts := make([]string, 0, len(tx.Splits)-1)
		for _, sp := range tx.Splits[1:] {
			counterparts = append(counterparts, sp.AccountName)
		}
		counter := strings.Join(counterparts, ", ")

		fmt.Fprintf(&sb, "%s  %s EUR  %s", tx.PostDate.Format("2006-01-02"), amount, tx.Description)
		if counter != "" {
			fmt.Fprintf(&sb, "  [%s]", counter)
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// SpendingByCategory returns expense totals grouped by category.
func (s *Service) SpendingByCategory(ctx context.Context, startDate, endDate, parentAccount string) (string, error) {
	now := time.Now()
	if startDate == "" {
		startDate = now.Format("2006-01") + "-01"
	}
	if endDate == "" {
		endDate = now.Format("2006-01-02")
	}

	var parentGUID string
	if parentAccount != "" {
		acc, err := s.resolveAccount(ctx, parentAccount)
		if err != nil {
			return "", err
		}
		parentGUID = acc.GUID
	}

	byAccount, names, err := s.db.GetExpenseSplits(ctx, startDate, endDate, parentGUID)
	if err != nil {
		return "", err
	}

	if len(byAccount) == 0 {
		return fmt.Sprintf("No expenses found from %s to %s.", startDate, endDate), nil
	}

	type catEntry struct {
		Name  string
		Total int64
		Denom int64
		Count int
	}
	var categories []catEntry
	for guid, splits := range byAccount {
		var total int64
		var denom int64 = 100
		for _, sp := range splits {
			total += sp.ValueNum
			denom = sp.ValueDenom
		}
		categories = append(categories, catEntry{
			Name:  names[guid],
			Total: total,
			Denom: denom,
			Count: len(splits),
		})
	}

	// Sort by total descending
	sort.Slice(categories, func(i, j int) bool {
		return categories[i].Total > categories[j].Total
	})

	var sb strings.Builder
	fmt.Fprintf(&sb, "Spending by category (%s to %s):\n\n", startDate, endDate)

	var grandTotal int64
	var grandDenom int64 = 100
	for _, cat := range categories {
		fmt.Fprintf(&sb, "  %-30s %10s EUR  (%d transactions)\n",
			cat.Name, FormatDecimal(cat.Total, cat.Denom), cat.Count)
		grandTotal += cat.Total
		grandDenom = cat.Denom
	}
	fmt.Fprintf(&sb, "\n  %-30s %10s EUR\n", "TOTAL", FormatDecimal(grandTotal, grandDenom))

	return sb.String(), nil
}

// IncomeVsExpenses returns a monthly comparison of income and expenses.
func (s *Service) IncomeVsExpenses(ctx context.Context, months int) (string, error) {
	if months <= 0 {
		months = 6
	}

	now := time.Now()
	endDate := now.Format("2006-01-02")
	startDate := now.AddDate(0, -months+1, -now.Day()+1).Format("2006-01-02")

	rows, err := s.db.GetMonthlyIncomeExpenses(ctx, startDate, endDate)
	if err != nil {
		return "", err
	}

	// Organize by month
	type monthData struct {
		Income   int64
		Expenses int64
		Denom    int64
	}
	byMonth := make(map[string]*monthData)
	var monthOrder []string

	for _, r := range rows {
		md, exists := byMonth[r.Month]
		if !exists {
			md = &monthData{Denom: 100}
			byMonth[r.Month] = md
			monthOrder = append(monthOrder, r.Month)
		}
		if r.Denom > 0 {
			md.Denom = r.Denom
		}
		switch r.AccType {
		case "INCOME":
			// Income splits are negative in GnuCash (credit), negate for display
			md.Income = -r.Total
		case "EXPENSE":
			md.Expenses = r.Total
		}
	}

	sort.Strings(monthOrder)

	var sb strings.Builder
	fmt.Fprintf(&sb, "Income vs Expenses (last %d months):\n\n", months)
	fmt.Fprintf(&sb, "  %-10s %12s %12s %12s\n", "Month", "Income", "Expenses", "Net")
	fmt.Fprintf(&sb, "  %s\n", strings.Repeat("-", 48))

	for _, month := range monthOrder {
		md := byMonth[month]
		net := md.Income - md.Expenses
		fmt.Fprintf(&sb, "  %-10s %12s %12s %12s\n",
			month,
			FormatDecimal(md.Income, md.Denom),
			FormatDecimal(md.Expenses, md.Denom),
			FormatDecimal(net, md.Denom))
	}

	return sb.String(), nil
}

// SearchTransactions searches for transactions by description or memo.
func (s *Service) SearchTransactions(ctx context.Context, query string, limit int) (string, error) {
	if limit <= 0 {
		limit = 20
	}

	transactions, err := s.db.SearchTransactions(ctx, query, limit)
	if err != nil {
		return "", err
	}

	if len(transactions) == 0 {
		return fmt.Sprintf("No transactions found matching '%s'.", query), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Search results for '%s' (%d found):\n\n", query, len(transactions))

	for _, tx := range transactions {
		fmt.Fprintf(&sb, "%s  %s\n", tx.PostDate.Format("2006-01-02"), tx.Description)
		for _, sp := range tx.Splits {
			fmt.Fprintf(&sb, "    %s: %s EUR", sp.AccountName, sp.FormatAmount())
			if sp.Memo != "" {
				fmt.Fprintf(&sb, "  (%s)", sp.Memo)
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}
