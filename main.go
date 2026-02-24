package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/cors"
)

// Job represents a conversion job
type Job struct {
	ID         string
	InputPath  string
	FromFmt    string
	ToFmt      string
	Content    string
	IsFile     bool
	ResultChan chan Result
}

// Result represents the result of a conversion job
type Result struct {
	OutputPath string
	Err        error
}

// JobStatus represents the status of a job
type JobStatus string

const (
	StatusQueued     JobStatus = "queued"
	StatusProcessing JobStatus = "processing"
	StatusDone       JobStatus = "done"
	StatusFailed     JobStatus = "failed"
)

// JobEntry represents a stored job entry
type JobEntry struct {
	Status     JobStatus
	OutputPath string
	Error      string
	CreatedAt  time.Time
}

// JobStore holds jobs in memory with thread-safe access
type JobStore struct {
	sync.RWMutex
	jobs map[string]*JobEntry
}

var (
	jobQueue = make(chan Job, 256)
	jobStore = JobStore{jobs: make(map[string]*JobEntry)}
)

// Format extension mapping
var formatExtensions = map[string]string{
	"markdown":  ".md",
	"html":      ".html",
	"pdf":       ".pdf",
	"docx":      ".docx",
	"odt":       ".odt",
	"rst":       ".rst",
	"latex":     ".tex",
	"plain":     ".txt",
	"mediawiki": ".wiki",
	"epub":      ".epub",
	"json":      ".json",
	"org":       ".org",
	"asciidoc":  ".adoc",
	"csv":       ".csv",
	"rtf":       ".rtf",
	"textile":   ".textile",
	"docbook":   ".xml",
	"jira":      ".txt",
	"ipynb":     ".ipynb",
	"opml":      ".opml",
	"fb2":       ".fb2",
	"vimwiki":   ".wiki",
	"twiki":     ".txt",
	"tikiwiki":  ".txt",
	"creole":    ".txt",
	"gfm":       ".md",
	"pptx":      ".pptx",
}

// Extension to format mapping (for auto-detection)
var extensionFormats = map[string]string{
	".md":        "markdown",
	".markdown":  "markdown",
	".html":      "html",
	".htm":       "html",
	".docx":      "docx",
	".odt":       "odt",
	".rst":       "rst",
	".tex":       "latex",
	".txt":       "plain",
	".wiki":      "mediawiki",
	".epub":      "epub",
	".json":      "json",
	".org":       "org",
	".adoc":      "asciidoc",
	".csv":       "csv",
	".rtf":       "rtf",
	".textile":   "textile",
	".ipynb":     "ipynb",
	".opml":      "opml",
	".fb2":       "fb2",
	".pptx":      "pptx",
	".pdf":       "pdf",
}

// INPUT formats - what Pandoc can read from (high-demand first)
var inputFormats = []string{
	// High-demand input formats
	"markdown", "html", "docx", "gfm", "rst",
	"latex", "odt", "plain", "epub", "mediawiki",
	// Additional input formats
	"org", "ipynb", "csv", "json", "rtf",
	"textile", "docbook", "jira", "opml", "fb2",
	"vimwiki", "twiki", "tikiwiki", "creole",
}

// OUTPUT formats - what Pandoc can write to (high-demand first)
var outputFormats = []string{
	// High-demand output formats
	"markdown", "html", "pdf", "docx", "gfm",
	"pptx", "rst", "latex", "odt", "plain",
	// Additional output formats
	"epub", "mediawiki", "json", "org", "asciidoc",
	"rtf", "textile", "docbook", "jira", "ipynb",
	"opml", "fb2", "vimwiki",
}

// For backward compatibility, keep supportedFormats as all unique formats
var supportedFormats = []string{
	"markdown", "html", "docx", "gfm", "pdf", "pptx",
	"rst", "latex", "odt", "plain", "epub", "mediawiki",
	"json", "org", "asciidoc", "csv", "rtf", "textile",
	"docbook", "jira", "ipynb", "opml", "fb2", "vimwiki",
	"twiki", "tikiwiki", "creole",
}

