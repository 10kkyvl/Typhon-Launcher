package selfupdate

import "strings"

// kindArtifact is shared by tests built with and without devmock.
func kindArtifact(goos, arch string, kind Kind, name string) Artifact {
	return Artifact{
		OS: goos, Arch: arch, Kind: kind, Name: name,
		URL: "https://cdn.example.com/" + name, Size: 1024, SHA256: strings.Repeat("0123456789abcdef", 4),
	}
}
