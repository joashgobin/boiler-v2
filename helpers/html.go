package helpers

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"regexp"
	"strings"

	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/js"
)

func SaveComponents(fs *embed.FS, shelf ShelfInterface, bank BankInterface, cmpLog *map[string]template.HTML) error {
	if fs == nil {
		return fmt.Errorf("files not embedded")
	}
	viewFiles, err := GetEmbedFiles(fs, "views")
	if err != nil {
		return err
	}
	for _, file := range viewFiles {
		// fmt.Printf("saving components from %s\n", file)
		if !strings.Contains(file, "/cmp/") {
			ExtractComponents(fs, file, shelf, bank, cmpLog)
		}
	}
	return nil
}

func ExtractComponents(fs *embed.FS, filePath string, shelf ShelfInterface, bank BankInterface, cmpLog *map[string]template.HTML) error {
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
	// cmpMap := make(map[string]string, len(results))
	for _, v := range results {
		if len(v) > 2 {
			// fmt.Println(v[1], v[2])
			hash := GetXXH3(v[2])
			key := "cmp-" + hash
			// cmpMap[key] = v[2]

			var snippet strings.Builder
			snippet.WriteString(`<div hx-get="/cmp/`)
			snippet.WriteString(hash)
			snippet.WriteString(`" hx-trigger="load" hx-target="this" hx-swap="outerHTML"></div>`)
			(*cmpLog)[v[1]] = template.HTML(snippet.String())
			bank.Delete(key)
			minifiedHTML, err := m.String("text/html", strings.TrimSpace(v[2]))
			if err != nil {
				continue
			}
			shelf.Set(key, minifiedHTML)
		}
	}
	return nil
}

func SaveInline(fs *embed.FS, inlineLog *map[string]template.HTML, funcMap map[string]any) error {
	if fs == nil {
		return fmt.Errorf("files not embedded")
	}
	viewFiles, err := GetEmbedFiles(fs, "views")
	if err != nil {
		return err
	}
	for _, file := range viewFiles {
		// fmt.Printf("saving inline from %s\n", file)
		ExtractInline(fs, file, inlineLog, funcMap)
	}
	return nil
}

func ExtractInline(fs *embed.FS, filePath string, inlineLog *map[string]template.HTML, funcMap map[string]any) error {
	data, err := fs.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %v", err)
	}

	m := minify.New()
	m.AddFunc("text/html", html.Minify)
	m.AddFunc("text/css", css.Minify)
	m.AddFunc("text/javascript", js.Minify)

	re := regexp.MustCompile(fmt.Sprintf(`{{\s*inline\s+%s((.|\n|\r\n)*?)%s\s*}}`, "`", "`"))
	results := re.FindAllStringSubmatch(string(data), -1)
	// cmpMap := make(map[string]string, len(results))
	for _, v := range results {
		if len(v) > 1 {
			// fmt.Println("inline:", v[1])
			tmpl, err := template.New("webpage").Funcs(funcMap).Parse(v[1])
			// log.Infof("get template string time: %v", time.Since(now).Microseconds())
			if err != nil {
				continue
			}
			var buf bytes.Buffer

			err = tmpl.Execute(&buf, nil)
			if err != nil {
				continue
			}

			minifiedHTML, err := m.String("text/html", buf.String())
			// fmt.Println(buf.String(), minifiedHTML)
			if err != nil {
				continue
			}
			(*inlineLog)[v[1]] = template.HTML(minifiedHTML)
		}
	}
	return nil
}
