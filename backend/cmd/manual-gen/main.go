package main

import (
	"bytes"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type DocumentSection struct {
	ID       string
	Title    string
	Filename string
	Content  string
	Headings []Heading
}

type Heading struct {
	Level int
	ID    string
	Text  string
}

func slugify(text string) string {
	re := regexp.MustCompile(`[^a-z0-9]+`)
	slug := strings.ToLower(strings.TrimSpace(text))
	slug = re.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "section"
	}
	return slug
}

func parseMarkdownHeadings(content string) (string, []Heading) {
	lines := strings.Split(content, "\n")
	var outLines []string
	var headings []Heading
	inCodeBlock := false

	headingRegex := regexp.MustCompile(`^(#{1,6})\s+(.+)$`)

	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inCodeBlock = !inCodeBlock
			outLines = append(outLines, line)
			continue
		}

		if !inCodeBlock && headingRegex.MatchString(line) {
			matches := headingRegex.FindStringSubmatch(line)
			level := len(matches[1])
			rawTitle := matches[2]
			// Strip links if any in heading
			cleanTitle := regexp.MustCompile(`\[(.*?)\]\(.*?\)`).ReplaceAllString(rawTitle, "$1")
			cleanTitle = strings.TrimSpace(cleanTitle)
			id := slugify(cleanTitle)

			headings = append(headings, Heading{
				Level: level,
				ID:    id,
				Text:  cleanTitle,
			})

			outLines = append(outLines, fmt.Sprintf(`<h%d id="%s" class="doc-heading">%s <a href="#%s" class="anchor-link" aria-label="Link to this section">#</a></h%d>`,
				level, id, html.EscapeString(cleanTitle), id, level))
		} else {
			outLines = append(outLines, line)
		}
	}

	return strings.Join(outLines, "\n"), headings
}

func markdownToHTML(md string) string {
	md, _ = parseMarkdownHeadings(md)

	var buf bytes.Buffer
	lines := strings.Split(md, "\n")
	inCodeBlock := false
	codeLang := ""
	var codeBuffer []string
	inList := false
	inTable := false
	var tableRows []string

	flushTable := func() {
		if !inTable || len(tableRows) == 0 {
			return
		}
		buf.WriteString("<div class=\"table-wrapper\"><table>\n")
		isHeader := true
		for _, row := range tableRows {
			cells := strings.Split(strings.Trim(row, "|"), "|")
			if len(cells) == 0 || (len(cells) > 0 && strings.Contains(cells[0], "---")) {
				isHeader = false
				continue
			}
			buf.WriteString("<tr>")
			tag := "td"
			if isHeader {
				tag = "th"
			}
			for _, cell := range cells {
				buf.WriteString(fmt.Sprintf("<%s>%s</%s>", tag, strings.TrimSpace(inlineFormat(cell)), tag))
			}
			buf.WriteString("</tr>\n")
			if isHeader {
				isHeader = false
			}
		}
		buf.WriteString("</table></div>\n")
		tableRows = nil
		inTable = false
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Code blocks
		if strings.HasPrefix(trimmed, "```") {
			flushTable()
			if inList {
				buf.WriteString("</ul>\n")
				inList = false
			}

			if inCodeBlock {
				// End code block
				codeContent := strings.Join(codeBuffer, "\n")
				buf.WriteString("<div class=\"code-container\">\n")
				if codeLang != "" {
					buf.WriteString(fmt.Sprintf("<div class=\"code-header\"><span class=\"code-lang\">%s</span><button class=\"copy-btn\" onclick=\"copyCode(this)\" title=\"Copy code to clipboard\">Copy</button></div>\n", html.EscapeString(codeLang)))
				} else {
					buf.WriteString("<div class=\"code-header\"><span class=\"code-lang\">text</span><button class=\"copy-btn\" onclick=\"copyCode(this)\" title=\"Copy code to clipboard\">Copy</button></div>\n")
				}
				buf.WriteString(fmt.Sprintf("<pre><code>%s</code></pre>\n", html.EscapeString(codeContent)))
				buf.WriteString("</div>\n")
				codeBuffer = nil
				inCodeBlock = false
				codeLang = ""
			} else {
				inCodeBlock = true
				codeLang = strings.TrimPrefix(trimmed, "```")
				codeBuffer = nil
			}
			continue
		}

		if inCodeBlock {
			codeBuffer = append(codeBuffer, line)
			continue
		}

		// Table rows
		if strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|") {
			if inList {
				buf.WriteString("</ul>\n")
				inList = false
			}
			inTable = true
			tableRows = append(tableRows, trimmed)
			continue
		} else if inTable {
			flushTable()
		}

		// Pre-formatted HTML headings
		if strings.HasPrefix(line, "<h") && strings.Contains(line, "class=\"doc-heading\"") {
			if inList {
				buf.WriteString("</ul>\n")
				inList = false
			}
			buf.WriteString(line + "\n")
			continue
		}

		// Lists
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			if !inList {
				buf.WriteString("<ul>\n")
				inList = true
			}
			itemText := strings.TrimPrefix(trimmed, "- ")
			itemText = strings.TrimPrefix(itemText, "* ")
			buf.WriteString(fmt.Sprintf("<li>%s</li>\n", inlineFormat(itemText)))
			continue
		} else if inList && trimmed == "" {
			buf.WriteString("</ul>\n")
			inList = false
			continue
		}

		// Horizontal rule
		if trimmed == "---" || trimmed == "***" {
			buf.WriteString("<hr class=\"doc-divider\" />\n")
			continue
		}

		// Paragraph
		if trimmed != "" {
			buf.WriteString(fmt.Sprintf("<p>%s</p>\n", inlineFormat(line)))
		}
	}

	flushTable()
	if inList {
		buf.WriteString("</ul>\n")
	}

	return buf.String()
}

