package option

// format types
var (
	FormatTypeJSON = &FormatType{
		Name:  "json",
		Usage: "Print in JSON format",
	}
	FormatTypeGoTemplate = &FormatType{
		Name:      "go-template",
		Usage:     "Print output using the given Go template",
		HasParams: true,
	}
	FormatTypeTable = &FormatType{
		Name:  "table",
		Usage: "Get direct referrers and output in table format",
	}
	FormatTypeTree = &FormatType{
		Name:  "tree",
		Usage: "Get referrers recursively and print in tree format",
	}
)

// FormatType represents a format type.
type FormatType struct {
	// Name is the format type name.
	Name string
	// Usage is the usage string in help doc.
	Usage string
	// HasParams indicates whether the format type has parameters.
	HasParams bool
}

// Format contains input and parsed options for formatted output flags.
type Format struct {
	FormatFlag   string
	Type         string
	Template     string
	AllowedTypes []*FormatType
}
