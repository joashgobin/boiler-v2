package helpers

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"os"
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v3/log"
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/js"
)

func SaveComponents(fs *embed.FS, lru *LRU[string], cmpLog *map[string]template.HTML, funcMaps ...map[string]any) error {
	if fs == nil {
		return fmt.Errorf("files not embedded")
	}
	viewFiles, err := GetEmbedFiles(fs, "views")
	if err != nil {
		return err
	}
	for _, file := range viewFiles {
		for _, fm := range funcMaps {
			if len(fm) > 0 {
				if !strings.Contains(file, "/cmp/") {
					extractComponents(fs, file, lru, cmpLog, fm)
				}
			}
		}
	}
	return nil
}

func extractComponents(fs *embed.FS, filePath string, lru *LRU[string], cmpLog *map[string]template.HTML, funcMap map[string]any) error {
	data, err := fs.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %v", err)
	}

	m := minify.New()
	m.AddFunc("text/html", html.Minify)
	m.AddFunc("text/css", css.Minify)
	m.AddFunc("text/javascript", js.Minify)

	re := regexp.MustCompile(fmt.Sprintf(`{{\s*cmp\s+"(.*?)"\s+%s((.|\n|\r\n)*?)%s\s*}}`, "`", "`"))
	results := re.FindAllStringSubmatch(string(data), -1)
	for _, v := range results {
		if len(v) > 2 {
			hash := GetXXH3(v[2])

			var snippet strings.Builder
			snippet.WriteString(`<div hx-get="/cmp/`)
			snippet.WriteString(hash)
			snippet.WriteString(`" hx-trigger="load" hx-target="this" hx-swap="outerHTML"></div>`)
			(*cmpLog)[v[1]] = template.HTML(snippet.String())

			// generate HTML code for cmp
			// log.Infof("key: %v", v[1])
			// fmt.Println("starting HTML:", v[2])
			tmpl, err := template.New("webpage").Funcs(funcMap).Parse(v[2])
			if err != nil {
				log.Errorf("extract components template parse error: %v", err)
				continue
			}
			var buf bytes.Buffer

			err = tmpl.Execute(&buf, nil)
			if err != nil {
				log.Errorf("extract components template execute error: %v", err)
				continue
			}

			minifiedHTML, err := m.String("text/html", buf.String())
			if err != nil {
				log.Errorf("extract components minify error: %v", err)
				continue
			}
			lru.Set(hash, minifiedHTML)
		}
	}
	return nil
}

func SaveFileSnippets(fs *embed.FS, snippetLog *map[string]string) error {
	if fs == nil {
		return fmt.Errorf("files not embedded")
	}
	viewFiles, err := GetEmbedFiles(fs, "views")
	if err != nil {
		return err
	}
	for _, file := range viewFiles {
		ExtractFileSnippets(fs, file, snippetLog)
	}
	return nil
}

func ExtractFileSnippets(fs *embed.FS, filePath string, snippetLog *map[string]string) error {
	data, err := fs.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %v", err)
	}

	/*
		m := minify.New()
		m.AddFunc("text/html", html.Minify)
		m.AddFunc("text/css", css.Minify)
		m.AddFunc("text/javascript", js.Minify)
	*/

	re := regexp.MustCompile(`{{\s*(js|css|html)\s+"(.*?)"\s*}}`)
	results := re.FindAllStringSubmatch(string(data), -1)
	for _, v := range results {
		if len(v) > 1 {
			switch v[1] {
			case "js":
				m := minify.New()
				m.AddFunc("text/javascript", js.Minify)
				// fmt.Println("reading", v[2])
				jsText, err := os.ReadFile(v[2])
				if err != nil {
					log.Errorf("error reading snippet: %v", err)
					continue
				}
				minifiedJS, err := m.String("text/javascript", string(jsText))
				if err != nil {
					log.Errorf("error minifying snippet: %v", err)
					continue
				}
				// fmt.Println("storing", minifiedJS)
				(*snippetLog)[v[2]] = minifiedJS
			default:
				(*snippetLog)[v[2]] = string(data)
			}
		}
	}
	return nil
}

func SaveInline(fs *embed.FS, inlineLog *map[string]template.HTML, funcMaps ...map[string]any) error {
	if fs == nil {
		return fmt.Errorf("files not embedded")
	}
	viewFiles, err := GetEmbedFiles(fs, "views")
	if err != nil {
		return err
	}
	for _, file := range viewFiles {
		// fmt.Printf("saving inline from %s\n", file)
		for _, fm := range funcMaps {
			if len(fm) > 0 {
				extractInline(fs, file, inlineLog, fm)
			}
		}
	}
	return nil
}

func extractInline(fs *embed.FS, filePath string, inlineLog *map[string]template.HTML, funcMap map[string]any) error {
	data, err := fs.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %v", err)
	}

	m := minify.New()
	m.AddFunc("text/html", html.Minify)
	m.AddFunc("text/css", css.Minify)
	m.AddFunc("text/javascript", js.Minify)

	re := regexp.MustCompile(fmt.Sprintf(`{{\s*inline\s+"(.*?)"\s+%s((.|\n|\r\n)*?)%s\s*}}`, "`", "`"))
	results := re.FindAllStringSubmatch(string(data), -1)
	// cmpMap := make(map[string]string, len(results))
	for _, v := range results {
		if len(v) > 2 {
			tmpl, err := template.New("webpage").Funcs(funcMap).Parse(v[2])
			if err != nil {
				continue
			}
			var buf bytes.Buffer

			err = tmpl.Execute(&buf, nil)
			if err != nil {
				log.Errorf("extract inline template execute error: %v", err)
				continue
			}

			minifiedHTML, err := m.String("text/html", buf.String())
			// fmt.Println(buf.String(), minifiedHTML)
			if err != nil {
				log.Errorf("extract inline minify error: %v", err)
				continue
			}
			(*inlineLog)[v[1]] = template.HTML(minifiedHTML)
		}
	}
	return nil
}
