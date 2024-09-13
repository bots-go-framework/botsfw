module github.com/bots-go-framework/bots-fw

go 1.22.3

//replace github.com/strongo/app => ../../strongo/app
//replace github.com/strongo/i18n => ../../strongo/i18n
//replace github.com/bots-go-framework/bots-fw-store => ../bots-fw-store

require (
	github.com/bots-go-framework/bots-fw-store v0.8.2
	github.com/bots-go-framework/bots-go-core v0.0.3
	github.com/dal-go/dalgo v0.13.1
	github.com/pquerna/ffjson v0.0.0-20190930134022-aa0246cd15f7
	github.com/stretchr/testify v1.9.0
	github.com/strongo/gamp v0.0.1
	github.com/strongo/i18n v0.5.0
	github.com/strongo/logus v0.2.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/strongo/random v0.0.1 // indirect
	github.com/strongo/validation v0.0.7 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
