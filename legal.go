package main

import "embed"

//go:embed PRIVACY.md TERMS.md COPYRIGHT.md THIRD_PARTY_NOTICES.md
//go:embed PRIVACY.en.md TERMS.en.md COPYRIGHT.en.md THIRD_PARTY_NOTICES.en.md

var legalDocs embed.FS