func inlineFormat(text string) string {
	// Inline code: `code`
	codeRe := regexp.MustCompile("`([^`]+)`")
	text = codeRe.ReplaceAllStringFunc(text, func(m string) string {
		inner := m[1 : len(m)-1]
		return fmt.Sprintf("<code>%s</code>", html.EscapeString(inner))
	})

	// Links: [text](url)
	linkRe := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	text = linkRe.ReplaceAllStringFunc(text, func(m string) string {
		parts := linkRe.FindStringSubmatch(m)
		linkText := parts[1]
		linkURL := parts[2]
		// Convert relative .md link to section anchor if applicable
		if strings.HasSuffix(linkURL, ".md") {
			linkURL = "#" + strings.TrimSuffix(filepath.Base(linkURL), ".md")
		}
		return fmt.Sprintf(`<a href="%s">%s</a>`, html.EscapeString(linkURL), html.EscapeString(linkText))
	})

	// Bold: **text**
	boldRe := regexp.MustCompile(`\*\*([^*]+)\*\*`)
	text = boldRe.ReplaceAllString(text, "<strong>$1</strong>")

	// Italic: *text*
	italicRe := regexp.MustCompile(`\*([^*]+)\*`)
	text = italicRe.ReplaceAllString(text, "<em>$1</em>")

	return text
}

