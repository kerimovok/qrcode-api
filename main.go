package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math"
	"net/http"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/gofiber/fiber/v2"
	"github.com/skip2/go-qrcode"
)

// QRCodeOptions represents the customization parameters for QR code generation
type QRCodeOptions struct {
	Data          string  `json:"data"`
	Size          int     `json:"size"`
	Foreground    string  `json:"foreground"`
	Background    string  `json:"background"`
	Error         string  `json:"error"`
	Border        int     `json:"border"`
	LogoURL       string  `json:"logo_url"`
	LogoSize      float64 `json:"logo_size"` // percentage of QR size
	GradientStart string  `json:"gradient_start"`
	GradientEnd   string  `json:"gradient_end"`
	GradientType  string  `json:"gradient_type"` // "linear", "radial"
}

// parseColor converts a color string to color.Color
func parseColor(colorStr string) color.Color {
	// Handle RGB/RGBA format
	var r, g, b, a uint8 = 0, 0, 0, 255

	if n, err := fmt.Sscanf(colorStr, "rgb(%d,%d,%d)", &r, &g, &b); err == nil && n == 3 {
		return color.RGBA{R: r, G: g, B: b, A: a}
	}
	if n, err := fmt.Sscanf(colorStr, "rgba(%d,%d,%d,%d)", &r, &g, &b, &a); err == nil && n == 4 {
		return color.RGBA{R: r, G: g, B: b, A: a}
	}

	// Handle basic named colors as fallback
	switch strings.ToLower(colorStr) {
	case "black":
		return color.Black
	case "white":
		return color.White
	case "red":
		return color.RGBA{R: 255, A: 255}
	case "green":
		return color.RGBA{G: 255, A: 255}
	case "blue":
		return color.RGBA{B: 255, A: 255}
	default:
		return color.Black
	}
}

// getErrorCorrection maps string to qrcode error correction level
func getErrorCorrection(level string) qrcode.RecoveryLevel {
	switch level {
	case "L":
		return qrcode.Low
	case "M":
		return qrcode.Medium
	case "Q":
		return qrcode.High
	case "H":
		return qrcode.Highest
	default:
		return qrcode.Medium
	}
}

// You'll need to add logo embedding logic after QR generation
func embedLogo(qrImage image.Image, logoURL string, sizePercent float64) (image.Image, error) {
	// Download logo
	resp, err := http.Get(logoURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read logo image
	logoImg, err := png.Decode(resp.Body)
	if err != nil {
		return nil, err
	}

	// Calculate logo size
	qrSize := qrImage.Bounds().Size()
	logoWidth := int(float64(qrSize.X) * sizePercent / 100)
	logoHeight := int(float64(qrSize.Y) * sizePercent / 100)

	// Resize logo
	logoImg = imaging.Fit(logoImg, logoWidth, logoHeight, imaging.Lanczos)

	// Create new image with same size as QR code
	finalImg := image.NewRGBA(qrImage.Bounds())

	// Draw QR code
	draw.Draw(finalImg, finalImg.Bounds(), qrImage, image.Point{}, draw.Over)

	// Calculate logo position (center)
	x := (qrSize.X - logoWidth) / 2
	y := (qrSize.Y - logoHeight) / 2
	logoPos := image.Rect(x, y, x+logoWidth, y+logoHeight)

	// Draw logo
	draw.Draw(finalImg, logoPos, logoImg, image.Point{}, draw.Over)

	return finalImg, nil
}

func createGradient(width, height int, startColor, endColor color.Color, gradientType string) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Convert colors to RGBA for easier manipulation
	startR, startG, startB, _ := startColor.RGBA()
	endR, endG, endB, _ := endColor.RGBA()

	// Convert from uint32 to uint8 (shift by 8 to get correct color values)
	startR, startG, startB = startR>>8, startG>>8, startB>>8
	endR, endG, endB = endR>>8, endG>>8, endB>>8

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			var ratio float64

			switch gradientType {
			case "linear":
				ratio = float64(x) / float64(width-1)
			case "radial":
				centerX, centerY := float64(width)/2, float64(height)/2
				distance := math.Sqrt(math.Pow(float64(x)-centerX, 2) + math.Pow(float64(y)-centerY, 2))
				maxDistance := math.Sqrt(math.Pow(centerX, 2) + math.Pow(centerY, 2))
				ratio = math.Min(distance/maxDistance, 1.0)
			default:
				ratio = float64(x) / float64(width-1)
			}

			r := uint8(float64(startR) + ratio*float64(int(endR)-int(startR)))
			g := uint8(float64(startG) + ratio*float64(int(endG)-int(startG)))
			b := uint8(float64(startB) + ratio*float64(int(endB)-int(startB)))

			img.Set(x, y, color.RGBA{r, g, b, 255})
		}
	}

	return img
}

