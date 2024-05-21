package option

import "os"

// Common option struct.
type Common struct {
	Debug   bool
	Verbose bool
	TTY     *os.File
}
