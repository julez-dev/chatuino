package mainui

import (
	"regexp"
	"strings"
)

var (
	duplicateBypass   = rune(917504)
	ansiRegex         = regexp.MustCompile(`(\x9B|\x1B\[)[0-?]*[ -\/]*[@-~]`)
	accountStartRegex = regexp.MustCompile(`^[^a-zA-Z0-9_-]+`)
	accountEndRegex   = regexp.MustCompile(`[^a-zA-Z0-9_-]+$`)
)

func stripAnsi(str string) string {
	return ansiRegex.ReplaceAllString(str, "")
}

func stripDisplayNameEdges(str string) string {
	return accountEndRegex.ReplaceAllString(accountStartRegex.ReplaceAllString(str, ""), "")
}

func clamp(v, low, high int) int {
	return min(max(v, low), high)
}

func selectWordAtIndex(sentence string, index int) string {
	if index > len(sentence) || sentence == "" {
		return ""
	}

	before, after := sentence[:index], sentence[index:]

	spaceIndexBefore := strings.LastIndex(before, " ")

	if spaceIndexBefore == -1 {
		spaceIndexBefore = 0
	} else {
		spaceIndexBefore++
	}

	spaceIndexAfter := strings.Index(after, " ")

	if spaceIndexAfter == -1 {
		spaceIndexAfter = index + len(after)
	} else {
		spaceIndexAfter = index + spaceIndexAfter
	}

	return sentence[spaceIndexBefore:spaceIndexAfter]
}
