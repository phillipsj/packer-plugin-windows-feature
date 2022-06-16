package feature

import (
	_ "embed"
	"text/template"
)

type elevatedOptions struct {
	Username        string
	Password        string
	TaskName        string
	TaskDescription string
	Command         string
}

//go:embed elevated-template.ps1
var elevatedTemplatePs1 string

func elevatedTemplate() *template.Template {
	return template.Must(
		template.New("Elevated").Parse(
			elevatedTemplatePs1))
}