// SEO landing page data
type SEOPage struct {
	Title       string
	Description string
	Keywords    string
	FromFmt     string
	ToFmt       string
	Slug        string
}

var seoPages = map[string]SEOPage{
	"markdown-to-html": {
		Title:       "Markdown to HTML Converter Online",
		Description: "Convert Markdown to HTML instantly with our free online converter. Powered by Pandoc, no registration required, fast and secure.",
		Keywords:    "markdown to html, md to html, markdown converter, online converter",
		FromFmt:     "markdown",
		ToFmt:       "html",
		Slug:        "markdown-to-html",
	},
	"markdown-to-pdf": {
		Title:       "Markdown to PDF Converter Online",
		Description: "Convert Markdown to PDF instantly with our free online converter. Professional PDF output from Markdown, powered by Pandoc.",
		Keywords:    "markdown to pdf, md to pdf, markdown pdf converter",
		FromFmt:     "markdown",
		ToFmt:       "pdf",
		Slug:        "markdown-to-pdf",
	},
	"html-to-markdown": {
		Title:       "HTML to Markdown Converter Online",
		Description: "Convert HTML to Markdown instantly with our free online converter. Clean Markdown output from HTML, powered by Pandoc.",
		Keywords:    "html to markdown, html to md, html converter",
		FromFmt:     "html",
		ToFmt:       "markdown",
		Slug:        "html-to-markdown",
	},
	"docx-to-markdown": {
		Title:       "DOCX to Markdown Converter Online",
		Description: "Convert DOCX to Markdown instantly with our free online converter. Extract content from Word documents to Markdown format.",
		Keywords:    "docx to markdown, word to markdown, docx converter",
		FromFmt:     "docx",
		ToFmt:       "markdown",
		Slug:        "docx-to-markdown",
	},
	"markdown-to-docx": {
		Title:       "Markdown to DOCX Converter Online",
		Description: "Convert Markdown to DOCX instantly with our free online converter. Create Word documents from Markdown files.",
		Keywords:    "markdown to docx, md to docx, markdown word converter",
		FromFmt:     "markdown",
		ToFmt:       "docx",
		Slug:        "markdown-to-docx",
	},
	"rst-to-html": {
		Title:       "RST to HTML Converter Online",
		Description: "Convert reStructuredText to HTML instantly with our free online converter. Fast RST to HTML conversion powered by Pandoc.",
		Keywords:    "rst to html, restructuredtext to html, rst converter",
		FromFmt:     "rst",
		ToFmt:       "html",
		Slug:        "rst-to-html",
	},
	"latex-to-html": {
		Title:       "LaTeX to HTML Converter Online",
		Description: "Convert LaTeX to HTML instantly with our free online converter. Transform LaTeX documents to HTML format.",
		Keywords:    "latex to html, tex to html, latex converter",
		FromFmt:     "latex",
		ToFmt:       "html",
		Slug:        "latex-to-html",
	},
	"html-to-pdf": {
		Title:       "HTML to PDF Converter Online",
		Description: "Convert HTML to PDF instantly with our free online converter. Generate PDF documents from HTML files.",
		Keywords:    "html to pdf, html pdf converter, webpage to pdf",
		FromFmt:     "html",
		ToFmt:       "pdf",
		Slug:        "html-to-pdf",
	},
	"markdown-to-epub": {
		Title:       "Markdown to EPUB Converter Online",
		Description: "Convert Markdown to EPUB instantly with our free online converter. Create EPUB ebooks from Markdown files.",
		Keywords:    "markdown to epub, md to epub, ebook converter",
		FromFmt:     "markdown",
		ToFmt:       "epub",
		Slug:        "markdown-to-epub",
	},
	"rst-to-markdown": {
		Title:       "RST to Markdown Converter Online",
		Description: "Convert reStructuredText to Markdown instantly with our free online converter. Fast RST to Markdown conversion.",
		Keywords:    "rst to markdown, restructuredtext to markdown",
		FromFmt:     "rst",
		ToFmt:       "markdown",
		Slug:        "rst-to-markdown",
	},
	// New underrated/high-value formats
	"rtf-to-markdown": {
		Title:       "RTF to Markdown Converter Online",
		Description: "Convert RTF (Rich Text Format) to Markdown instantly with our free online converter. Extract content from RTF files to Markdown.",
		Keywords:    "rtf to markdown, rich text format to markdown, rtf converter",
		FromFmt:     "rtf",
		ToFmt:       "markdown",
		Slug:        "rtf-to-markdown",
	},
	"jira-to-markdown": {
		Title:       "Jira to Markdown Converter Online",
		Description: "Convert Jira wiki markup to Markdown instantly. Migrate Jira tickets to Markdown documentation with our free converter.",
		Keywords:    "jira to markdown, jira wiki markup to markdown, jira converter",
		FromFmt:     "jira",
		ToFmt:       "markdown",
		Slug:        "jira-to-markdown",
	},
	"ipynb-to-markdown": {
		Title:       "Jupyter Notebook to Markdown Converter Online",
		Description: "Convert Jupyter Notebooks (IPYNB) to Markdown instantly. Extract code and documentation from Jupyter notebooks to Markdown.",
		Keywords:    "jupyter to markdown, ipynb to markdown, jupyter notebook converter",
		FromFmt:     "ipynb",
		ToFmt:       "markdown",
		Slug:        "ipynb-to-markdown",
	},
	"ipynb-to-pdf": {
		Title:       "Jupyter Notebook to PDF Converter Online",
		Description: "Convert Jupyter Notebooks (IPYNB) to PDF instantly. Create professional PDFs from Jupyter notebooks with code and outputs.",
		Keywords:    "jupyter to pdf, ipynb to pdf, jupyter notebook pdf",
		FromFmt:     "ipynb",
		ToFmt:       "pdf",
		Slug:        "ipynb-to-pdf",
	},
	"vimwiki-to-markdown": {
		Title:       "Vimwiki to Markdown Converter Online",
		Description: "Convert Vimwiki files to Markdown instantly. Migrate your Vimwiki notes to Markdown format with our free converter.",
		Keywords:    "vimwiki to markdown, vim wiki converter, vimwiki markdown",
		FromFmt:     "vimwiki",
		ToFmt:       "markdown",
		Slug:        "vimwiki-to-markdown",
	},
	"opml-to-markdown": {
		Title:       "OPML to Markdown Converter Online",
		Description: "Convert OPML (Outline Processor Markup Language) to Markdown instantly. Import outlines from OPML to Markdown.",
		Keywords:    "opml to markdown, outline to markdown, opml converter",
		FromFmt:     "opml",
		ToFmt:       "markdown",
		Slug:        "opml-to-markdown",
	},
	"textile-to-html": {
		Title:       "Textile to HTML Converter Online",
		Description: "Convert Textile markup to HTML instantly. Convert Textile formatted text to HTML with our free online converter.",
		Keywords:    "textile to html, textile converter, textile markup",
		FromFmt:     "textile",
		ToFmt:       "html",
		Slug:        "textile-to-html",
	},
	// High-demand formats
	"gfm-to-html": {
		Title:       "GitHub-Flavored Markdown to HTML Converter Online",
		Description: "Convert GitHub-Flavored Markdown (GFM) to HTML instantly. Convert GitHub README and GFM comments to HTML with our free converter.",
		Keywords:    "gfm to html, github markdown to html, gfm converter",
		FromFmt:     "gfm",
		ToFmt:       "html",
		Slug:        "gfm-to-html",
	},
	"gfm-to-pdf": {
		Title:       "GitHub-Flavored Markdown to PDF Converter Online",
		Description: "Convert GitHub-Flavored Markdown (GFM) to PDF instantly. Create PDFs from GitHub README files and GFM content.",
		Keywords:    "gfm to pdf, github markdown to pdf",
		FromFmt:     "gfm",
		ToFmt:       "pdf",
		Slug:        "gfm-to-pdf",
	},
	"markdown-to-pptx": {
		Title:       "Markdown to PowerPoint Converter Online",
		Description: "Convert Markdown to PowerPoint (PPTX) instantly. Create PowerPoint presentations from Markdown files.",
		Keywords:    "markdown to pptx, md to powerpoint, markdown to slides",
		FromFmt:     "markdown",
		ToFmt:       "pptx",
		Slug:        "markdown-to-pptx",
	},
	"html-to-pptx": {
		Title:       "HTML to PowerPoint Converter Online",
		Description: "Convert HTML to PowerPoint (PPTX) instantly. Create PowerPoint presentations from HTML content.",
		Keywords:    "html to pptx, html to powerpoint, html to slides",
		FromFmt:     "html",
		ToFmt:       "pptx",
		Slug:        "html-to-pptx",
	},
}

