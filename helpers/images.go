package helpers

import (
	"net/url"
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
	SrcPath           string
	intermediatePath  string
	intermediateWidth int
	outputPath        string
	outputWidth       int
	startTime         time.Time
}

func GetImageDimensions(imgPath string) (int, int) {
	file, err := os.Open(imgPath)
	if err != nil {
		log.Errorf("error getting image dimensions: %v", err)
		return 0, 0
	}
	defer file.Close()

	config, _, err := image.DecodeConfig(file)
	if err != nil {
		log.Errorf("error getting image dimensions: %v", err)
		return 0, 0
	}
	return config.Width, config.Height
}

func GetTempName(name string) string {
	return fmt.Sprintf("%s.%s.%d.lock", name, time.Now().Format(time.RFC3339), os.Getpid())
}

func (si *SafeImage) ProcessImage(start time.Time) {
	// log.Infof("processing: %s", si.SrcPath)
	si.startTime = time.Now()

	if !FileExists(si.intermediatePath) {
		// log.Infof("creating intermediate image: %s", si.intermediatePath)
		vipsThumbnail(si.SrcPath, si.intermediatePath, si.intermediateWidth)
	}

	if FileExists(si.outputPath) {
		return
	}

	/*
		if si.intermediateWidth == si.outputWidth {
			return
		}
	*/

	// log.Infof("creating final image: %s", si.outputPath)
	vipsThumbnail(si.intermediatePath, si.outputPath, si.outputWidth)

	// log.Infof("(%v) converted image (%s): %s", time.Since(si.startTime), si.SrcPath, si.outputPath)
}

type InlineImage struct {
	AVIF     string
	WEBP     string
	Fallback string
}

func filePathURL(srcPath string) string {
	u := url.URL{Path: filepath.ToSlash(srcPath)}
	return u.String()
}

func ConvertInline(imageChannel *chan *SafeImage, lru *LRU[string], imageLru *LRU[InlineImage], srcPath string, toDir string, dimensions ...int) InlineImage {
	// now := time.Now()

	var imageLruKeyBuilder strings.Builder
	imageLruKeyBuilder.WriteString(srcPath)
	// imageLruKeyBuilder.WriteString(toDir)
	if len(dimensions) > 0 {
		imageLruKeyBuilder.WriteString(strconv.Itoa(dimensions[0]))
	}
	currentImage := imageLru.Get(imageLruKeyBuilder.String())
	if currentImage.Fallback != "" {
		// fmt.Println("lru inline image:", time.Since(now))
		return currentImage
	}

	width := 500
	intermediateWidth := 1000
	imageWidth, _ := GetImageDimensions(srcPath)
	if imageWidth > 0 {
		intermediateWidth = min(1000, imageWidth)
	}

	if len(dimensions) > 0 {
		width = dimensions[0]
	}
	fromDir := filepath.Dir(srcPath)
	hashString := GetFileHash(srcPath)

	outputs := make(map[string]string, 3)
	exts := []string{"avif", "webp", strings.ReplaceAll(filepath.Ext(srcPath), ".", "")}
	for _, ext := range exts {
		var lruKeyBuilder strings.Builder
		lruKeyBuilder.WriteString(hashString)
		// lruKeyBuilder.WriteString("-")
		lruKeyBuilder.WriteString(strconv.Itoa(width))
		// lruKeyBuilder.WriteString("-")
		lruKeyBuilder.WriteString(ext)

		var outputPath string
		cachedOutputPath := lru.Get(lruKeyBuilder.String())
		if cachedOutputPath == "" {
			var outputBuilder strings.Builder
			outputBuilder.WriteString(strings.TrimSuffix(strings.Replace(
				srcPath,
				fromDir, toDir, -1),
				filepath.Ext(srcPath)))
			outputBuilder.WriteString("_")
			outputBuilder.WriteString(strconv.Itoa(width))
			outputBuilder.WriteString("x.")
			outputBuilder.WriteString(hashString)
			outputBuilder.WriteString(".")
			outputBuilder.WriteString(ext)
			outputPath = outputBuilder.String()
			lru.Set(lruKeyBuilder.String(), outputPath)
			// fmt.Println("cold:", time.Since(now))
		} else {
			outputPath = cachedOutputPath
			// fmt.Println("hot:", time.Since(now))
		}

		if ext == "avif" || ext == "webp" {
			outputs[ext] = filePathURL(outputPath)
		} else {
			outputs["original"] = filePathURL(outputPath)
		}

		if !FileExists(outputPath) {
			intermediatePath := fmt.Sprintf("%s_%dx.%s%s",
				strings.TrimSuffix(strings.Replace(
					srcPath,
					fromDir, toDir, -1),
					filepath.Ext(srcPath)), intermediateWidth, hashString, filepath.Ext(srcPath))

			si := SafeImage{
				SrcPath:           srcPath,
				intermediatePath:  intermediatePath,
				intermediateWidth: intermediateWidth,
				outputPath:        outputPath,
				outputWidth:       width,
			}

			*imageChannel <- &si
		}
	}

	// fmt.Println(outputs)
	// fmt.Println("new inline image:", time.Since(now))
	newInlineImage := InlineImage{
		AVIF:     outputs["avif"],
		WEBP:     outputs["webp"],
		Fallback: outputs["original"],
	}
	imageLru.Set(imageLruKeyBuilder.String(), newInlineImage)
	return newInlineImage
}

