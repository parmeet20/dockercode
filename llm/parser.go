package llm

import (
	"strings"
)

func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}

	charCount := len(text)
	wordCount := len(strings.Fields(text))
	estFromChars := charCount / 4
	estFromWords := (wordCount * 4) / 3
	res := estFromChars
	if estFromWords > res {
		res = estFromWords
	}
	if res < 1 {
		return 1
	}
	return res
}
