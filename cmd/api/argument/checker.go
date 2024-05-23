package argument

import "fmt"

// Exactly checks if the number of arguments is exactly cnt.
func Exactly(cnt int) func(args []string) (bool, string) {
	return func(args []string) (bool, string) {
		return len(args) == cnt, fmt.Sprintf("exactly %d argument", cnt)
	}
}