// SEO landing page template
const seoTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}} | Convertly</title>
    <meta name="description" content="{{.Description}}">
    <meta name="keywords" content="{{.Keywords}}">
    <link rel="canonical" href="https://convertly.onrender.com/convert/{{.Slug}}">
    <meta property="og:title" content="{{.Title}}">
    <meta property="og:description" content="{{.Description}}">
    <meta property="og:type" content="website">
    <meta name="robots" content="index, follow">
    <script type="application/ld+json">
    {
      "@context": "https://schema.org",
      "@type": "WebApplication",
      "name": "Convertly",
      "description": "{{.Description}}",
      "url": "https://convertly.onrender.com",
      "applicationCategory": "UtilityApplication",
      "operatingSystem": "Any"
    }
    </script>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Geist:wght@400;500;600&family=DM+Mono:wght@400;500&family=Instrument+Serif:ital,wght@0,400;1,400&display=swap" rel="stylesheet">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        :root {
            --ink: #0d0d0d;
            --paper: #f5f0e8;
            --cream: #ede8dc;
            --gold: #c8a84b;
            --muted: #8a7f6e;
            --border: #d4cdc0;
            --white: #fefcf8;
        }
        body {
            font-family: 'Geist', sans-serif;
            background: var(--paper);
            color: var(--ink);
            line-height: 1.6;
            min-height: 100vh;
        }
        nav {
            position: sticky;
            top: 0;
            background: rgba(254, 252, 248, 0.9);
            backdrop-filter: blur(16px);
            border-bottom: 1px solid var(--border);
            padding: 1rem 2rem;
            display: flex;
            justify-content: space-between;
            align-items: center;
            z-index: 100;
        }
        .logo {
            font-family: 'Instrument Serif', serif;
            font-size: 1.5rem;
            font-weight: 400;
            color: var(--ink);
            text-decoration: none;
        }
        .logo span { color: var(--gold); font-style: italic; }
        .container {
            max-width: 800px;
            margin: 0 auto;
            padding: 4rem 2rem;
        }
        h1 {
            font-family: 'Instrument Serif', serif;
            font-size: 3rem;
            font-weight: 400;
            margin-bottom: 1rem;
            line-height: 1.2;
        }
        h1 em { color: var(--gold); font-style: italic; }
        .subtitle {
            font-size: 1.25rem;
            color: var(--muted);
            margin-bottom: 3rem;
        }
        .cta-button {
            display: inline-block;
            background: var(--ink);
            color: var(--gold);
            padding: 1rem 2rem;
            border-radius: 8px;
            text-decoration: none;
            font-weight: 500;
            transition: transform 0.2s, box-shadow 0.2s;
        }
        .cta-button:hover {
            transform: translateY(-2px);
            box-shadow: 0 4px 12px rgba(0,0,0,0.1);
        }
        .features {
            margin-top: 4rem;
        }
        .feature {
            padding: 1.5rem 0;
            border-bottom: 1px solid var(--border);
        }
        .feature h3 {
            font-family: 'DM Mono', monospace;
            font-size: 1.1rem;
            margin-bottom: 0.5rem;
        }
        .feature p {
            color: var(--muted);
        }
        footer {
            text-align: center;
            padding: 2rem;
            border-top: 1px solid var(--border);
            color: var(--muted);
            font-size: 0.9rem;
        }
        footer a { color: var(--ink); text-decoration: none; }
    </style>
