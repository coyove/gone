package main

import (
	"image"
	"image/draw"
	"image/png"
	"io/ioutil"
	"os"
	"strings"
)

func main() {
	icons, _ := ioutil.ReadDir(".")
	count := 0
	for _, icon := range icons {
		if !strings.HasSuffix(icon.Name(), ".png") {
			continue
		}
		count++
	}

	canvas := image.NewRGBA(image.Rect(0, 0, count*22, 22))
	i := 0
	for _, icon := range icons {
		if !strings.HasSuffix(icon.Name(), ".png") {
			continue
		}
		ifs, _ := os.Open(icon.Name())
		img, _, _ := image.Decode(ifs)
		draw.Draw(canvas, image.Rect(22*i, 0, 22*i+22, 22), img, image.ZP, draw.Src)
		ifs.Close()
		i++
	}
	of, _ := os.Create("main.png")
	png.Encode(of, canvas)
	of.Close()
}
