package model

// CategoryType represents the type of category
type CategoryType int

const (
	CategoryTypeGeneral    CategoryType = 1
	CategoryTypeExpense    CategoryType = 2
	CategoryTypeIncome     CategoryType = 3
	CategoryTypeInvestment CategoryType = 4
)

// String returns the string representation of CategoryType
func (ct CategoryType) String() string {
	switch ct {
	case CategoryTypeGeneral:
		return "General"
	case CategoryTypeExpense:
		return "Expense"
	case CategoryTypeIncome:
		return "Income"
	case CategoryTypeInvestment:
		return "Investment"
	default:
		return "General"
	}
}

// CategoryTypeFromString converts a string to CategoryType
func CategoryTypeFromString(s string) CategoryType {
	switch s {
	case "Expense":
		return CategoryTypeExpense
	case "Income":
		return CategoryTypeIncome
	case "Investment":
		return CategoryTypeInvestment
	default:
		return CategoryTypeGeneral
	}
}

// CategorySource indicates how a transaction was categorized
type CategorySource int

const (
	CategorySourceNone   CategorySource = 0 // Uncategorized
	CategorySourceRule   CategorySource = 1 // Auto-categorized by pattern
	CategorySourceManual CategorySource = 2 // Manually assigned by user
)

// String returns the string representation of CategorySource
func (cs CategorySource) String() string {
	switch cs {
	case CategorySourceNone:
		return "None"
	case CategorySourceRule:
		return "Rule"
	case CategorySourceManual:
		return "Manual"
	default:
		return "None"
	}
}
