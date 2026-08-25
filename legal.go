package main

import "embed"

//go:embed PRIVACY.md TERMS.md COPYRIGHT.md THIRD_PARTY_NOTICES.md
var legalDocs embed.FS
