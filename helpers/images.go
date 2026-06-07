package helpers

import (
	"os/exec"
	"strconv"

	"fmt"
	"image"
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

func ConvertInlineAvif(imageChannel *chan *SafeImage, lru *LRU, srcPath string, toDir string, dimensions ...int) string {
	width := 500
	intermediateWidth := 1000

	if len(dimensions) > 0 {
		width = dimensions[0]
	}
	fromDir := filepath.Dir(srcPath)
	hashString := GetFileHash(srcPath)
	var lruKeyBuilder strings.Builder
	lruKeyBuilder.WriteString(hashString)
	lruKeyBuilder.WriteString("-")
	lruKeyBuilder.WriteString(strconv.Itoa(width))
	lruKeyBuilder.WriteString("-avif")

	var outputPath string
	cachedOutputPath := lru.Get(lruKeyBuilder.String())
	if cachedOutputPath == "" {
		var outputBuilder strings.Builder
		outputBuilder.WriteString(strings.TrimSuffix(strings.Replace(srcPath, fromDir, toDir, -1),
			filepath.Ext(srcPath)))
		outputBuilder.WriteString("_")
		outputBuilder.WriteString(strconv.Itoa(width))
		outputBuilder.WriteString("x.")
		outputBuilder.WriteString(hashString)
		outputBuilder.WriteString(".avif")
		outputPath = outputBuilder.String()
		lru.Set(lruKeyBuilder.String(), outputPath)
	} else {
		outputPath = cachedOutputPath
	}

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
		// fmt.Println("avif output path:", outputPath)
		return outputPath
		// return srcPath

	}
	// fmt.Println("avif output path:", outputPath)
	return outputPath
}

func ConvertInlineWebp(imageChannel *chan *SafeImage, lru *LRU, srcPath string, toDir string, dimensions ...int) string {
	width := 500
	intermediateWidth := 1000

	if len(dimensions) > 0 {
		width = dimensions[0]
	}
	fromDir := filepath.Dir(srcPath)
	hashString := GetFileHash(srcPath)
	var lruKeyBuilder strings.Builder
	lruKeyBuilder.WriteString(hashString)
	lruKeyBuilder.WriteString("-")
	lruKeyBuilder.WriteString(strconv.Itoa(width))
	lruKeyBuilder.WriteString("-webp")

	var outputPath string
	cachedOutputPath := lru.Get(lruKeyBuilder.String())
	if cachedOutputPath == "" {
		var outputBuilder strings.Builder
		outputBuilder.WriteString(strings.TrimSuffix(strings.Replace(srcPath, fromDir, toDir, -1),
			filepath.Ext(srcPath)))
		outputBuilder.WriteString("_")
		outputBuilder.WriteString(strconv.Itoa(width))
		outputBuilder.WriteString("x.")
		outputBuilder.WriteString(hashString)
		outputBuilder.WriteString(".webp")
		outputPath = outputBuilder.String()
		lru.Set(lruKeyBuilder.String(), outputPath)
	} else {
		outputPath = cachedOutputPath
	}

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
		// fmt.Println("webp output path:", outputPath)
		return outputPath
		// return srcPath

	}
	// fmt.Println("webp output path:", outputPath)
	return outputPath
}

func ConvertInlineOriginal(imageChannel *chan *SafeImage, lru *LRU, srcPath string, toDir string, dimensions ...int) string {
	width := 500
	intermediateWidth := 1000

	if len(dimensions) > 0 {
		width = dimensions[0]
	}
	fromDir := filepath.Dir(srcPath)
	hashString := GetFileHash(srcPath)
	var lruKeyBuilder strings.Builder
	lruKeyBuilder.WriteString(hashString)
	lruKeyBuilder.WriteString("-")
	lruKeyBuilder.WriteString(strconv.Itoa(width))
	lruKeyBuilder.WriteString("-original")

	var outputPath string
	cachedOutputPath := lru.Get(lruKeyBuilder.String())
	if cachedOutputPath == "" {
		var outputBuilder strings.Builder
		outputBuilder.WriteString(strings.TrimSuffix(strings.Replace(srcPath, fromDir, toDir, -1),
			filepath.Ext(srcPath)))
		outputBuilder.WriteString("_")
		outputBuilder.WriteString(strconv.Itoa(width))
		outputBuilder.WriteString("x.")
		outputBuilder.WriteString(hashString)
		outputBuilder.WriteString(filepath.Ext(srcPath))
		outputPath = outputBuilder.String()
		lru.Set(lruKeyBuilder.String(), outputPath)
	} else {
		outputPath = cachedOutputPath
	}

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
		// fmt.Println("webp output path:", outputPath)
		return outputPath
		// return srcPath

	}
	// fmt.Println("webp output path:", outputPath)
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

func ConvertPNGToJPG(inputPath, outputPath string) {
	if FileExists(outputPath) {
		return
	}

	reader, err := os.Open(inputPath)
	if err != nil {
		return
	}
	defer reader.Close()

	config, _, err := image.DecodeConfig(reader)
	if err != nil {
		return
	}

	err = vipsThumbnail(inputPath, outputPath, config.Width, config.Height)
	if err != nil {
		log.Errorf("error converting png to jpeg: %v", err)
		return
	}
}

func ConvertJPGToPNG(inputPath, outputPath string) {
	if FileExists(outputPath) {
		return
	}

	reader, err := os.Open(inputPath)
	if err != nil {
		return
	}
	defer reader.Close()

	config, _, err := image.DecodeConfig(reader)
	if err != nil {
		return
	}

	err = vipsThumbnail(inputPath, outputPath, config.Width, config.Height)
	if err != nil {
		log.Errorf("error converting jpeg to png: %v", err)
		return
	}
}
