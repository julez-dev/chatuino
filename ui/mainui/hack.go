package mainui

import (
	"regexp"
	"strings"
)

var ansiRegex = regexp.MustCompile(`(\x9B|\x1B\[)[0-?]*[ -\/]*[@-~]`)

func stripAnsi(str string) string {
	return ansiRegex.ReplaceAllString(str, "")
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
