package main

func countTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return len(text) / 4
}
