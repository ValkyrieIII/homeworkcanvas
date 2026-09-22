package ingestion

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func ParseDocument(ctx context.Context, path, filename, dir string) ([]Page, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	ext := strings.ToLower(filepath.Ext(filename))
	pdf := path
	if ext == ".ppt" || ext == ".pptx" {
		out := filepath.Join(dir, "converted")
		if e := os.MkdirAll(out, 0o700); e != nil {
			return nil, e
		}
		b, e := exec.CommandContext(ctx, "soffice", "--headless", "--convert-to", "pdf", "--outdir", out, path).CombinedOutput()
		if e != nil {
			return nil, fmt.Errorf("ppt conversion: %w: %s", e, b)
		}
		pdf = filepath.Join(out, strings.TrimSuffix(filepath.Base(filename), ext)+".pdf")
	}
	if ext != ".pdf" && ext != ".ppt" && ext != ".pptx" {
		return nil, fmt.Errorf("unsupported")
	}
	count, e := pageCount(ctx, pdf)
	if e != nil {
		return nil, e
	}
	out := make([]Page, 0, count)
	for n := 1; n <= count; n++ {
		textPath := filepath.Join(dir, fmt.Sprintf("page-%d.txt", n))
		b, e := exec.CommandContext(ctx, "pdftotext", "-f", strconv.Itoa(n), "-l", strconv.Itoa(n), "-layout", pdf, textPath).CombinedOutput()
		if e != nil {
			return nil, fmt.Errorf("page %d: %w: %s", n, e, b)
		}
		text, e := os.ReadFile(textPath)
		if e != nil {
			return nil, e
		}
		out = append(out, Page{Number: n, Text: string(text)})
	}
	return out, nil
}

// RenderPreviews creates only the located-page images in the temporary job directory.
func RenderPreviews(ctx context.Context, filename, dir string, pages []Page) error {
	ext := strings.ToLower(filepath.Ext(filename))
	pdf := filepath.Join(dir, "source"+ext)
	if ext == ".ppt" || ext == ".pptx" {
		pdf = filepath.Join(dir, "converted", strings.TrimSuffix(filepath.Base(filename), ext)+".pdf")
	}
	for i := range pages {
		base := filepath.Join(dir, fmt.Sprintf("preview-%d", pages[i].Number))
		output, err := exec.CommandContext(ctx, "pdftoppm", "-f", strconv.Itoa(pages[i].Number), "-l", strconv.Itoa(pages[i].Number), "-png", "-singlefile", pdf, base).CombinedOutput()
		if err != nil {
			return fmt.Errorf("preview page %d: %w: %s", pages[i].Number, err, output)
		}
		pages[i].PreviewPath = base + ".png"
	}
	return nil
}
func pageCount(ctx context.Context, pdf string) (int, error) {
	b, e := exec.CommandContext(ctx, "pdfinfo", pdf).CombinedOutput()
	if e != nil {
		return 0, e
	}
	m := regexp.MustCompile(`(?m)^Pages:\s+(\d+)\s*$`).FindStringSubmatch(string(b))
	if len(m) != 2 {
		return 0, fmt.Errorf("page count missing")
	}
	return strconv.Atoi(m[1])
}
