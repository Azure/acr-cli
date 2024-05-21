package template

import (
	"io"
	"text/template"

	"github.com/Azure/acr-cli/cmd/api/display/utils"
	"github.com/Masterminds/sprig/v3"
)

func parseAndWrite(out io.Writer, object any, templateStr string) error {
	// parse template
	t, err := template.New("format output").Funcs(sprig.FuncMap()).Parse(templateStr)
	if err != nil {
		return err
	}
	// convert object to map[string]any
	converted, err := utils.ToMap(object)
	if err != nil {
		return err
	}
	return t.Execute(out, converted)
}
