package main

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	_ "image/png"
	"path/filepath"
	"strings"
)

const gnomeIcons64 = `iVBORw0KGgoAAAANSUhEUgAAAeQAAAAWCAMAAAAmXR9tAAAC5VBMVEVHcEwBAQF+f3uPkYwQDQIZGAYAAAEAAQFZWVVAQj6UlpHV2NJbaRimp6SKjYepAgGho56Ok5F2eHSDhYCanJcZGRi0t7IGBgVtUShjOlk5PT0DCA8hIylph7C3urN4V4Clp6ldYFysrqqmqaMJCQqtsKqws6x6msldXR8qIS9ucGz7Li6Ji4UxRmR/XiyIkZlBXYTO0MxnQnB0dnFZM2OWmpNodh+anZe1ubGPDQ1lZ2VKTEqsrqgcLEoTGSJlZ2Ovsaz4YWFvjr6ipJ2WYhCPYBWuBARwfI5WXxNYZRWLjYiVmJMTFBMuMzSGlib9dnYYGRhhY19fOWmXmpRVV1O7vbYgSocpO1pgORSTXAR8Xi9nYWlVVlOBiF++aBtXTTl6mtCqbQeUAACtAQF/gXzAFhbfNzdMag51WS5WQyeoAACAZ0NribtTa5ZlgrGXczv////3+Pfs7ur+/v7w8e/y8/Lq6+iIioX19vTt7u36+vq6vLf8/Pzn5+jm6ORucGyPWQKxs67T1NNVV1PeqFTd3tvIysazookhSobP0M3Z2tjitW/CxcGunILZn0iNpuG+wL2pqqiCmbZch7zg4N/k4+PTyNrAscnqxY1yns4tMjPGt6GQsde80OeKeF/O2us+ZZqtweLMv9SQorq4qI/AsZjQkzqllX3Huc9nlMe8q5Lz5smlfj9MebHTybmBptFCRkads+Gsmbj+FBTAytTOCwugiavFmFiuu8rX4vGdr8adczGiszh6l9t7jaSyqZja0uFObJVceqKuvVC9AgK4p8JCVmxyfoqpoJCSeJxnSRnXqWf+9avFpHXnrl7/QUF/YYfj6/SVh3K/ynjDfD3X36jVtILz8NzWgDLw0qCNajLlvYD3w3Ty2bZOUU/53k60mXHrISHrRkbwqz/OJSXRQUHPXwX50CjGow3754j70I77tUnbw6jOXl6NdUyciWz9h4dbUT1OmgZ2Yj+J4TOS40O3Pya9QAaOAAAA33RSTlMALvv9EAobJQcD/f76//39/iX9/P79/lP+Uv44YP2i+Bb+/txEXPT9KnbF+OH4/kb9+/atY4zz3cf8/P7o95rRmvL+vfxw723G6Kns6PD++cmU96mx2ODIlMP1/eb2+WuIpcekpPHWo+XmbPy11qfp///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////+3N9C2gAAE3pJREFUaN61mnlcU1fax2MICQECBAIoggqlRSou1Y6tONrqWLdaq9Va208/7XRfZuZ9b7abPUhCIgmREkiIIsjmAiIFEVlHCrijIrSUumur1WqdLjPv+/79PufcJTcs2k6nvwQueTif55x7vvd3zrnnhsfL0IxSBu9hevKlSbx/T9OmxGUjPfb0s4+Keb9Nry57NeTBeX+vKFLwpI3PvG61vv7MxknBTGzyply/Nk1my66IWME5/Lra/hDEQRPD50dHZ2Vlrn9YT05/Z/nydyYx3aPRjZLmoYynFk/9NylPsRkJJKNcFfbHRwP/J332kWen/Ypcy+paWcpj553iHDNq+61RQLw+OUaW4rKR9rgUfvJ6uhmb6ov8qt/Elo5wRXAOv6A2o5aNBukBCWn22pxhFofd7iowGAwpyQ/uyUlvL7+1ZMm370W+87GAgux2S9wS9u12cyCHwCUiDhnN2NRteijlNydifRhY7jGlR+tUORVhOq9e/8QjXKbSP6q0jj/4LwdLGJbFgWTHck1hLtTn3poEkC8OvSpm8sq1apVOodBpOXkfU6p1CpI0G1SB0bHL/vIoT/qMxCHLTolOtmZmK+ye0EwpxKa8nDtIsDK25aKSz20BSbRRnMNzD65NLpeXNZSzUdbJMdjEWZnJycnW9eO2DK6/j5cv/3rJ+fPnX/zpp2/ei3z7Y4BMSkiujeEjDfnPaWlpf4bDDnwIYFx8vHg8ymJxSHAwGr8mnvrs1KlT2/a9Np377xRC7VRVd1R3FFZ36ZS2R0I4jNVGteop9nOYkRglYwHut7eWdZ67vwwg51w88CqbV4XOmDSbw/x5UwgFYmwmFaSTGx277C+P8uYZ4jKtyYYwV5zdGW1Ndsvm8aSGsOZcrirC//kBj7flTF5enkaenccezmx5QG1aub6+vr60vcHLRg2mMKfT5vFS8ticTpNh/Jbx3ln+3ozzMw4dmjEDOC958cX3IzFkT6h/0JexkKeltbeXwhyyorS9PG1aIGOfzzeK8spwjqYjyLuQPv9x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`

