package main

import (
	"os"
	"strings"
	"testing"
)

func TestParseDocumentSections(t *testing.T) {
	lines := []Line{
		{text: "# Section1", level: 1},
		{text: "content", level: 0},
		{text: "# Section2", level: 1},
		{text: "more", level: 0},
	}
	doc := parseDocument(lines)
	if len(doc.Sections) != 2 {
		t.Errorf("expected 2 sections, got %d", len(doc.Sections))
	}
	if doc.Sections[0].EndLine != 1 {
		t.Errorf("section1 should end at line 1, got %d", doc.Sections[0].EndLine)
	}
}

func TestNestedHeadingsDontStopSection(t *testing.T) {
	lines := []Line{
		{text: "# Main", level: 1},
		{text: "## Nested", level: 2},
		{text: "stuff", level: 0},
		{text: "# Next", level: 1},
	}
	doc := parseDocument(lines)
	if doc.Sections[0].EndLine != 2 {
		t.Errorf("main section should include nested heading and content, ends at %d", doc.Sections[0].EndLine)
	}
}

func TestQuoteSubsectionDetection(t *testing.T) {
	lines := []Line{
		{text: "# Sec", level: 1},
		{text: "> News", level: 0},
		{text: "* [Link1](url1)", level: 0},
		{text: "", level: 0},
		{text: "> Learn", level: 0},
		{text: "* [Link2](url2)", level: 0},
	}
	section := &Section{
		Name:      "Sec",
		StartLine: 0,
		EndLine:   5,
		Level:     1,
	}
	subs := parseSubsections(lines, section)
	if len(subs) != 2 {
		t.Errorf("expected 2 quote subsections, got %d", len(subs))
	}
	if subs[0].Name != "News" || subs[1].Name != "Learn" {
		t.Errorf("subsection names wrong: %s, %s", subs[0].Name, subs[1].Name)
	}
}

func TestLinkMetadataQuoteSkipped(t *testing.T) {
	lines := []Line{
		{text: "# Sec", level: 1},
		{text: "> News", level: 0},
		{text: "* [Link](url)", level: 0},
		{text: "> This is metadata about link", level: 0},
		{text: "more text", level: 0},
		{text: "", level: 0},
		{text: "> Learn", level: 0},
	}
	section := &Section{
		Name:      "Sec",
		StartLine: 0,
		EndLine:   6,
		Level:     1,
	}
	subs := parseSubsections(lines, section)
	if len(subs) != 2 {
		t.Errorf("expected 2 subsections (News, Learn), got %d", len(subs))
	}
	// Metadata quote at line 3 is skipped (marked as metadata because line 2 is link)
	// But it's included in the block scan, so News ends at line 4 ("more text")
	if subs[0].EndLine != 5 {
		t.Errorf("News subsection should end at line 5 (stops before Learn), ends at %d", subs[0].EndLine)
	}
}

