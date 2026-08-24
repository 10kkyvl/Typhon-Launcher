package download

import "strings"

const PartFileSuffix = ".part"

func IsPartFile(name string) bool {
	return strings.HasSuffix(name, PartFileSuffix)
}
