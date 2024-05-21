package errors

// UnsupportedFormatTypeError generates the error message for an invalid type.
type UnsupportedFormatTypeError string

// Error implements the error interface.
func (e UnsupportedFormatTypeError) Error() string {
	return "unsupported format type: " + string(e)
}