func main() {
	app := fiber.New()

	app.Get("/generate", func(c *fiber.Ctx) error {
		options := QRCodeOptions{
			Data:          c.Query("data", ""),
			Size:          c.QueryInt("size", 300),
			Foreground:    c.Query("foreground", "black"),
			Background:    c.Query("background", "white"),
			Error:         c.Query("error", "M"),
			Border:        c.QueryInt("border", 4),
			LogoURL:       c.Query("logo_url", ""),
			LogoSize:      c.QueryFloat("logo_size", 20.0),
			GradientStart: c.Query("gradient_start", ""),
			GradientEnd:   c.Query("gradient_end", ""),
			GradientType:  c.Query("gradient_type", "linear"),
		}

		// Validation
		if options.Data == "" {
			return c.Status(400).JSON(fiber.Map{"error": "Data parameter is required"})
		}

		// Validation
		if options.Border < 0 {
			options.Border = 0
		}

		// Generate base QR code
		qr, err := qrcode.New(options.Data, getErrorCorrection(options.Error))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to generate QR code"})
		}

		// Set QR code properties
		qr.ForegroundColor = parseColor(options.Foreground)
		qr.BackgroundColor = parseColor(options.Background)

		// Handle border
		if options.Border == 0 {
			qr.DisableBorder = true
		} else {
			qr.DisableBorder = false
			// The QR code library uses 4 as the default border size
			// We might need to add padding manually if we want a larger border
			extraPadding := options.Border - 4
			if extraPadding > 0 {
				options.Size += (extraPadding * 2) // Increase size to accommodate extra padding
			}
		}

		// Generate initial image
		var buf bytes.Buffer
		if err := qr.Write(options.Size, &buf); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to generate image"})
		}

		// Decode the generated image
		img, err := png.Decode(bytes.NewReader(buf.Bytes()))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to process image"})
		}

		// Apply gradient if specified
		if options.GradientStart != "" && options.GradientEnd != "" {
			startColor := parseColor(options.GradientStart)
			endColor := parseColor(options.GradientEnd)
			gradient := createGradient(img.Bounds().Dx(), img.Bounds().Dy(), startColor, endColor, options.GradientType)

			// Create a new RGBA image for the result
			finalImg := image.NewRGBA(img.Bounds())

			// Draw the gradient first
			draw.Draw(finalImg, finalImg.Bounds(), gradient, image.Point{}, draw.Src)

			// Draw the QR code on top, but only where it's the foreground color
			for y := 0; y < img.Bounds().Dy(); y++ {
				for x := 0; x < img.Bounds().Dx(); x++ {
					r, g, b, _ := img.At(x, y).RGBA()
					// Check if the pixel matches the foreground color
					fr, fg, fb, _ := qr.ForegroundColor.RGBA()
					if r == fr && g == fg && b == fb {
						finalImg.Set(x, y, gradient.At(x, y))
					} else {
						finalImg.Set(x, y, qr.BackgroundColor)
					}
				}
			}

			img = finalImg
		}

		// Embed logo if specified
		if options.LogoURL != "" {
			img, err = embedLogo(img, options.LogoURL, options.LogoSize)
			if err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "Failed to embed logo"})
			}
		}

		// Encode final image
		var finalBuf bytes.Buffer
		if err := png.Encode(&finalBuf, img); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to encode final image"})
		}

		c.Set("Content-Type", "image/png")
		return c.Send(finalBuf.Bytes())
	})

	log.Fatal(app.Listen(":3007"))
}