func buildHTMLPage(docs []DocumentSection) string {
	var sidebarNav bytes.Buffer
	var contentArea bytes.Buffer

	for _, doc := range docs {
		sidebarNav.WriteString(fmt.Sprintf("<div class=\"nav-group\"><a href=\"#%s\" class=\"nav-title\">%s</a>\n", doc.ID, html.EscapeString(doc.Title)))
		if len(doc.Headings) > 0 {
			sidebarNav.WriteString("<ul class=\"nav-subitems\">\n")
			for _, h := range doc.Headings {
				if h.Level == 2 || h.Level == 3 {
					sidebarNav.WriteString(fmt.Sprintf("<li><a href=\"#%s\" class=\"nav-link level-%d\">%s</a></li>\n",
						h.ID, h.Level, html.EscapeString(h.Text)))
				}
			}
			sidebarNav.WriteString("</ul>\n")
		}
		sidebarNav.WriteString("</div>\n")

		contentArea.WriteString(fmt.Sprintf("<section id=\"%s\" class=\"doc-section\">\n", doc.ID))
		contentArea.WriteString(markdownToHTML(doc.Content))
		contentArea.WriteString("</section>\n")
	}

	htmlTemplate := `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>CoreRP Platform Manual & Reference</title>
  <style>
    :root {
      --bg: #ffffff;
      --fg: #1a1a24;
      --sidebar-bg: #f8f9fa;
      --border: #e2e8f0;
      --accent: #2563eb;
      --accent-hover: #1d4ed8;
      --code-bg: #f1f5f9;
      --code-block-bg: #0f172a;
      --code-block-fg: #f8fafc;
      --table-header: #f1f5f9;
      --card-bg: #ffffff;
      --nav-hover: #e2e8f0;
    }
    @media (prefers-color-scheme: dark) {
      :root {
        --bg: #0b0f19;
        --fg: #f1f5f9;
        --sidebar-bg: #0f172a;
        --border: #1e293b;
        --accent: #3b82f6;
        --accent-hover: #60a5fa;
        --code-bg: #1e293b;
        --code-block-bg: #030712;
        --code-block-fg: #f8fafc;
        --table-header: #1e293b;
        --card-bg: #111827;
        --nav-hover: #1e293b;
      }
    }
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
      background: var(--bg);
      color: var(--fg);
      line-height: 1.6;
      display: flex;
      min-height: 100vh;
    }
    /* Sidebar */
    #sidebar {
      width: 300px;
      flex-shrink: 0;
      background: var(--sidebar-bg);
      border-right: 1px solid var(--border);
      position: sticky;
      top: 0;
      height: 100vh;
      overflow-y: auto;
      padding: 1.5rem 1rem;
      display: flex;
      flex-direction: column;
      gap: 1rem;
    }
    .brand {
      font-size: 1.25rem;
      font-weight: 700;
      color: var(--accent);
      text-decoration: none;
      display: flex;
      align-items: center;
      gap: 0.5rem;
    }
    .search-box {
      position: relative;
    }
    #search-input {
      width: 100%;
      padding: 0.5rem 0.75rem;
      border: 1px solid var(--border);
      border-radius: 6px;
      background: var(--bg);
      color: var(--fg);
      font-size: 0.875rem;
      outline: none;
    }
    #search-input:focus {
      border-color: var(--accent);
      box-shadow: 0 0 0 2px rgba(37,99,235,0.2);
    }
    .nav-container {
      display: flex;
      flex-direction: column;
      gap: 0.75rem;
    }
    .nav-group {
      display: flex;
      flex-direction: column;
      gap: 0.25rem;
    }
    .nav-title {
      font-weight: 600;
      font-size: 0.95rem;
      color: var(--fg);
      text-decoration: none;
      padding: 0.25rem 0.5rem;
      border-radius: 4px;
    }
    .nav-title:hover {
      background: var(--nav-hover);
      color: var(--accent);
    }
    .nav-subitems {
      list-style: none;
      padding-left: 0.75rem;
      display: flex;
      flex-direction: column;
      gap: 0.15rem;
    }
    .nav-link {
      font-size: 0.85rem;
      color: var(--fg);
      opacity: 0.8;
      text-decoration: none;
      display: block;
      padding: 0.2rem 0.5rem;
      border-radius: 4px;
    }
    .nav-link:hover {
      opacity: 1;
      background: var(--nav-hover);
      color: var(--accent);
    }
    .nav-link.level-3 { padding-left: 1rem; font-size: 0.8rem; }
    /* Content */
    #content {
      flex: 1;
      max-width: 900px;
      padding: 2.5rem 3rem;
      overflow-y: auto;
    }
    .doc-section {
      margin-bottom: 4rem;
    }
    h1, h2, h3, h4, h5, h6 {
      color: var(--fg);
      margin-top: 1.75rem;
      margin-bottom: 0.75rem;
      font-weight: 600;
      position: relative;
    }
    h1 { font-size: 2rem; border-bottom: 1px solid var(--border); padding-bottom: 0.5rem; }
    h2 { font-size: 1.5rem; border-bottom: 1px solid var(--border); padding-bottom: 0.25rem; }
    h3 { font-size: 1.25rem; }
    .anchor-link {
      opacity: 0;
      color: var(--accent);
      text-decoration: none;
      margin-left: 0.4rem;
      transition: opacity 0.15s ease-in-out;
    }
    h1:hover .anchor-link, h2:hover .anchor-link, h3:hover .anchor-link { opacity: 1; }
    p { margin-bottom: 1rem; }
    ul, ol { margin-bottom: 1rem; padding-left: 1.5rem; }
    li { margin-bottom: 0.25rem; }
    code {
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
      background: var(--code-bg);
      padding: 0.2rem 0.4rem;
      border-radius: 4px;
      font-size: 0.9em;
    }
    .code-container {
      background: var(--code-block-bg);
      color: var(--code-block-fg);
      border-radius: 8px;
      margin: 1.25rem 0;
      overflow: hidden;
      border: 1px solid var(--border);
    }
    .code-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      padding: 0.5rem 1rem;
      background: rgba(255,255,255,0.05);
      border-bottom: 1px solid rgba(255,255,255,0.1);
      font-size: 0.75rem;
      text-transform: uppercase;
      letter-spacing: 0.05em;
    }
    .copy-btn {
      background: transparent;
      border: 1px solid rgba(255,255,255,0.2);
      color: #f8fafc;
      padding: 0.25rem 0.6rem;
      border-radius: 4px;
      cursor: pointer;
      font-size: 0.75rem;
      transition: background 0.15s;
    }
    .copy-btn:hover { background: rgba(255,255,255,0.15); }
    .code-container pre {
      padding: 1rem;
      overflow-x: auto;
      font-size: 0.875rem;
      line-height: 1.5;
    }
    .code-container pre code {
      background: transparent;
      padding: 0;
      color: inherit;
    }
    .table-wrapper {
      overflow-x: auto;
      margin: 1.5rem 0;
    }
    table {
      width: 100%;
      border-collapse: collapse;
      text-align: left;
      font-size: 0.9rem;
    }
    th, td {
      padding: 0.75rem 1rem;
      border: 1px solid var(--border);
    }
    th { background: var(--table-header); font-weight: 600; }
    .doc-divider {
      border: none;
      border-top: 1px solid var(--border);
      margin: 2.5rem 0;
    }
    /* Mobile responsive */
    .mobile-menu-btn {
      display: none;
      position: fixed;
      bottom: 1rem;
      right: 1rem;
      background: var(--accent);
      color: white;
      border: none;
      border-radius: 50%;
      width: 48px;
      height: 48px;
      font-size: 1.25rem;
      cursor: pointer;
      box-shadow: 0 4px 6px rgba(0,0,0,0.2);
      z-index: 100;
    }
    @media (max-width: 768px) {
      body { flex-direction: column; }
      #sidebar {
        position: fixed;
        left: -320px;
        top: 0;
        z-index: 99;
        transition: left 0.3s ease;
        box-shadow: 2px 0 10px rgba(0,0,0,0.3);
      }
      #sidebar.open { left: 0; }
      .mobile-menu-btn { display: block; }
      #content { padding: 1.5rem; max-width: 100%; }
    }
    /* Search highlight */
    .search-match {
      background: #fef08a;
      color: #000;
      padding: 0 2px;
      border-radius: 2px;
    }
  </style>
</head>
<body>
  <button class="mobile-menu-btn" onclick="toggleSidebar()" aria-label="Toggle navigation">☰</button>
  <nav id="sidebar">
    <a href="#" class="brand">CoreRP Manual</a>
    <div class="search-box">
      <input type="text" id="search-input" placeholder="Search manual (Press '/' to focus)" aria-label="Search documentation" />
    </div>
    <div class="nav-container" id="nav-container">
` + sidebarNav.String() + `
    </div>
  </nav>

  <main id="content">
` + contentArea.String() + `
  </main>

  <script>
    function copyCode(btn) {
      const container = btn.closest('.code-container');
      const code = container.querySelector('code').innerText;
      navigator.clipboard.writeText(code).then(() => {
        const orig = btn.innerText;
        btn.innerText = 'Copied!';
        setTimeout(() => btn.innerText = orig, 1800);
      });
    }

    function toggleSidebar() {
      const sidebar = document.getElementById('sidebar');
      sidebar.classList.toggle('open');
    }

    // Keyboard shortcuts: '/' to search, 'Esc' to exit search
    window.addEventListener('keydown', (e) => {
      if (e.key === '/' && document.activeElement.id !== 'search-input') {
        e.preventDefault();
        document.getElementById('search-input').focus();
      } else if (e.key === 'Escape' && document.activeElement.id === 'search-input') {
        const input = document.getElementById('search-input');
        input.value = '';
        filterNav('');
        input.blur();
      }
    });

    const searchInput = document.getElementById('search-input');
    searchInput.addEventListener('input', (e) => {
      filterNav(e.target.value.toLowerCase().trim());
    });

    function filterNav(query) {
      const navGroups = document.querySelectorAll('.nav-group');
      navGroups.forEach(group => {
        let hasMatch = false;
        const groupTitle = group.querySelector('.nav-title');
        const links = group.querySelectorAll('.nav-link');

        if (groupTitle && groupTitle.innerText.toLowerCase().includes(query)) {
          hasMatch = true;
        }

        links.forEach(link => {
          if (query === '' || link.innerText.toLowerCase().includes(query)) {
            link.style.display = 'block';
            hasMatch = true;
          } else {
            link.style.display = 'none';
          }
        });

        if (query === '' || hasMatch) {
          group.style.display = 'flex';
        } else {
          group.style.display = 'none';
        }
      });
    }
  </script>
</body>
</html>`

	return htmlTemplate
}

