package helpers

import (
	"image"
	"os/exec"
	"slices"

	"fmt"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Kagami/go-avif"
	"github.com/gofiber/fiber/v3/log"
	"github.com/kolesa-team/go-webp/encoder"
	"github.com/kolesa-team/go-webp/webp"
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
			ConvertInlineWebp(imageChannel, fullPath, "static/gen/img", 1200)
		}
	}
}

func ConvertInlineWebp(imageChannel *chan *SafeImage, srcPath string, toDir string, dimensions ...int) string {
	width := 600
	intermediateWidth := 1200

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

func ConvertToAVIF(srcPath string, fileListPtr *map[string]string, fromDir, toDir string) error {
	start := time.Now()
	hashString := GetFileHash(srcPath)
	outputPath := fmt.Sprintf("%s.%s.avif",
		strings.TrimSuffix(strings.Replace(srcPath, fromDir, toDir, -1),
			filepath.Ext(srcPath)), hashString)

	if FileExists(outputPath) {
		// log.Info("skipping ", outputPath)
		if fileListPtr != nil {
			(*fileListPtr)[strings.TrimPrefix(srcPath, "static/")] = outputPath
		}
		return nil
	}
	file, err := os.Open(srcPath)
	if err != nil {
		return err
	}

	var img image.Image

	switch filepath.Ext(srcPath) {
	case ".png":
		img, err = png.Decode(file)
		if err != nil {
			return err
		}
	case ".jpg", ".jpeg":
		img, err = jpeg.Decode(file)
		if err != nil {
			return err
		}
	}

	output, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer output.Close()
	if err := avif.Encode(output, img, nil); err != nil {
		return err
	}
	log.Infof("(%v) converted image (%s) to avif: %s", time.Since(start), srcPath, outputPath)
	if fileListPtr != nil {
		(*fileListPtr)[strings.TrimPrefix(srcPath, "static/")] = outputPath
	}
	return nil
}

func ConvertToWebp(srcPath string, fileListPtr *map[string]string, fromDir, toDir string) error {
	start := time.Now()
	hashString := GetFileHash(srcPath)
	outputPath := fmt.Sprintf("%s.%s.webp",
		strings.TrimSuffix(strings.Replace(srcPath, fromDir, toDir, -1),
			filepath.Ext(srcPath)), hashString)

	if FileExists(outputPath) {
		// log.Info("skipping ", outputPath)
		if fileListPtr != nil {
			(*fileListPtr)[strings.TrimPrefix(srcPath, "static/")] = outputPath
		}
		return nil
	}
	file, err := os.Open(srcPath)
	if err != nil {
		return err
	}

	var img image.Image

	switch filepath.Ext(srcPath) {
	case ".png":
		img, err = png.Decode(file)
		if err != nil {
			return err
		}
	case ".jpg", ".jpeg":
		img, err = jpeg.Decode(file)
		if err != nil {
			return err
		}
	}

	output, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer output.Close()
	options, err := encoder.NewLossyEncoderOptions(encoder.PresetDefault, 75)
	if err != nil {
		return err
	}
	if err := webp.Encode(output, img, options); err != nil {
		return err
	}
	log.Infof("(%v) converted image (%s) to webp: %s", time.Since(start), srcPath, outputPath)
	if fileListPtr != nil {
		(*fileListPtr)[strings.TrimPrefix(srcPath, "static/")] = outputPath
	}
	return nil
}

func ConvertInFolderToAVIF(folderPath string, targetFolder string, ext string, fileListPtr *map[string]string) {
	err := os.MkdirAll(targetFolder, 0755)
	if err != nil {
		log.Infof("failed to create directory %s", targetFolder)
	}
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		fmt.Printf("error reading directory (%s): %v\n", folderPath, err)
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ext {
			err := ConvertToAVIF(filepath.Join(folderPath, entry.Name()), fileListPtr, folderPath, targetFolder)
			if err != nil {
				log.Errorf("could not convert file (%s) to avif: err\n", entry.Name(), err)
			}
		}
	}

}

func ConvertInFolderToWebp(folderPath string, targetFolder string, ext string, fileListPtr *map[string]string) {
	err := os.MkdirAll(targetFolder, 0755)
	if err != nil {
		log.Infof("failed to create directory %s", targetFolder)
	}
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		fmt.Printf("error reading directory (%s): %v\n", folderPath, err)
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ext {
			err := ConvertToWebp(filepath.Join(folderPath, entry.Name()), fileListPtr, folderPath, targetFolder)
			if err != nil {
				log.Errorf("could not convert file (%s) to webp: err\n", entry.Name(), err)
			}
		}
	}

}

func vipsThumbnail(inputPath, outputPath string, dimensions ...int) error {
	outputFolderPath := filepath.Dir(outputPath) + "/"
	outputName := filepath.Base(outputPath)
	tempPath := filepath.Dir(inputPath) + "/" + outputName

	dimStr := "600x"
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
