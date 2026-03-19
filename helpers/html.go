package helpers

import (
	"embed"
	"fmt"
	"regexp"
	"strings"
)

func SaveComponents(fs *embed.FS, shelf ShelfInterface, bank BankInterface, cmpLog *map[string]string) error {
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

func ExtractComponents(fs *embed.FS, filePath string, shelf ShelfInterface, bank BankInterface, cmpLog *map[string]string) error {
	data, err := fs.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %v", err)
	}

	re := regexp.MustCompile(fmt.Sprintf(`{{\s*cmp\s+"(.*?)"\s+%s((.|\n|\r\n)*?)%s\s*}}`, "`", "`"))
	results := re.FindAllStringSubmatch(string(data), -1)
	// cmpMap := make(map[string]string, len(results))
	for _, v := range results {
		if len(v) > 2 {
			// fmt.Println(v[1], v[2])
			hash := GetHash(v[2])
			key := "cmp-" + hash
			// cmpMap[key] = v[2]
			(*cmpLog)[v[1]] = hash
			bank.Delete(key)
			shelf.Set(key, strings.TrimSpace(v[2]))
		}
	}
	return nil
}
