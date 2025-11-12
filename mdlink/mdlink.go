// mdlink - Markdown Link Manager CLI
// =====================================
// USAGE:
//   mdlink find <file> <url>
//   mdlink insert <file> <url> <title> <section> [subsection]
//   mdlink update <file> <old_url> <new_url> <title>
//   mdlink list <file> [section]
//   mdlink --help
//   mdlink --version
//
// Core logic:
// - Sections: # headings only, end at next #
// - Subsections: > blockquotes (direct children before any heading) OR ## headings
// - Duplicates: per-section, same URL OK across sections

package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

const version = "0.11.0"

type Line struct {
	text  string
	level int
}

type Subsection struct {
	Name      string
	StartLine int
	EndLine   int
	IsQuote   bool // true if "> ", false if heading
}

type Section struct {
	Name        string
	StartLine   int
	EndLine     int
	Level       int
	Subsections []*Subsection
}

type Document struct {
	Lines    []Line
	Sections []*Section
}

func readFile(path string) ([]Line, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lines []Line
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		text := scanner.Text()
		level := getHeadingLevel(text)
		lines = append(lines, Line{text, level})
	}
	return lines, scanner.Err()
}

func writeFile(path string, lines []Line) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for i, line := range lines {
		if i > 0 {
			w.WriteString("\n")
		}
		w.WriteString(line.text)
	}
	return w.Flush()
}

func getHeadingLevel(text string) int {
	match := regexp.MustCompile(`^(#+)`).FindString(text)
	return len(match)
}

func getHeadingTitle(text string) string {
	re := regexp.MustCompile(`^#+\s+(.+)$`)
	m := re.FindStringSubmatch(text)
	if len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func normalize(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "\\&", "&")
	return strings.TrimSpace(s)
}

// parseDocument: extract top-level sections (# only)
// Section boundary: next # heading (## and ### don't stop it)
func parseDocument(lines []Line) *Document {
	doc := &Document{Lines: lines, Sections: []*Section{}}

	for i := 0; i < len(lines); i++ {
		if lines[i].level != 1 {
			continue
		}

		title := getHeadingTitle(lines[i].text)
		section := &Section{
			Name:      title,
			StartLine: i,
			EndLine:   len(lines) - 1,
			Level:     1,
		}

		for j := i + 1; j < len(lines); j++ {
			if lines[j].level == 1 {
				section.EndLine = j - 1
				break
			}
		}

		section.Subsections = parseSubsections(lines, section)
		doc.Sections = append(doc.Sections, section)
		i = section.EndLine
	}

	return doc
}

// parseSubsections: find > blockquotes AND ## headings
// Blockquotes: level-0 text before first heading, prefixed "> "
// Headings: any level > 1
func parseSubsections(lines []Line, section *Section) []*Subsection {
	var subs []*Subsection

	// Pass 1: collect ALL blockquotes in direct block
	// A blockquote is any level-0 line starting with "> " BEFORE any heading
	firstHeadingIdx := -1
	for i := section.StartLine + 1; i <= section.EndLine; i++ {
		if lines[i].level > 0 {
			firstHeadingIdx = i
			break
		}
	}

	// Scan blockquotes only from section start to first heading (or section end)
	quoteEnd := section.EndLine
	if firstHeadingIdx >= 0 {
		quoteEnd = firstHeadingIdx - 1
	}

	for i := section.StartLine + 1; i <= quoteEnd; i++ {
		trim := strings.TrimSpace(lines[i].text)
		if strings.HasPrefix(trim, "> ") {
			label := strings.TrimSpace(strings.TrimPrefix(trim, "> "))
			sub := &Subsection{
				Name:      label,
				StartLine: i,
				IsQuote:   true,
				EndLine:   i,
			}

			// Find end of blockquote block: next blockquote or next heading
			for j := i + 1; j <= section.EndLine; j++ {
				if lines[j].level > 0 {
					sub.EndLine = j - 1
					break
				}
				trim := strings.TrimSpace(lines[j].text)
				if strings.HasPrefix(trim, "> ") {
					sub.EndLine = j - 1
					break
				}
				if j == section.EndLine {
					sub.EndLine = j
				}
			}

			subs = append(subs, sub)
		}
	}

	// Pass 2: collect all headings (## and deeper)
	for i := section.StartLine + 1; i <= section.EndLine; i++ {
		if lines[i].level > 1 {
			label := getHeadingTitle(lines[i].text)
			sub := &Subsection{
				Name:      label,
				StartLine: i,
				IsQuote:   false,
				EndLine:   i,
			}

			// Find heading end: next heading of same/lower level
			for j := i + 1; j <= section.EndLine; j++ {
				if lines[j].level > 0 && lines[j].level <= lines[i].level {
					sub.EndLine = j - 1
					break
				}
				if j == section.EndLine {
					sub.EndLine = j
				}
			}

			subs = append(subs, sub)
		}
	}

	return subs
}

