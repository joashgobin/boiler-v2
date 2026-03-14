package helpers

import (
	"embed"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v3/log"
)

func SaveComponents(fs *embed.FS, bank BankInterface, cmpLog *map[string]string) error {
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
			ExtractComponents(fs, file, bank, cmpLog)
		}
	}
	return nil
}

func ExtractComponents(fs *embed.FS, filePath string, bank BankInterface, cmpLog *map[string]string) error {
	data, err := fs.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %v", err)
	}

	re := regexp.MustCompile(fmt.Sprintf(`{{\s*cmp\s+"(.*?)"\s+%s((.|\n|\r\n)*?)%s\s*}}`, "`", "`"))
	results := re.FindAllStringSubmatch(string(data), -1)
	cmpMap := make(map[string]string, len(results))
	for _, v := range results {
		if len(v) > 2 {
			// fmt.Println(v[1], v[2])
			hash := GetHash(v[2])
			key := "cmp-" + hash
			cmpMap[key] = v[2]
			(*cmpLog)[v[1]] = hash
			bank.Delete(key)

			fileName := fmt.Sprintf("views/cmp/%s.html", key)
			templateContent := strings.TrimSpace(v[2])
			err := os.WriteFile(fileName, []byte(templateContent), 0644)
			if err != nil {
				log.Infof("error writing cmp file: %v", err)
				continue
			}
		}
	}
	return nil
}
