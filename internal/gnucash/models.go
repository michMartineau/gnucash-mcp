package gnucash

import (
	"fmt"
	"time"
)

// Account represents a GnuCash account in the chart of accounts.
type Account struct {
	GUID        string
	Name        string
	AccountType string
	ParentGUID  string
	Description string
	Hidden      bool
	Placeholder bool
	Children    []*Account
	FullName    string // computed: "Parent:Child:Grandchild"
}

// Transaction represents a GnuCash transaction header.
type Transaction struct {
	GUID        string
	PostDate    time.Time
	Description string
	Splits      []Split
}

// Split represents one leg of a double-entry transaction.
type Split struct {
	GUID        string
	TxGUID      string
	AccountGUID string
	AccountName string // joined from accounts table
	Memo        string
	ValueNum    int64
	ValueDenom  int64
}

// Amount returns the split value as a float64.
func (s Split) Amount() float64 {
	if s.ValueDenom == 0 {
		return 0
	}
	return float64(s.ValueNum) / float64(s.ValueDenom)
}

// FormatAmount returns the split value as a 2-decimal string.
func (s Split) FormatAmount() string {
	return FormatDecimal(s.ValueNum, s.ValueDenom)
}

// FormatDecimal formats a num/denom pair as a 2-decimal-place string.
func FormatDecimal(num, denom int64) string {
	if denom == 0 {
		return "0.00"
	}
	negative := false
	if num < 0 {
		negative = true
		num = -num
	}
	whole := num / denom
	frac := (num % denom) * 100 / denom
	sign := ""
	if negative {
		sign = "-"
	}
	return fmt.Sprintf("%s%d.%02d", sign, whole, frac)
}

// CategoryTotal holds aggregated spending for one expense category.
type CategoryTotal struct {
	Name  string
	Total string // formatted decimal
	Count int
}

// MonthSummary holds income vs expense totals for one month.
type MonthSummary struct {
	Month    string // YYYY-MM
	Income   string
	Expenses string
	Net      string
}