func findLinkInLines(lines []Line, url string) (int, string) {
	for i, line := range lines {
		if strings.Contains(line.text, "]"+url+")") || strings.Contains(line.text, "]("+url) {
			return i, line.text
		}
	}
	return -1, ""
}

func findSectionInDoc(doc *Document, section string) *Section {
	normSection := normalize(section)
	for _, sec := range doc.Sections {
		if normalize(sec.Name) == normSection {
			return sec
		}
	}
	return nil
}

func findSubsectionInSection(section *Section, subsection string) *Subsection {
	normSub := normalize(subsection)
	for _, sub := range section.Subsections {
		if normalize(sub.Name) == normSub {
			return sub
		}
	}
	return nil
}

func countListItems(lines []Line, startIdx int, endIdx int) int {
	count := 0
	for i := startIdx; i <= endIdx && i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i].text), "* ") {
			count++
		}
	}
	return count
}

func chooseBestQuoteSubsection(lines []Line, section *Section) *Subsection {
	var quotes []*Subsection
	for _, sub := range section.Subsections {
		if sub.IsQuote {
			quotes = append(quotes, sub)
		}
	}

	if len(quotes) == 0 {
		return nil
	}

	// Prefer "Learn"
	for _, sub := range quotes {
		if normalize(sub.Name) == normalize("Learn") {
			return sub
		}
	}

	// Else: most items
	best := quotes[len(quotes)-1]
	bestCount := -1
	for _, sub := range quotes {
		count := countListItems(lines, sub.StartLine+1, sub.EndLine)
		if count > bestCount {
			bestCount = count
			best = sub
		}
	}
	return best
}

func parseLink(text string) (string, string) {
	re := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	m := re.FindStringSubmatch(text)
	if len(m) > 2 {
		return m[1], m[2]
	}
	return "", ""
}

func cmdFind(file, url string) {
	lines, _ := readFile(file)
	idx, text := findLinkInLines(lines, url)
	if idx < 0 {
		fmt.Println("NOT_FOUND")
		return
	}
	title, _ := parseLink(text)
	fmt.Printf("Line %d: %s\n", idx+1, text)
	fmt.Printf("Title: %s\n", title)
}

func cmdList(file, section string) {
	lines, _ := readFile(file)
	doc := parseDocument(lines)

	if section != "" {
		sec := findSectionInDoc(doc, section)
		if sec == nil {
			fmt.Printf("Section '%s' not found\n", section)
			return
		}
		fmt.Printf("# %s (lines %d-%d)\n", sec.Name, sec.StartLine+1, sec.EndLine+1)
		for _, sub := range sec.Subsections {
			marker := ">"
			if !sub.IsQuote {
				marker = "##"
			}
			fmt.Printf("  %s %s (lines %d-%d)\n", marker, sub.Name, sub.StartLine+1, sub.EndLine+1)
		}
		fmt.Println()
		linkRe := regexp.MustCompile(`^\s*\*\s+\[`)
		for i := sec.StartLine; i <= sec.EndLine && i < len(lines); i++ {
			if linkRe.MatchString(lines[i].text) {
				title, url := parseLink(lines[i].text)
				fmt.Printf("  [%d] %s (%s)\n", i+1, title, url)
			}
		}
	} else {
		fmt.Println("Sections:")
		for _, sec := range doc.Sections {
			fmt.Printf("# %s (lines %d-%d)\n", sec.Name, sec.StartLine+1, sec.EndLine+1)
			for _, sub := range sec.Subsections {
				marker := ">"
				if !sub.IsQuote {
					marker = "##"
				}
				fmt.Printf("  %s %s (lines %d-%d)\n", marker, sub.Name, sub.StartLine+1, sub.EndLine+1)
			}
		}
	}
}

