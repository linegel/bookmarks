// mdlink - Markdown Link Manager CLI
// =====================================
// USAGE:
//   mdlink find <file> <url>              - Locate link
//   mdlink insert <file> <url> <title> [section] [subsection] - Add link
//   mdlink update <file> <old_url> <new_url> <title> - Replace link
//   mdlink list <file> [section]          - List all links (optional: in section)
//   mdlink --help                         - This
//
// LINK SEMANTICS:
// ===============
// Files use standard Markdown format:
//   # Section
//   ## Subsection
//   * [Title](url) - description
//
// Links are stored as list items under headings.
// Sections matched case-insensitive. Auto-created if missing.

package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Line with metadata
type Line struct {
	text  string
	level int // heading level (0 if not heading)
}

// Read file into lines
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

// Write lines back
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

// Extract heading level (0 = not heading)
func getHeadingLevel(text string) int {
	match := regexp.MustCompile(`^(#+)`).FindString(text)
	return len(match)
}

// Extract heading title
func getHeadingTitle(text string) string {
	re := regexp.MustCompile(`^#+\s+(.+)$`)
	m := re.FindStringSubmatch(text)
	if len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// Find link in lines
func findLinkInLines(lines []Line, url string) (int, string) {
	for i, line := range lines {
		if strings.Contains(line.text, "]"+url+")") ||
			strings.Contains(line.text, "]("+url) {
			return i, line.text
		}
	}
	return -1, ""
}

// Find section by name (case-insensitive)
func findSection(lines []Line, section string, level int) int {
	re := regexp.MustCompile(`(?i)^#{` + fmt.Sprintf("%d", level) + `}\s+` + regexp.QuoteMeta(section))
	for i, line := range lines {
		if re.MatchString(line.text) {
			return i
		}
	}
	return -1
}

// Next insertion point after section (before next heading same/higher level)
func findInsertionPoint(lines []Line, sectionIdx int, sectionLevel int) int {
	for i := sectionIdx + 1; i < len(lines); i++ {
		if lines[i].level > 0 && lines[i].level <= sectionLevel {
			return i
		}
	}
	return len(lines)
}

// Parse link: [Title](url) => (title, url)
func parseLink(text string) (string, string) {
	re := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	m := re.FindStringSubmatch(text)
	if len(m) > 2 {
		return m[1], m[2]
	}
	return "", ""
}

// Find command
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

// List command
func cmdList(file, section string) {
	lines, err := readFile(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var startIdx int
	if section != "" {
		startIdx = findSection(lines, section, 2)
		if startIdx < 0 {
			fmt.Printf("Section '%s' not found\n", section)
			return
		}
		startIdx++
	}

	linkRe := regexp.MustCompile(`^\s*\*\s+\[`)
	for i := startIdx; i < len(lines); i++ {
		if lines[i].level > 0 && i > startIdx {
			break
		}
		if linkRe.MatchString(lines[i].text) {
			title, url := parseLink(lines[i].text)
			fmt.Printf("[%s] %s\n", title, url)
		}
	}
}

// Insert command
func cmdInsert(file, url, title, section, subsection string) {
	lines, err := readFile(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Check exists
	if idx, _ := findLinkInLines(lines, url); idx >= 0 {
		fmt.Println("EXISTS")
		return
	}

	newLink := fmt.Sprintf("* [%s](%s)", title, url)

	// Find insertion point
	secIdx := findSection(lines, section, 2)
	if secIdx < 0 {
		// Create section
		lines = append(lines, Line{fmt.Sprintf("## %s", section), 2})
		secIdx = len(lines) - 1
	}

	insertIdx := findInsertionPoint(lines, secIdx, 2)

	// Insert
	newLines := make([]Line, len(lines)+1)
	copy(newLines, lines[:insertIdx])
	newLines[insertIdx] = Line{newLink, 0}
	copy(newLines[insertIdx+1:], lines[insertIdx:])

	if err := writeFile(file, newLines); err != nil {
		fmt.Fprintf(os.Stderr, "Write error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("INSERTED at line %d\n", insertIdx+1)
}

// Update command
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

// Help
func showHelp() {
	help := `mdlink - Markdown Link Manager

COMMANDS:
  find <file> <url>               Find link location
  list <file> [section]           List all links (optionally in section)
  insert <file> <url> <title> <section> [subsection]
                                  Add new link
  update <file> <old_url> <new_url> <title>
                                  Replace link

LINK FORMAT:
  * [Title](url)                  List item with link
  Organized under ## Section headings

EXAMPLES:
  mdlink find README.md "https://example.com"
  mdlink insert README.md "https://golang.org" "Go" "Programming"
  mdlink update README.md "http://old.com" "https://new.com" "NewTitle"
  mdlink list sections/devops.md "Kubernetes"
`
	fmt.Print(help)
}

func main() {
	flag.Parse()
	args := flag.Args()

	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		showHelp()
		return
	}

	cmd := args[0]

	switch cmd {
	case "find":
		if len(args) < 3 {
			fmt.Fprintf(os.Stderr, "Usage: mdlink find <file> <url>\n")
			os.Exit(1)
		}
		cmdFind(args[1], args[2])

	case "list":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: mdlink list <file> [section]\n")
			os.Exit(1)
		}
		section := ""
		if len(args) > 2 {
			section = args[2]
		}
		cmdList(args[1], section)

	case "insert":
		if len(args) < 5 {
			fmt.Fprintf(os.Stderr, "Usage: mdlink insert <file> <url> <title> <section> [subsection]\n")
			os.Exit(1)
		}
		subsection := ""
		if len(args) > 5 {
			subsection = args[5]
		}
		cmdInsert(args[1], args[2], args[3], args[4], subsection)

	case "update":
		if len(args) < 5 {
			fmt.Fprintf(os.Stderr, "Usage: mdlink update <file> <old_url> <new_url> <title>\n")
			os.Exit(1)
		}
		cmdUpdate(args[1], args[2], args[3], args[4])

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		showHelp()
		os.Exit(1)
	}
}
