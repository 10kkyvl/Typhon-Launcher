package wine

import "strings"

func contains(haystack, needle string) bool { return strings.Contains(haystack, needle) }

func countLines(text, prefix string) int {
	count := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, prefix) {
			count++
		}
	}
	return count
}

// retarget переносит фикстуру ps на букву, которую бутыль получил в тесте.
func retarget(text, drive string) string {
	return strings.ReplaceAll(text, `T:\`, strings.ToUpper(drive)+`:\`)
}