func main() {
	manualDir := "docs/manual"
	if _, err := os.Stat(filepath.Join(manualDir, "index.md")); err != nil {
		if _, err := os.Stat(filepath.Join("..", manualDir, "index.md")); err == nil {
			manualDir = filepath.Join("..", manualDir)
		}
	}
	if len(os.Args) > 1 {
		manualDir = os.Args[1]
	}

	distDir := filepath.Join(manualDir, "dist")
	if len(os.Args) > 2 {
		distDir = os.Args[2]
	}

	if err := os.MkdirAll(distDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create dist dir: %v\n", err)
		os.Exit(1)
	}

	docFiles := []struct {
		id       string
		title    string
		filename string
	}{
		{"index", "Overview & Quickstart", "index.md"},
		{"player", "Player Guide", "player.md"},
		{"creator", "Creator Guide", "creator.md"},
		{"external-agent", "External Agent & MCP", "external-agent.md"},
		{"maintainer", "Maintainer Architecture & Ops", "maintainer.md"},
	}

	var sections []DocumentSection
	for _, df := range docFiles {
		path := filepath.Join(manualDir, df.filename)
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to read %s: %v\n", path, err)
			os.Exit(1)
		}
		_, headings := parseMarkdownHeadings(string(data))
		sections = append(sections, DocumentSection{
			ID:       df.id,
			Title:    df.title,
			Filename: df.filename,
			Content:  string(data),
			Headings: headings,
		})
	}

	renderedHTML := buildHTMLPage(sections)
	outputPath := filepath.Join(distDir, "index.html")
	if err := os.WriteFile(outputPath, []byte(renderedHTML), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write %s: %v\n", outputPath, err)
		os.Exit(1)
	}

	fmt.Printf("Successfully generated static HTML manual at: %s (%d bytes)\n", outputPath, len(renderedHTML))
}
