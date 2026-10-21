package bootstrap

import _ "embed"

//go:embed tribal-knowledge.md
var Script string

const Version = "1"