var DefaultIcons = map[string][]byte{}

func init() {
	buf, _ := base64.StdEncoding.DecodeString(gnomeIcons64)
	img, _, _ := image.Decode(bytes.NewReader(buf))

	const w = 22
	fill := func(name string, x int) {
		p := &bytes.Buffer{}
		png.Encode(p, img.(*image.Paletted).SubImage(image.Rect(w*x, 0, w*x+w, w)))
		DefaultIcons[name+".png"] = p.Bytes()
	}

	empty, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAABYAAAAWCAQAAABuvaSwAAAABGdBTUEAALGPC/xhBQAAAAFzUkdCAK7OHOkAAAAgY0hSTQAAeiYAAICEAAD6AAAAgOgAAHUwAADqYAAAOpgAABdwnLpRPAAAAAJiS0dEAACqjSMyAAAACXBIWXMAAABIAAAASABGyWs+AAAAEUlEQVQoz2NgGAWjYBQMTwAAA94AAaNKIU4AAABWdEVYdGNvbW1lbnQAVGhpcyBhcnQgaXMgaW4gdGhlIHB1YmxpYyBkb21haW4uIEtldmluIEh1Z2hlcywga2V2aW5oQGVpdC5jb20sIFNlcHRlbWJlciAxOTk1dvbvnAAAACV0RVh0ZGF0ZTpjcmVhdGUAMjAxNi0wNy0wN1QxMjozMzozOCswMjowMCegfkEAAAAldEVYdGRhdGU6bW9kaWZ5ADIwMDctMDktMTFUMDc6MTE6MDUrMDI6MDCpDaFiAAAAAElFTkSuQmCC")
	DefaultIcons["empty.png"] = empty
	fill("cert", 0)
	fill("exec", 2)
	fill("music", 3)
	fill("folder", 4)
	fill("back", 6)
	fill("image", 7)
	fill("archive", 8)
	fill("web", 9)
	fill("text", 11)
	fill("generic", 12)
	fill("shell", 13)
	fill("video", 14)
	fill("word", 17)
	fill("powerpoint", 19)
	fill("excel", 20)
}

var mimeMapping = map[string]string{
	".pem": "cert", ".crt": "cert", ".cer": "cert", ".der": "cert",
	".sh": "shell", ".bat": "shell", ".cmd": "shell",
	".exe": "exec", ".dmg": "exec", ".rpm": "exec", ".msi": "exec",
	".txt": "text", ".md": "text", ".rst": "text",
	".mp3": "music", ".wav": "music", ".flac": "music", ".ogg": "music",
	".mp4": "video", ".webm": "video", ".flv": "video",
	".html": "web", ".htm": "web", ".js": "web", ".css": "web",
	".doc": "word", ".docx": "word",
	".ppt": "powerpoint", ".pptx": "powerpoint",
	".xls": "excel", ".csv": "excel", ".xlsx": "excel",
	".jpg": "image", ".jpeg": "image", ".png": "image", ".gif": "image", ".webp": "image", ".tiff": "image", ".bmp": "image",
	".iso": "archive", ".zip": "archive", ".tar": "archive", ".gz": "archive", ".bz": "archive", ".bz2": "archive", ".7z": "archive", ".rar": "archive", ".deb": "archive",
}

func nameIcon(name string, folder bool) string {
	if folder {
		return "folder.png"
	}

	ext := strings.ToLower(filepath.Ext(name))
	t, ok := mimeMapping[ext]
	if ok {
		return t + ".png"
	}
	return "generic.png"
}
