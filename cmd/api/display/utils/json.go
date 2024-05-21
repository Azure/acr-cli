package utils

import (
	"encoding/json"
	"io"
)

// PrintPrettyJSON prints the object to the writer in JSON format.
func PrintPrettyJSON(out io.Writer, object any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(object)
}

// ToMap converts the data to a map[string]any with json tag as key.
func ToMap(data any) (map[string]any, error) {
	// slow but easy
	content, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	var ret map[string]any
	if err = json.Unmarshal(content, &ret); err != nil {
		return nil, err
	}
	return ret, nil
}