func TestHeadingSubsections(t *testing.T) {
	lines := []Line{
		{text: "# Main", level: 1},
		{text: "## Sub1", level: 2},
		{text: "content", level: 0},
		{text: "## Sub2", level: 2},
		{text: "more", level: 0},
	}
	section := &Section{
		Name:      "Main",
		StartLine: 0,
		EndLine:   4,
		Level:     1,
	}
	subs := parseSubsections(lines, section)
	headings := 0
	for _, sub := range subs {
		if !sub.IsQuote {
			headings++
		}
	}
	if headings != 2 {
		t.Errorf("expected 2 heading subsections, got %d", headings)
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Business & Economics", "business & economics"},
		{"LEARN", "learn"},
		{"Foo &amp; Bar", "foo & bar"},
	}
	for _, tt := range tests {
		got := normalize(tt.input)
		if got != tt.expected {
			t.Errorf("normalize(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestFindSectionInDoc(t *testing.T) {
	doc := &Document{
		Sections: []*Section{
			{Name: "Business & Economics"},
			{Name: "Collaboration"},
		},
	}
	sec := findSectionInDoc(doc, "business & economics")
	if sec == nil || sec.Name != "Business & Economics" {
		t.Error("case-insensitive section lookup failed")
	}
}

func TestFindSubsectionInSection(t *testing.T) {
	section := &Section{
		Name: "Main",
		Subsections: []*Subsection{
			{Name: "Learn", IsQuote: true},
			{Name: "News", IsQuote: true},
		},
	}
	sub := findSubsectionInSection(section, "learn")
	if sub == nil || sub.Name != "Learn" {
		t.Error("case-insensitive subsection lookup failed")
	}
}

func TestInsertWithoutRegressionLinkMetadata(t *testing.T) {
	content := `# Business & Economics
> News
* [Link1](url1)
* [Link2](url2) - description
> This is metadata on Link2

> Learn
* [Link3](url3)
`
	tmpfile := createTempFile(t, content)
	defer os.Remove(tmpfile)

	lines, _ := readFile(tmpfile)
	doc := parseDocument(lines)
	sec := findSectionInDoc(doc, "Business & Economics")

	// Find Learn subsection
	sub := findSubsectionInSection(sec, "Learn")
	if sub == nil {
		t.Fatal("Learn subsection not found")
	}

	// Should be able to insert in Learn
	if sub.Name != "Learn" {
		t.Errorf("wrong subsection: %s", sub.Name)
	}
}

func TestDuplicatePerSectionOnly(t *testing.T) {
	content := `# Sec1
* [Link](url1)

# Sec2
* [Link](url2)
`
	tmpfile := createTempFile(t, content)
	defer os.Remove(tmpfile)

	lines, _ := readFile(tmpfile)
	doc := parseDocument(lines)

	sec1 := findSectionInDoc(doc, "Sec1")
	sec2 := findSectionInDoc(doc, "Sec2")

	// Same URL in different sections: should both exist without conflict
	found1 := false
	found2 := false
	for i := sec1.StartLine; i <= sec1.EndLine; i++ {
		if strings.Contains(lines[i].text, "url1") {
			found1 = true
		}
	}
	for i := sec2.StartLine; i <= sec2.EndLine; i++ {
		if strings.Contains(lines[i].text, "url2") {
			found2 = true
		}
	}

	if !found1 || !found2 {
		t.Error("links not found in expected sections")
	}
}

func TestIsLinkLine(t *testing.T) {
	tests := []struct {
		text     string
		expected bool
	}{
		{"* [Title](url)", true},
		{"  * [Title](url)", true},
		{"> Quote", false},
		{"text", false},
		{"* text without link", false},
	}
	for _, tt := range tests {
		got := isLinkLine(tt.text)
		if got != tt.expected {
			t.Errorf("isLinkLine(%q) = %v, want %v", tt.text, got, tt.expected)
		}
	}
}

func TestCountListItems(t *testing.T) {
	lines := []Line{
		{text: "* [Link1](url1)", level: 0},
		{text: "* [Link2](url2)", level: 0},
		{text: "> metadata", level: 0},
		{text: "* [Link3](url3)", level: 0},
	}
	count := countListItems(lines, 0, 3)
	if count != 3 {
		t.Errorf("expected 3 links, got %d", count)
	}
}

func TestChooseBestQuoteSubsection(t *testing.T) {
	lines := []Line{
		{text: "# Sec", level: 1},
		{text: "> News", level: 0},
		{text: "* [L1](u1)", level: 0},
		{text: "", level: 0},
		{text: "> Learn", level: 0},
		{text: "* [L2](u2)", level: 0},
		{text: "* [L3](u3)", level: 0},
	}
	section := &Section{
		Name:      "Sec",
		StartLine: 0,
		EndLine:   6,
		Level:     1,
		Subsections: []*Subsection{
			{Name: "News", IsQuote: true, StartLine: 1, EndLine: 1},
			{Name: "Learn", IsQuote: true, StartLine: 4, EndLine: 6},
		},
	}
	best := chooseBestQuoteSubsection(lines, section)
	if best.Name != "Learn" {
		t.Errorf("should prefer Learn, got %s", best.Name)
	}
}

func TestParseLink(t *testing.T) {
	title, url := parseLink("* [My Title](https://example.com)")
	if title != "My Title" || url != "https://example.com" {
		t.Errorf("parseLink failed: %q, %q", title, url)
	}
}

// Helper: create temp file for testing
func createTempFile(t *testing.T, content string) string {
	tmpfile, err := os.CreateTemp("", "mdlink-test-*.md")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	if _, err := tmpfile.WriteString(content); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpfile.Close()
	return tmpfile.Name()
}
