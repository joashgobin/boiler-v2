package helpers

import (
	"os/exec"
	"slices"

	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3/log"
)

type SafeImage struct {
	srcPath           string
	intermediatePath  string
	intermediateWidth int
	outputPath        string
	outputWidth       int
	startTime         time.Time
}

func GetTempName(name string) string {
	return fmt.Sprintf("%s.%s.%d.lock", name, time.Now().Format(time.RFC3339), os.Getpid())
}

func (si *SafeImage) ProcessImage(start time.Time) {
	si.startTime = time.Now()
	lockPath := si.outputPath + "." + start.Format(time.RFC3339) + ".safe.lock"
	if FileExists(lockPath) {
		// log.Error("lock file already exists...aborting...")
		return
	}
	err := TouchFile(lockPath)
	if err != nil {
		log.Errorf("error creating safe image lock file: %v", err)
	}

	// log.Infof("processing image: %s -> %s -> %s", si.srcPath, si.intermediatePath, si.outputPath)
	// use intermediate if present
	if !FileExists(si.intermediatePath) {
		vipsThumbnail(si.srcPath, si.intermediatePath, si.intermediateWidth)
	}

	if FileExists(si.outputPath) {
		err = DeleteFile(lockPath)
		if err != nil {
			log.Errorf("error deleting safe image lock file: %v", err)
		}
		return
	}

	if si.intermediateWidth == si.outputWidth {
		err = DeleteFile(lockPath)
		if err != nil {
			log.Errorf("error deleting safe image lock file: %v", err)
		}
		return
	}

	vipsThumbnail(si.intermediatePath, si.outputPath, si.outputWidth)

	err = DeleteFile(lockPath)
	if err != nil {
		log.Errorf("error deleting safe image lock file: %v", err)
	}
	log.Infof("(%v) converted image (%s) to webp: %s", time.Since(si.startTime), si.srcPath, si.outputPath)
}

func ConvertInlineWebpFolder(imageChannel *chan *SafeImage, folderPath string, exts ...string) {
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		fmt.Printf("error reading directory (%s): %v\n", folderPath, err)
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() && slices.Contains(exts, filepath.Ext(entry.Name())) {
			fullPath := filepath.Join(folderPath, entry.Name())
			// fmt.Println("converting", fullPath)
			ConvertInlineWebp(imageChannel, fullPath, "static/gen/img", 1000)
		}
	}
}

func ConvertInlineWebp(imageChannel *chan *SafeImage, srcPath string, toDir string, dimensions ...int) string {
	width := 500
	intermediateWidth := 1000

	if len(dimensions) > 0 {
		width = dimensions[0]
	}
	fromDir := filepath.Dir(srcPath)
	// start := time.Now()
	hashString := GetFileHash(srcPath)

	outputPath := fmt.Sprintf("%s_%dx.%s.webp",
		strings.TrimSuffix(strings.Replace(srcPath, fromDir, toDir, -1),
			filepath.Ext(srcPath)), width, hashString)

	if !FileExists(outputPath) {
		intermediatePath := fmt.Sprintf("%s_%dx.%s%s",
			strings.TrimSuffix(strings.Replace(srcPath, fromDir, toDir, -1),
				filepath.Ext(srcPath)), intermediateWidth, hashString, filepath.Ext(srcPath))

		si := SafeImage{
			srcPath:           srcPath,
			intermediatePath:  intermediatePath,
			intermediateWidth: intermediateWidth,
			outputPath:        outputPath,
			outputWidth:       width,
		}

		*imageChannel <- &si
		return srcPath

		// si.ProcessImage(time.Now())
	}
	return outputPath
}

func vipsThumbnail(inputPath, outputPath string, dimensions ...int) error {
	outputFolderPath := filepath.Dir(outputPath) + "/"
	outputName := filepath.Base(outputPath)
	tempPath := filepath.Dir(inputPath) + "/" + outputName

	dimStr := "500x"
	if len(dimensions) > 0 {
		dimStr = fmt.Sprintf("%dx", dimensions[0])
	}
	if len(dimensions) > 1 {
		dimStr = fmt.Sprintf("%dx%d", dimensions[0], dimensions[1])
	}
	cmd := exec.Command("vipsthumbnail", inputPath, "--size", dimStr, "-o", outputName)
	_, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("vips thumbnail error: %v", err)
	}

	mvCmd := exec.Command("mv", tempPath, outputFolderPath)
	_, err = mvCmd.Output()
	if err != nil {
		return fmt.Errorf("vips move image error: %v", err)
	}
	return nil
}
