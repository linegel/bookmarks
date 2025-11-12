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
// Notes:
// - Sections matched case-insensitive, ampersand-safe.
// - Arrow-style subsections recognized ONLY as lines starting with "-> ".
// - Internal document model ensures insertions never cross section boundaries.
// - Top-level sections stop at ANY deeper heading level, not just same level.

package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

const version = "0.8.1"

type Line struct {
	text  string
	level int // heading level (0 if not heading)
}

// Section represents a top-level section (content before any nested heading)
type Section struct {
	Name       string
	StartLine  int
	EndLine    int
	Level      int
	Subsections []*Subsection
}

// Subsection represents an arrow-style subsection with bounds
type Subsection struct {
	Name      string
	StartLine int
	EndLine   int
	IsArrow   bool
}

// Document represents the parsed markdown structure
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

// parseDocument builds internal model of top-level sections only
// Top-level sections are defined as: content that stops when ANY heading is encountered
// (not just same-level headings)
func parseDocument(lines []Line) *Document {
	doc := &Document{Lines: lines, Sections: []*Section{}}

	for i := 0; i < len(lines); i++ {
		if lines[i].level == 0 {
			continue
		}

		title := getHeadingTitle(lines[i].text)
		section := &Section{
			Name:      title,
			StartLine: i,
			EndLine:   len(lines) - 1,
			Level:     lines[i].level,
		}

		// Find section end: next heading of ANY kind stops this section
		for j := i + 1; j < len(lines); j++ {
			if lines[j].level > 0 {
				section.EndLine = j - 1
				break
			}
		}

		// Parse arrow subsections in the section (they don't include nested headings)
		section.Subsections = parseArrowSubsections(lines, i, section.EndLine, section.Level)

		doc.Sections = append(doc.Sections, section)
		i = section.EndLine
	}

	return doc
}

