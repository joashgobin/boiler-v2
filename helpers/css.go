package helpers

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/gofiber/fiber/v3/log"
)

func GetEmbedFiles(fs *embed.FS, path string) ([]string, error) {
	if fs == nil {
		return []string{}, fmt.Errorf("get files from FS error: embedded file system not found")
	}
	entries, err := fs.ReadDir(filepath.Clean(path))
	if err != nil {
		return nil, err
	}

	var out []string
	for _, entry := range entries {
		fullPath := filepath.Join(path, entry.Name())
		if entry.IsDir() {
			res, err := GetEmbedFiles(fs, fullPath)
			if err != nil {
				return nil, err
			}
			out = append(out, res...)
			continue
		}
		out = append(out, fullPath)
	}

	return out, nil
}

func ExtractClassNames(fs *embed.FS, filePath string, classes *[]string) error {
	// Read file content
	data, err := fs.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %v", err)
	}

	// HTML class attribute regex pattern
	re := regexp.MustCompile(`class="([^"]+)"|class='([^']+)'`)

	// Find all matches
	matches := re.FindAllStringSubmatch(string(data), -1)

	// Process matches and split by spaces
	for _, match := range matches {
		// Handle both double quotes and single quotes
		classList := match[1]
		if classList == "" {
			classList = match[2]
		}

		// Split by spaces and add individual classes
		for className := range strings.SplitSeq(classList, " ") {
			if className != "" { // Skip empty entries
				if !slices.Contains(*classes, className) {
					*classes = append(*classes, className)
				}
			}
		}
	}

	return nil
}

func SaveCSSClasses(fs *embed.FS, targetFile string, cssFiles ...string) error {
	if fs == nil {
		return fmt.Errorf("files not embedded")
	}
	tempExt := fmt.Sprintf(".%d.lock", os.Getpid())
	tempFile := targetFile + tempExt
	viewFiles, err := GetEmbedFiles(fs, "views")
	classes := []string{}
	if err != nil {
		return err
	}
	for _, file := range viewFiles {
		// fmt.Println(file)
		err = ExtractClassNames(fs, file, &classes)
		if err != nil {
			return err
		}
	}
	// fmt.Println(classes)
	var accruedString strings.Builder
	for _, file := range cssFiles {
		// fmt.Println("optimizing:", file)
		data, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read file: %v", err)
		}

		targetCSSstring := string(data)
		// fmt.Println(targetCSSstring)

		// getting chunks that match the regex \n*\.([^{]*){[^}]*}|@[^{]*{([^{]*){[^}]*}[^}*]}
		queryExp := `(?m)@([^{]*?{[^{]*?){([^}]*?)*}([^}])*}`
		queryRe := regexp.MustCompile(queryExp)
		queryMatches := queryRe.FindAllStringSubmatch(targetCSSstring, -1)

		for _, match := range queryMatches {
			// fmt.Printf("Query (CSS): %s\n", match[1])
			// fmt.Printf("Query match: %s\n", match[0])
			for _, class := range classes {
				// add query content to accrued CSS string
				// if query name contains class
				if strings.Contains(match[1], "."+class) {
					accruedString.WriteString(match[0])
				}
			}

			// remove query match from body
			targetCSSstring = strings.ReplaceAll(targetCSSstring, match[0], "")
		}

		classExp := `(?m)^\n*?([^{]*?){[^}]*?}`
		classRe := regexp.MustCompile(classExp)
		classMatches := classRe.FindAllStringSubmatch(targetCSSstring, -1)

		for _, match := range classMatches {
			selectorName := strings.TrimSpace(match[1])
			selectorContent := match[0]
			// fmt.Printf("Selector (CSS): %s\n", selectorName)
			// fmt.Printf("Class match: %s\n", selectorContent)
			for _, class := range classes {
				// add selector content to accrued CSS string
				// if selector name contains class
				if strings.Contains(selectorName, "."+class) {
					accruedString.WriteString(selectorContent)
					// fmt.Println(selectorName, "contains", "."+class)
				} else {
					// fmt.Println(selectorName, "does not contain", "."+class)
				}
			}
		}
	}
	// fmt.Println(accruedString)
	accruedFile, err := os.Create(tempFile)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := accruedFile.Close(); closeErr != nil {
			log.Errorf("error closing accrued CSS file: %v", err)
		}
	}()

	_, err = accruedFile.WriteString(accruedString.String())
	if err != nil {
		log.Errorf("error saving accrued CSS file: %v", err)
	}

	err = os.Rename(tempFile, targetFile)
	if err != nil {
		return err
	}

	return nil
}
