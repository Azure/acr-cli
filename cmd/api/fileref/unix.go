package fileref

import (
	"fmt"
	"strings"
)

// Parse parses file reference on unix.
func Parse(reference string, defaultMetadata string) (filePath, metadata string, err error) {
	i := strings.LastIndex(reference, ":")
	if i < 0 {
		filePath, metadata = reference, defaultMetadata
	} else {
		filePath, metadata = reference[:i], reference[i+1:]
	}
	if filePath == "" {
		return "", "", fmt.Errorf("found empty file path in %q", reference)
	}
	return filePath, metadata, nil
}
