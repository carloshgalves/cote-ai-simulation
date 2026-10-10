//go:build ignore

// Generates the compact Unicode 15.1 General_Category=Cn range table used by
// the canonical codec. The input hash is pinned in unicode.json.
package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var linePattern = regexp.MustCompile(`^([0-9A-F]+)(?:\.\.([0-9A-F]+))?\s*;\s*Cn\b`)

func main() {
	if len(os.Args) != 4 {
		panic("usage: go run gen_unicode_table.go <DerivedGeneralCategory.txt> <output.go> <output.ts>")
	}
	input, err := os.Open(os.Args[1])
	if err != nil { panic(err) }
	defer input.Close()
	output, err := os.Create(os.Args[2])
	if err != nil { panic(err) }
	defer output.Close()
	typeScript, err := os.Create(os.Args[3])
	if err != nil { panic(err) }
	defer typeScript.Close()
	fmt.Fprintln(output, "// Code generated from Unicode 15.1.0 DerivedGeneralCategory.txt; DO NOT EDIT.")
	fmt.Fprintln(output, "package codec")
	fmt.Fprintln(output, "\nvar unicode15Unassigned = [...][2]rune{")
	fmt.Fprintln(typeScript, "// Code generated from Unicode 15.1.0 DerivedGeneralCategory.txt; DO NOT EDIT.")
	fmt.Fprintln(typeScript, "export const unicode15Unassigned = [")
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		match := linePattern.FindStringSubmatch(scanner.Text())
		if match == nil { continue }
		start, _ := strconv.ParseUint(match[1], 16, 32)
		end := start
		if strings.TrimSpace(match[2]) != "" { end, _ = strconv.ParseUint(match[2], 16, 32) }
		fmt.Fprintf(output, "\t{0x%X, 0x%X},\n", start, end)
		fmt.Fprintf(typeScript, "  [0x%X, 0x%X],\n", start, end)
	}
	if err := scanner.Err(); err != nil { panic(err) }
	fmt.Fprintln(output, "}")
	fmt.Fprintln(typeScript, "];")
}
