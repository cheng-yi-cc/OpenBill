package models

// Refund preserves the identity of a refund even after it is deleted. The
// linked transaction is the actual cash movement; the original expense stays
// unchanged and report amounts are derived from the active refund facts.
type Refund struct {
	Id                          string  `xorm:"PK VARCHAR(64)"`
	Uid                         int64   `xorm:"NOT NULL INDEX UNIQUE(UQE_refund_request)"`
	ExpenseId                   int64   `xorm:"NOT NULL INDEX"`
	TransactionId               int64   `xorm:"NOT NULL INDEX"`
	InstallmentMode             string  `xorm:"VARCHAR(16) NOT NULL DEFAULT ''"`
	RequestId                   string  `xorm:"VARCHAR(64) NOT NULL UNIQUE(UQE_refund_request)"`
	RequestDigest               string  `xorm:"VARCHAR(64) NOT NULL"`
	TimeZone                    string  `xorm:"VARCHAR(64) NOT NULL"`
	ExcessCategoryId            int64   `xorm:"INDEX NOT NULL DEFAULT 0"`
	ExcessBookId                string  `xorm:"VARCHAR(64) NOT NULL DEFAULT ''"`
	ExcessComment               string  `xorm:"VARCHAR(200) NOT NULL DEFAULT ''"`
	ExcessTagIds                []int64 `xorm:"TEXT JSON"`
	ExcessExcludeFromStatistics bool    `xorm:"NOT NULL DEFAULT 0"`
	ExcessExcludeFromBudget     bool    `xorm:"NOT NULL DEFAULT 0"`
	Version                     int64   `xorm:"NOT NULL"`
	Deleted                     bool    `xorm:"NOT NULL DEFAULT 0"`
	CreatedAt                   int64   `xorm:"NOT NULL"`
	UpdatedAt                   int64   `xorm:"NOT NULL"`
}

type RefundExcessInfo struct {
	CategoryId            string   `json:"categoryId"`
	BookId                string   `json:"bookId"`
	Comment               string   `json:"comment"`
	TagIds                []string `json:"tagIds"`
	ExcludeFromStatistics bool     `json:"excludeFromStatistics"`
	ExcludeFromBudget     bool     `json:"excludeFromBudget"`
}

// RefundProjection is an informational reporting view. Amounts are decimal
// strings; cash transactions and the original expense remain unchanged.
type RefundProjection struct {
	Kind           string            `json:"kind"`
	RefundId       string            `json:"refundId,omitempty"`
	ExpenseId      string            `json:"expenseId"`
	NetExpense     string            `json:"netExpense,omitempty"`
	RefundedAmount string            `json:"refundedAmount,omitempty"`
	OffsetAmount   string            `json:"offsetAmount,omitempty"`
	ExcessAmount   string            `json:"excessAmount,omitempty"`
	Excess         *RefundExcessInfo `json:"excess,omitempty"`
}