func cmdInsert(file, url, title, section, subsection string) {
	lines, _ := readFile(file)
	doc := parseDocument(lines)
	sec := findSectionInDoc(doc, section)
	if sec == nil {
		fmt.Printf("Section '%s' not found\n", section)
		os.Exit(1)
	}

	for i := sec.StartLine; i <= sec.EndLine && i < len(lines); i++ {
		if strings.Contains(lines[i].text, "]"+url+")") || strings.Contains(lines[i].text, "]("+url) {
			fmt.Println("EXISTS")
			return
		}
	}

	newLink := fmt.Sprintf("* [%s](%s)", title, url)
	var endOfContent int

	if subsection != "" {
		sub := findSubsectionInSection(sec, subsection)
		if sub == nil {
			fmt.Printf("Subsection '%s' not found under '%s'\n", subsection, section)
			os.Exit(1)
		}
		endOfContent = sub.EndLine
	} else {
		bestSub := chooseBestQuoteSubsection(lines, sec)
		if bestSub != nil {
			endOfContent = bestSub.EndLine
		} else {
			endOfContent = sec.EndLine
		}
	}

	insertIdx := endOfContent
	for insertIdx > 0 && strings.TrimSpace(lines[insertIdx].text) == "" {
		insertIdx--
	}
	insertIdx++

	for insertIdx < len(lines) && strings.TrimSpace(lines[insertIdx].text) == "" {
		lines = append(lines[:insertIdx], lines[insertIdx+1:]...)
	}

	newLines := make([]Line, 0, len(lines)+2)
	newLines = append(newLines, lines[:insertIdx]...)
	newLines = append(newLines, Line{newLink, 0})
	if insertIdx < len(lines) {
		newLines = append(newLines, Line{"", 0})
	}
	newLines = append(newLines, lines[insertIdx:]...)

	writeFile(file, newLines)
	fmt.Printf("INSERTED at line %d\n", insertIdx+1)
}

func cmdUpdate(file, oldURL, newURL, title string) {
	lines, _ := readFile(file)
	idx, _ := findLinkInLines(lines, oldURL)
	if idx < 0 {
		fmt.Println("NOT_FOUND")
		return
	}
	lines[idx].text = fmt.Sprintf("* [%s](%s)", title, newURL)
	writeFile(file, lines)
	fmt.Printf("UPDATED line %d\n", idx+1)
}

func showHelp() {
	fmt.Print(`mdlink - Markdown Link Manager

COMMANDS:
  find <file> <url>
  list <file> [section]
  insert <file> <url> <title> <section> [subsection]
  update <file> <old_url> <new_url> <title>

Sections: # only
Subsections: > blockquotes (before first heading) OR ## headings
`)
}

func main() {
	if len(os.Args) < 2 {
		showHelp()
		return
	}
	switch os.Args[1] {
	case "--help", "-h", "help":
		showHelp()
	case "--version", "-v", "version":
		fmt.Printf("mdlink %s\n", version)
	case "find":
		if len(os.Args) < 4 {
			fmt.Fprintf(os.Stderr, "Usage: mdlink find <file> <url>\n")
			os.Exit(1)
		}
		cmdFind(os.Args[2], os.Args[3])
	case "list":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Usage: mdlink list <file> [section]\n")
			os.Exit(1)
		}
		section := ""
		if len(os.Args) > 3 {
			section = os.Args[3]
		}
		cmdList(os.Args[2], section)
	case "insert":
		if len(os.Args) < 6 {
			fmt.Fprintf(os.Stderr, "Usage: mdlink insert <file> <url> <title> <section> [subsection]\n")
			os.Exit(1)
		}
		subsection := ""
		if len(os.Args) > 6 {
			subsection = os.Args[6]
		}
		cmdInsert(os.Args[2], os.Args[3], os.Args[4], os.Args[5], subsection)
	case "update":
		if len(os.Args) < 6 {
			fmt.Fprintf(os.Stderr, "Usage: mdlink update <file> <old_url> <new_url> <title>\n")
			os.Exit(1)
		}
		cmdUpdate(os.Args[2], os.Args[3], os.Args[4], os.Args[5])
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}