func ConvertInlineAvif(imageChannel *chan *SafeImage, lru *LRU[string], srcPath string, toDir string, dimensions ...int) string {
	// now := time.Now()
	width := 500
	intermediateWidth := 1000
	imageWidth, _ := GetImageDimensions(srcPath)
	if imageWidth > 0 {
		intermediateWidth = min(1000, imageWidth)
	}

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
		// fmt.Println("cold:", time.Since(now))
	} else {
		outputPath = cachedOutputPath
		// fmt.Println("hot:", time.Since(now))
	}

	if !FileExists(outputPath) {
		intermediatePath := fmt.Sprintf("%s_%dx.%s%s",
			strings.TrimSuffix(strings.Replace(srcPath, fromDir, toDir, -1),
				filepath.Ext(srcPath)), intermediateWidth, hashString, filepath.Ext(srcPath))

		si := SafeImage{
			SrcPath:           srcPath,
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

func ConvertInlineWebp(imageChannel *chan *SafeImage, lru *LRU[string], srcPath string, toDir string, dimensions ...int) string {
	width := 500
	intermediateWidth := 1000
	imageWidth, _ := GetImageDimensions(srcPath)
	if imageWidth > 0 {
		intermediateWidth = min(1000, imageWidth)
	}

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
			SrcPath:           srcPath,
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

func ConvertInlineOriginal(imageChannel *chan *SafeImage, lru *LRU[string], srcPath string, toDir string, dimensions ...int) string {
	width := 500
	intermediateWidth := 1000
	imageWidth, _ := GetImageDimensions(srcPath)
	if imageWidth > 0 {
		intermediateWidth = min(1000, imageWidth)
	}

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
			SrcPath:           srcPath,
			intermediatePath:  intermediatePath,
			intermediateWidth: intermediateWidth,
			outputPath:        outputPath,
			outputWidth:       width,
		}

		*imageChannel <- &si
		// fmt.Println("fallback output path:", outputPath)
		return outputPath
		// return srcPath

	}
	// fmt.Println("webp output path:", outputPath)
	return outputPath
}

func GetFileSize(path string) int {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return int(fileInfo.Size())
}

func vipsThumbnail(inputPath, outputPath string, dimensions ...int) error {
	outputFolderPath := filepath.Dir(outputPath) + "/"
	outputName := filepath.Base(outputPath)
	// tempPath := filepath.Dir(inputPath) + "/" + outputName
	ext := strings.TrimPrefix(filepath.Ext(outputPath), ".")

	endArgs := ""
	if strings.HasSuffix(outputName, ".avif") {
		endArgs = "[Q=40,effort=4,subsample-mode=auto,strip]"
	}

	// set target dimensions for conversion
	dimStr := "500x"
	if len(dimensions) > 0 {
		dimStr = fmt.Sprintf("%dx", dimensions[0])
	}
	if len(dimensions) > 1 {
		dimStr = fmt.Sprintf("%dx%d", dimensions[0], dimensions[1])
	}
	inputCopyPath := outputFolderPath + "_copy_" + ext + "_" + dimStr + "_" + filepath.Base(inputPath)
	log.Infof("vips: %s -> %s (dim: %s)", inputPath, outputPath, dimStr)

	lockPath := outputFolderPath + "_lock_" + ext + "_" + dimStr + "_" + filepath.Base(outputPath) + ".lock"

	if FileExists(inputCopyPath) {
		log.Infof("retrying based on input copy path: %s", lockPath)
		DeleteFile(outputPath)
		DeleteFile(inputCopyPath)
		DeleteFile(lockPath)
	}

	if FileExists(lockPath) {
		log.Infof("retrying based on lock path: %s", lockPath)
		DeleteFile(outputPath)
		DeleteFile(inputCopyPath)
		DeleteFile(lockPath)
	}

	// create copying lock file
	TouchFile(lockPath)

	// copy input file to output directory
	CopyFile(inputPath, inputCopyPath)

	// convert input copy in output directory
	cmd := exec.Command("vipsthumbnail", "--vips-concurrency=1", inputCopyPath, "--size", dimStr, "-o", outputName+endArgs)
	cmd.Output()

	// delete input copy
	DeleteFile(inputCopyPath)

	// delete converting lock
	DeleteFile(lockPath)

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

	vipsThumbnail(inputPath, outputPath, config.Width, config.Height)
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
