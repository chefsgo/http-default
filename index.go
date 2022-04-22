package http_default

import (
	"github.com/chefsgo/chef"
)

func Driver() chef.HttpDriver {
	return &defaultHttpDriver{}
}

func init() {
	chef.Register("default", Driver())
}