</head>
<body>
    <nav>
        <a href="/" class="logo">cv <span>Convertly</span></a>
    </nav>
    <div class="container">
        <h1>{{.Title}}</h1>
        <p class="subtitle">{{.Description}}</p>
        <a href="/?from={{.FromFmt}}&to={{.ToFmt}}" class="cta-button">Start Converting</a>

        <div class="features">
            <div class="feature">
                <h3>Free & Unlimited</h3>
                <p>No registration required, no file size limits, completely free forever.</p>
            </div>
            <div class="feature">
                <h3>Powered by Pandoc</h3>
                <p>Uses the industry-standard Pandoc converter for accurate, reliable conversions.</p>
            </div>
            <div class="feature">
                <h3>Privacy First</h3>
                <p>All files are deleted within 2 seconds of download. Nothing is stored on our servers.</p>
            </div>
            <div class="feature">
                <h3>Lightning Fast</h3>
                <p>Conversions happen in seconds. No queues, no waiting.</p>
            </div>
        </div>
    </div>
    <footer>
        <p>Convertly — Convert anything to anything. <a href="/sitemap.xml">Sitemap</a></p>
    </footer>
</body>
</html>`

func main() {
	// Start worker pool
	startWorkers()

	// Start cleanup goroutine
	startCleanup()

	// Create router
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/convert", handleConvert)
	mux.HandleFunc("/api/download", handleDownload)
	mux.HandleFunc("/api/formats", handleFormats)
	mux.HandleFunc("/ping", handlePing)

	// SEO landing pages
	mux.HandleFunc("/convert/", handleSEOLanding)

	// Static files
	mux.Handle("/", http.FileServer(http.Dir("./static")))

	// Build middleware chain
	handler := withHeaders(withGzip(cors.Default().Handler(mux)))

	// Configure server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:           ":" + port,
		Handler:        handler,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   90 * time.Second,
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	log.Printf("Server starting on port %s", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// withHeaders adds security headers
func withHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// withGzip adds gzip compression
func withGzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()

		gzw := &gzipWriter{Writer: gz, ResponseWriter: w}
		next.ServeHTTP(gzw, r)
	})
}

// gzipWriter wraps gzip.Writer for http.ResponseWriter
type gzipWriter struct {
	Writer          *gzip.Writer
	ResponseWriter  http.ResponseWriter
}

func (g *gzipWriter) Header() http.Header {
	return g.ResponseWriter.Header()
}

func (g *gzipWriter) Write(b []byte) (int, error) {
	return g.Writer.Write(b)
}

func (g *gzipWriter) WriteHeader(statusCode int) {
	g.ResponseWriter.WriteHeader(statusCode)
}

// startWorkers spawns the worker pool
func startWorkers() {
	for i := 0; i < 8; i++ {
		go func() {
			for job := range jobQueue {
				processJob(job)
			}
		}()
	}
}

// startCleanup runs periodic cleanup of old jobs
func startCleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	go func() {
		for range ticker.C {
			cleanupOldJobs()
		}
	}()
}

// cleanupOldJobs removes jobs older than 30 minutes
func cleanupOldJobs() {
	jobStore.Lock()
	defer jobStore.Unlock()

	now := time.Now()
	for id, entry := range jobStore.jobs {
		if now.Sub(entry.CreatedAt) > 30*time.Minute {
			// Delete output file if exists
			if entry.OutputPath != "" {
				os.Remove(entry.OutputPath)
			}
			delete(jobStore.jobs, id)
		}
	}
}

// processJob processes a single conversion job
func processJob(job Job) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Update job status
	jobStore.Lock()
	jobStore.jobs[job.ID].Status = StatusProcessing
	jobStore.Unlock()

	result := Result{}

	// Prepare input/output paths
	var inputPath string
	if job.IsFile {
		inputPath = job.InputPath
		defer os.Remove(inputPath)
	} else {
		// Create temp file from content
		ext := formatExtensions[job.FromFmt]
		tmpFile, err := os.CreateTemp("", "pandoc_upload_*"+ext)
		if err != nil {
			result.Err = fmt.Errorf("failed to create temp file: %w", err)
			job.ResultChan <- result
			return
		}
		inputPath = tmpFile.Name()
		if _, err := tmpFile.WriteString(job.Content); err != nil {
			tmpFile.Close()
			result.Err = fmt.Errorf("failed to write content: %w", err)
			job.ResultChan <- result
			return
		}
		tmpFile.Close()
		defer os.Remove(inputPath)
	}

	// Prepare output path
	outExt := formatExtensions[job.ToFmt]
	outputPath := filepath.Join(os.TempDir(), "pandoc_output_"+job.ID+outExt)

	// Build pandoc command with improved flags
	args := []string{
		inputPath,
		"-f", job.FromFmt,
		"-t", job.ToFmt,
		"--standalone", // Create complete documents (fixes DOCX issues)
		"--wrap=none",  // Prevent unwanted line wrapping
	}

	// Add PDF-specific options - try multiple engines in order of preference
	if job.ToFmt == "pdf" {
		// Check which PDF engines are available
		pdfEngines := []string{"xelatex", "pdflatex", "luatex"}
		selectedEngine := ""

		for _, engine := range pdfEngines {
			if _, err := exec.LookPath(engine); err == nil {
				selectedEngine = engine
				break
			}
		}

		if selectedEngine == "" {
			// No PDF engine available - fail fast with clear error
			result.Err = fmt.Errorf("PDF conversion requires a LaTeX engine (xelatex, pdflatex, or luatex) to be installed. Please install texlive-latex-recommended and lmodern packages")
			job.ResultChan <- result

			jobStore.Lock()
			jobStore.jobs[job.ID].Status = StatusFailed
			jobStore.jobs[job.ID].Error = result.Err.Error()
			jobStore.Unlock()
			os.Remove(inputPath)
			return
		}

		args = append(args, "--pdf-engine="+selectedEngine)
	}

	// Output file must be last
	args = append(args, "-o", outputPath)

	cmd := exec.CommandContext(ctx, "pandoc", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		result.Err = fmt.Errorf("pandoc failed: %w, stderr: %s", err, stderr.String())
		job.ResultChan <- result

		// Update job status
		jobStore.Lock()
		jobStore.jobs[job.ID].Status = StatusFailed
		jobStore.jobs[job.ID].Error = result.Err.Error()
		jobStore.Unlock()
		return
	}

	result.OutputPath = outputPath
	job.ResultChan <- result

	// Update job status
	jobStore.Lock()
	jobStore.jobs[job.ID].Status = StatusDone
	jobStore.jobs[job.ID].OutputPath = outputPath
	jobStore.Unlock()
}

// handleConvert handles conversion requests
func handleConvert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Cache-Control", "no-store")

	var job Job
	job.ID = uuid.New().String()
	job.ResultChan = make(chan Result, 1)

	contentType := r.Header.Get("Content-Type")

	if strings.HasPrefix(contentType, "multipart/form-data") {
		// File upload
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "No file provided", http.StatusBadRequest)
			return
		}
		defer file.Close()

		// Save uploaded file
		ext := filepath.Ext(header.Filename)
		tmpFile, err := os.CreateTemp("", "pandoc_upload_*"+ext)
		if err != nil {
			http.Error(w, "Failed to create temp file", http.StatusInternalServerError)
			return
		}
		defer tmpFile.Close()

		if _, err := io.Copy(tmpFile, file); err != nil {
			http.Error(w, "Failed to save file", http.StatusInternalServerError)
			return
		}

		job.InputPath = tmpFile.Name()
		job.IsFile = true
		job.FromFmt = r.FormValue("from")
		job.ToFmt = r.FormValue("to")

		// Auto-detect from format if not provided
		if job.FromFmt == "" {
			if fmt, ok := extensionFormats[ext]; ok {
				job.FromFmt = fmt
			} else {
				job.FromFmt = "markdown"
			}
		}
	} else {
		// JSON content
		var data struct {
			FromFmt string `json:"from"`
			ToFmt   string `json:"to"`
			Content string `json:"content"`
		}

		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		job.Content = data.Content
		job.FromFmt = data.FromFmt
		job.ToFmt = data.ToFmt
		job.IsFile = false
	}

	// Validate formats
	if job.FromFmt == "" || job.ToFmt == "" {
		http.Error(w, "Missing format specification", http.StatusBadRequest)
		return
	}

	// Create job entry
	jobStore.Lock()
	jobStore.jobs[job.ID] = &JobEntry{
		Status:    StatusQueued,
		CreatedAt: time.Now(),
	}
	jobStore.Unlock()

	// Enqueue job
	select {
	case jobQueue <- job:
	default:
		http.Error(w, "Queue full, try again later", http.StatusServiceUnavailable)
		return
	}

	// Wait for result with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 65*time.Second)
	defer cancel()

	select {
	case result := <-job.ResultChan:
		if result.Err != nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":  result.Err.Error(),
				"job_id": job.ID,
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"job_id": job.ID,
			"status": "done",
		})

	case <-ctx.Done():
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":  "Conversion timeout",
			"job_id": job.ID,
		})
	}
}

// handleDownload handles file downloads
func handleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	jobID := r.URL.Query().Get("id")
	if jobID == "" {
		http.Error(w, "Missing job ID", http.StatusBadRequest)
		return
	}

	jobStore.RLock()
	entry, exists := jobStore.jobs[jobID]
	jobStore.RUnlock()

	if !exists {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	if entry.Status != StatusDone {
		http.Error(w, "Job not complete", http.StatusAccepted)
		return
	}

	if entry.OutputPath == "" {
		http.Error(w, "Output file not available", http.StatusNotFound)
		return
	}

	// Read file
	data, err := os.ReadFile(entry.OutputPath)
	if err != nil {
		http.Error(w, "Failed to read file", http.StatusInternalServerError)
		return
	}

	// Determine content type
	contentType := "application/octet-stream"
	if strings.HasSuffix(entry.OutputPath, ".html") {
		contentType = "text/html; charset=utf-8"
	} else if strings.HasSuffix(entry.OutputPath, ".pdf") {
		contentType = "application/pdf"
	} else if strings.HasSuffix(entry.OutputPath, ".json") {
		contentType = "application/json"
	} else if strings.HasSuffix(entry.OutputPath, ".txt") {
		contentType = "text/plain; charset=utf-8"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(entry.OutputPath))
	w.Header().Set("Cache-Control", "no-store")

	if _, err := w.Write(data); err != nil {
		return
	}

	// File will be cleaned up by the periodic cleanup job (30 minutes)
}

// handleFormats returns supported formats
func handleFormats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"input":  inputFormats,
		"output": outputFormats,
	})
}

// handlePing returns health check
func handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"ts":     time.Now().Format(time.RFC3339),
	})
}

// handleSEOLanding serves SEO-optimized landing pages
func handleSEOLanding(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract slug from path
	slug := strings.TrimPrefix(r.URL.Path, "/convert/")
	slug = strings.TrimSuffix(slug, "/")

	if slug == "" {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	// Look up SEO page data
	page, exists := seoPages[slug]
	if !exists {
		// Try to serve from static folder
		http.ServeFile(w, r, "./static/404.html")
		return
	}

	// Parse and render template
	tmpl, err := template.New("seo").Parse(seoTemplate)
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")

	if err := tmpl.Execute(w, page); err != nil {
		log.Printf("Template execute error: %v", err)
	}
}
