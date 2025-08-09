package main

import (
	"fmt"
	"regexp"
	"strconv"
)

func decodeUnicode(text string) string {
	// Regex to match \U+xxxx patterns
	re := regexp.MustCompile(`\\U\+([0-9A-Fa-f]{4})`)
	
	result := re.ReplaceAllStringFunc(text, func(match string) string {
		fmt.Printf("Found match: %s\n", match)
		// Extract the hex code (remove \U+)
		hexCode := match[3:]
		fmt.Printf("Hex code: %s\n", hexCode)
		
		// Parse the hex code to integer
		if codePoint, err := strconv.ParseInt(hexCode, 16, 32); err == nil {
			// Convert to Unicode character
			char := string(rune(codePoint))
			fmt.Printf("Converted to: %s (code point: %d)\n", char, codePoint)
			return char
		}
		
		// If parsing fails, return original
		fmt.Printf("Failed to parse, returning original\n")
		return match
	})
	
	return result
}

func main() {
	text := "90\\U+00B0 LR-Elbow"
	fmt.Printf("Input: %s\n", text)
	result := decodeUnicode(text)
	fmt.Printf("Output: %s\n", result)
	
	// Also test with the exact bytes we see in the CSV
	fmt.Printf("Degree symbol: %s (bytes: %X)\n", "°", []byte("°"))
}
