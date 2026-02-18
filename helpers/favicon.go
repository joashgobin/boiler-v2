/*
Refactored from: https://github.com/shinshin86/gosfg
*/
package helpers

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"

	"github.com/disintegration/imaging"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/log"
)

type Icon struct {
	Src   string `json:"src"`
	Sizes string `json:"sizes"`
	Type  string `json:"type"`
}

type Icons []Icon

type WebManifest struct {
	Name            string `json:"name"`
	ShortName       string `json:"short_name"`
	Icons           Icons  `json:"icons"`
	ThemeColor      string `json:"theme_color"`
	BackgroundColor string `json:"background_color"`
	Display         string `json:"display"`
}

func generateWebManifest(dstPath, siteName, themeColor, displayMode string) {
	icon192 := Icon{Src: "/android-chrome-192x192.png", Sizes: "192x192", Type: "image/png"}
	icon512 := Icon{Src: "/android-chrome-512x512.png", Sizes: "512x512", Type: "image/png"}

	var icons Icons
	icons = append(icons, icon192)
	icons = append(icons, icon512)

	webManifestJson := WebManifest{
		Name:            siteName,
		ShortName:       siteName,
		Icons:           icons,
		ThemeColor:      themeColor,
		BackgroundColor: themeColor,
		Display:         displayMode,
	}

	f, err := os.Create(dstPath)
	if err != nil {
		fmt.Printf("[ERROR] Failed to create WebManifestJson:\n%v", err)
	}
	defer f.Close()

	data, err2 := json.Marshal(webManifestJson)
	if err2 != nil {
		fmt.Printf("[ERROR] Failed to json marshal:\n%v", err)
		os.Exit(1)
	}

	_, err3 := f.Write(data)
	if err3 != nil {
		fmt.Printf("[ERROR] Failed to write file:\n%v", err)
		os.Exit(1)
	}
}

func generateBrowserConfigXML(dstPath, tileColor string) {
	configXml := `<?xml version="1.0" encoding="utf-8"?>
<browserconfig>
    <msapplication>
        <tile>
            <square70x70logo src="/mstile-70x70.png"/>
            <square150x150logo src="/mstile-150x150.png"/>
            <wide310x150logo src="/mstile-310x150.png"/>
            <square310x310logo src="/mstile-310x310.png"/>
            <TileColor>` + tileColor + `</TileColor>
        </tile>
    </msapplication>
</browserconfig>`

	f, err := os.Create(dstPath)
	if err != nil {
		fmt.Printf("[ERROR] Failed to create BrowserConfigXML:\n%v", err)
	}
	defer f.Close()

	data := []byte(configXml)
	_, err2 := f.Write(data)
	if err2 != nil {
		fmt.Printf("[ERROR] Failed to write file:\n%v", err)
		os.Exit(1)
	}
}

func generateImage(img *image.Image, hash, dstPath string, width, height int) {
	favLockPath := fmt.Sprintf("%s.%s.fav.lock", dstPath, hash)
	if FileExists(favLockPath) {
		return
	}
	newImg := imaging.Clone(*img)
	resizeImg := imaging.Resize(newImg, width, height, imaging.Lanczos)

	err := imaging.Save(resizeImg, dstPath)
	if err != nil {
		fmt.Printf("[ERROR] Failed to save image(%s):\n%v", filepath.Base(dstPath), err)
		os.Exit(1)
	}
	TouchFile(favLockPath)
}

func generateFaviconImages(targetImg, outputDir string) {
	hash := GetFileHash(targetImg)
	src, err := imaging.Open(targetImg)
	if err != nil {
		fmt.Printf("[ERROR] Failed to open image:\n%v", err)
		os.Exit(1)
	}

	generateImage(&src, hash, filepath.Join(outputDir, "android-chrome-192x192.png"), 192, 192)
	generateImage(&src, hash, filepath.Join(outputDir, "android-chrome-512x512.png"), 512, 512)
	generateImage(&src, hash, filepath.Join(outputDir, "apple-touch-icon.png"), 180, 180)
	generateImage(&src, hash, filepath.Join(outputDir, "favicon-16x16.png"), 16, 16)
	generateImage(&src, hash, filepath.Join(outputDir, "favicon-32x32.png"), 32, 32)
	generateImage(&src, hash, filepath.Join(outputDir, "favicon.png"), 48, 48)
	generateImage(&src, hash, filepath.Join(outputDir, "mstile-70x70.png"), 70, 70)
	generateImage(&src, hash, filepath.Join(outputDir, "mstile-150x150.png"), 150, 150)
	generateImage(&src, hash, filepath.Join(outputDir, "mstile-310x150.png"), 310, 150)
	generateImage(&src, hash, filepath.Join(outputDir, "mstile-310x310.png"), 310, 310)
}

func GenerateFavicon(targetImg, outputDir string) {
	if FileExists(targetImg) {
		tileColor := "red"
		generateFaviconImages(targetImg, outputDir)
		generateBrowserConfigXML(filepath.Join(outputDir, "browserconfig.xml"), tileColor)
		generateWebManifest(filepath.Join(outputDir, "site.webmanifest"), "", "", "")
	} else {
		if !fiber.IsChild() {
			log.Warnf("image %s could not be processed into favicon for %s", targetImg, outputDir)
		}
	}
}