// parseArrowSubsections finds arrow-style subsections before any nested heading
func parseArrowSubsections(lines []Line, sectionStart int, sectionEnd int, sectionLevel int) []*Subsection {
	var subs []*Subsection

	// Scan from sectionStart+1 until we hit ANY nested heading (level > sectionLevel)
	for i := sectionStart + 1; i <= sectionEnd; i++ {
		if lines[i].level > sectionLevel {
			// Nested heading: stop direct content scan
			break
		}

		trim := strings.TrimSpace(lines[i].text)
		if strings.HasPrefix(trim, "-> ") {
			label := strings.TrimSpace(strings.TrimPrefix(trim, "-> "))
			sub := &Subsection{
				Name:      label,
				StartLine: i,
				IsArrow:   true,
			}

			// Find end of arrow block: next arrow, nested heading, or section end
			for j := i + 1; j <= sectionEnd; j++ {
				if lines[j].level > sectionLevel {
					sub.EndLine = j - 1
					break
				}
				trim := strings.TrimSpace(lines[j].text)
				if strings.HasPrefix(trim, "-> ") {
					sub.EndLine = j - 1
					break
				}
				if j == sectionEnd {
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

// findSectionInDoc returns section by name from document
func findSectionInDoc(doc *Document, section string) *Section {
	normSection := normalize(section)
	for _, sec := range doc.Sections {
		if normalize(sec.Name) == normSection {
			return sec
		}
	}
	return nil
}

// findSubsectionInSection returns subsection by name within section
func findSubsectionInSection(section *Section, subsection string) *Subsection {
	normSub := normalize(subsection)
	for _, sub := range section.Subsections {
		if normalize(sub.Name) == normSub {
			return sub
		}
	}
	return nil
}

// countListItems in a range
func countListItems(lines []Line, startIdx int, endIdx int) int {
	count := 0
	for i := startIdx; i <= endIdx && i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i].text), "* ") {
			count++
		}
	}
	return count
}

// chooseBestArrowSubsection: prefer Learn, else most items
func chooseBestArrowSubsection(lines []Line, section *Section) *Subsection {
	if len(section.Subsections) == 0 {
		return nil
	}

	// First pass: look for Learn
	for _, sub := range section.Subsections {
		if normalize(sub.Name) == normalize("Learn") {
			return sub
		}
	}

	// Fallback: most items
	best := section.Subsections[len(section.Subsections)-1]
	bestCount := -1

	for _, sub := range section.Subsections {
		count := countListItems(lines, sub.StartLine+1, sub.EndLine)
		if count > bestCount || (count == bestCount && sub.StartLine >= best.StartLine) {
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
	lines, err := readFile(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
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
	lines, err := readFile(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	doc := parseDocument(lines)

	if section != "" {
		sec := findSectionInDoc(doc, section)
		if sec == nil {
			fmt.Printf("Section '%s' not found\n", section)
			return
		}
		fmt.Printf("# %s (lines %d-%d)\n", sec.Name, sec.StartLine+1, sec.EndLine+1)
		for _, sub := range sec.Subsections {
			fmt.Printf("  -> %s (lines %d-%d)\n", sub.Name, sub.StartLine+1, sub.EndLine+1)
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
				fmt.Printf("  -> %s (lines %d-%d)\n", sub.Name, sub.StartLine+1, sub.EndLine+1)
			}
		}
	}
}

func cmdInsert(file, url, title, section, subsection string) {
	lines, err := readFile(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if idx, _ := findLinkInLines(lines, url); idx >= 0 {
		fmt.Println("EXISTS")
		return
	}

	doc := parseDocument(lines)
	sec := findSectionInDoc(doc, section)
	if sec == nil {
		fmt.Printf("Section '%s' not found\n", section)
		os.Exit(1)
	}

	newLink := fmt.Sprintf("* [%s](%s)", title, url)
	var insertIdx int

	if subsection != "" {
		// Explicit subsection requested
		sub := findSubsectionInSection(sec, subsection)
		if sub == nil {
			fmt.Printf("Subsection '%s' not found under '%s'\n", subsection, section)
			os.Exit(1)
		}
		insertIdx = sub.EndLine + 1
	} else {
		// No subsection: use best arrow subsection or section end
		bestSub := chooseBestArrowSubsection(lines, sec)
		if bestSub != nil {
			insertIdx = bestSub.EndLine + 1
		} else {
			insertIdx = sec.EndLine + 1
		}
	}

	// Bounds check: ensure we stay within section
	if insertIdx > sec.EndLine+1 {
		insertIdx = sec.EndLine + 1
	}

	// Build new lines with spacing
	newLines := make([]Line, 0, len(lines)+2)
	newLines = append(newLines, lines[:insertIdx]...)
	newLines = append(newLines, Line{newLink, 0})

	if insertIdx < len(lines) {
		if strings.TrimSpace(lines[insertIdx].text) != "" {
			newLines = append(newLines, Line{"", 0})
		}
	} else {
		newLines = append(newLines, Line{"", 0})
	}
	newLines = append(newLines, lines[insertIdx:]...)

	if err := writeFile(file, newLines); err != nil {
		fmt.Fprintf(os.Stderr, "Write error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("INSERTED at line %d\n", insertIdx+1)
}

func cmdUpdate(file, oldURL, newURL, title string) {
	lines, err := readFile(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	idx, _ := findLinkInLines(lines, oldURL)
	if idx < 0 {
		fmt.Println("NOT_FOUND")
		return
	}
	newLink := fmt.Sprintf("* [%s](%s)", title, newURL)
	lines[idx].text = newLink
	if err := writeFile(file, lines); err != nil {
		fmt.Fprintf(os.Stderr, "Write error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("UPDATED line %d\n", idx+1)
}

func showHelp() {
	help := `mdlink - Markdown Link Manager

COMMANDS:
  find <file> <url>
  list <file> [section]
  insert <file> <url> <title> <section> [subsection]
  update <file> <old_url> <new_url> <title>

Arrow subsections: "-> Learn", "-> News" supported.
Section boundaries stop at ANY heading level.
`
	fmt.Print(help)
}

func main() {
	if len(os.Args) < 2 {
		showHelp()
		return
	}
	cmd := os.Args[1]
	switch cmd {
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
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		showHelp()
		os.Exit(1)
	}
}
