package kittyimg

import (
	"compress/zlib"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	_ "golang.org/x/image/webp"

	"github.com/gen2brain/avif"
	awebp "github.com/gen2brain/webp"
	"github.com/rs/zerolog/log"

	"github.com/adrg/xdg"
	easyjson "github.com/mailru/easyjson"
	"github.com/spf13/afero"
	"golang.org/x/sync/syncmap"
)

var (
	BaseImageDirectory = filepath.Join(xdg.DataHome, "chatuino")
)

var ErrUnsupportedAnimatedFormat = errors.New("emote is animated but in non supported format")

var (
	globalImagePlacementIDCounter atomic.Int32 = atomic.Int32{}
	globalPlacedImages                         = &syncmap.Map{}
)

//easyjson:json
type DecodedImage struct {
	ID     int32               `json:"-"`
	Cols   int                 `json:"cols"`
	Images []DecodedImageFrame `json:"images"`

	lastUsed time.Time `json:"-"`
}

//easyjson:json
type DecodedImageFrame struct {
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	EncodedPath string `json:"encoded_path"`
	DelayInMS   int    `json:"delay_in_ms"`
}

func (i DecodedImage) PrepareCommand() string {
	// not animated
	if len(i.Images) == 1 {
		transmitCMD := fmt.Sprintf("\x1b_Gf=32,i=%d,t=f,q=2,s=%d,v=%d,o=z;%s\x1b\\", i.ID, i.Images[0].Width, i.Images[0].Height, i.Images[0].EncodedPath)
		placementCMD := fmt.Sprintf("\x1b_Ga=p,i=%d,p=%d,q=2,U=1,r=1,c=%d\x1b\\", i.ID, i.ID, i.Cols)
		return transmitCMD + placementCMD
	}

	// is animated

	var b strings.Builder

	// transmit first image
	fmt.Fprintf(&b, "\033_Gf=32,i=%d,t=f,q=2,s=%d,v=%d,o=z;%s\033\\", i.ID, i.Images[0].Width, i.Images[0].Height, i.Images[0].EncodedPath)

	// send first frame
	fmt.Fprintf(&b, "\033_Ga=a,i=%d,r=1,z=%d,q=2;\033\\", i.ID, i.Images[0].DelayInMS)

	// send each frame after first image
	for img := range slices.Values(i.Images[1:]) {
		fmt.Fprintf(&b, "\033_Ga=f,i=%d,t=t,f=32,s=%d,v=%d,z=%d,q=2,o=z;%s\033\\", i.ID, img.Width, img.Height, img.DelayInMS, img.EncodedPath)
	}

	// start animation
	fmt.Fprintf(&b, "\033_Ga=a,i=%d,s=3,v=1,q=2;\033\\", i.ID)

	// create virtual placement
	fmt.Fprintf(&b, "\x1b_Ga=p,i=%d,p=%d,q=2,U=1,r=1,c=%d\x1b\\", i.ID, i.ID, i.Cols)

	return b.String()
}

func (i DecodedImage) DisplayUnicodePlaceholder() string {
	r, g, b := intToRGB(i.ID)
	return fmt.Sprintf("\033[38;2;%d;%d;%dm%s\033[39m", r, g, b, strings.Repeat("\U0010EEEE", i.Cols))
}

func intToRGB(i int32) (byte, byte, byte) {
	return byte(i >> 16), byte(i >> 8), byte(i)
}

type DisplayUnit struct {
	ID           string
	Directory    string
	IsAnimated   bool
	RightPadding int                                   // pixels of transparent padding to add on right side
	Load         func() (io.ReadCloser, string, error) `json:"-"`
}

type KittyDisplayUnit struct {
	PrepareCommand  string
	ReplacementText string
}

type DisplayManager struct {
	fs                    afero.Fs
	cellWidth, cellHeight float32
}

func NewDisplayManager(fs afero.Fs, cellWidth, cellHeight float32) *DisplayManager {
	return &DisplayManager{
		fs:         fs,
		cellWidth:  cellWidth,
		cellHeight: cellHeight,
	}
}

func (d *DisplayManager) Convert(unit DisplayUnit) (KittyDisplayUnit, error) {
	// 1st: image was already placed in this session, reusing placement
	if cached, ok := globalPlacedImages.Load(unit.ID); ok {
		i, ok := cached.(DecodedImage)
		if !ok {
			log.Logger.Error().Str("id", unit.ID).Type("type", cached).Msg("unexpected type in session cache")
			globalPlacedImages.Delete(unit.ID)
		} else {
			i.lastUsed = time.Now()
			globalPlacedImages.Swap(unit.ID, i)

			return KittyDisplayUnit{
				// don't resend placement command
				ReplacementText: i.DisplayUnicodePlaceholder(),
			}, nil
		}
	}

	// 2nd: image was not placed in session yet, but is already cached on FS
	incrementID := globalImagePlacementIDCounter.Add(1)

	cachedDecoded, found, err := d.openCached(unit)
	if err != nil {
		log.Logger.Warn().Err(err).Str("id", unit.ID).Msg("failed to open cached image, will re-download")
	}

	if found {
		cachedDecoded.ID = incrementID
		cachedDecoded.lastUsed = time.Now()

		//log.Logger.Info().Str("id", unit.ID).Int32("placement-id", cachedDecoded.ID).Msg("load image from storage cache")

		globalPlacedImages.Store(unit.ID, cachedDecoded)
		return KittyDisplayUnit{
			PrepareCommand:  cachedDecoded.PrepareCommand(),
			ReplacementText: cachedDecoded.DisplayUnicodePlaceholder(),
		}, nil
	}

	// 3rd: image was not downloaded yet, download and convert and save
	imageBody, contentType, err := unit.Load()
	if err != nil {
		return KittyDisplayUnit{}, err
	}

	log.Logger.Info().Str("id", unit.ID).Str("type", contentType).Msg("downloaded image")

	defer imageBody.Close()

	decoded, err := d.convertImageBytes(imageBody, unit, contentType)
	if err != nil {
		log.Logger.Err(err).Any("unit", unit).Send()
		return KittyDisplayUnit{}, err
	}

	decoded.ID = incrementID                                   // set id
	decoded.lastUsed = time.Now()                              // last used for clean up
	globalPlacedImages.Store(unit.ID, decoded)                 // store placement
	if err := d.cacheDecodedImage(decoded, unit); err != nil { // cache decoded image
		log.Logger.Warn().Err(err).Str("id", unit.ID).Msg("failed to cache decoded image")
	}

	return KittyDisplayUnit{
		PrepareCommand:  decoded.PrepareCommand(),
		ReplacementText: decoded.DisplayUnicodePlaceholder(),
	}, nil
}

func (d *DisplayManager) CleanupOldImagesCommand(maxAge time.Duration) string {
	var cmd strings.Builder

	globalPlacedImages.Range(func(key, value any) bool {
		c, ok := value.(DecodedImage)
		if !ok {
			globalPlacedImages.Delete(key)
			return true
		}
		if time.Since(c.lastUsed) > maxAge {
			fmt.Fprintf(&cmd, "\x1b_Ga=D,i=%d,q=2\x1b\\", c.ID)
			globalPlacedImages.Delete(key)
		}
		return true
	})

	return cmd.String()
}

func (d *DisplayManager) CleanupAllImagesCommand() string {
	return "\x1b_Ga=D\x1b\\"
}

func (d *DisplayManager) convertImageBytes(r io.Reader, unit DisplayUnit, contentType string) (DecodedImage, error) {
	if contentType == "image/avif" {
		return d.convertAnimatedAvif(r, unit)
	}

	if unit.IsAnimated && contentType == "image/webp" {
		return d.convertAnimatedWebP(r, unit)
	}

	if unit.IsAnimated && contentType == "image/gif" {
		//log.Logger.Info().Any("unit", unit).Msg("converting animated gif")
		return d.convertAnimatedGif(r, unit)
	}

	if unit.IsAnimated {
		return DecodedImage{}, fmt.Errorf("%w: got content type: %s with animated flag", ErrUnsupportedAnimatedFormat, contentType)
	}

	return d.convertDefault(r, unit)
}

func (d *DisplayManager) convertAnimatedAvif(r io.Reader, unit DisplayUnit) (DecodedImage, error) {
	images, err := avif.DecodeAll(r)
	if err != nil {
		return DecodedImage{}, fmt.Errorf("failed to convert avif: %w", err)
	}

	var cols int
	var decodedEmote DecodedImage
	for i, img := range images.Image {
		frame, c, err := d.convertImageFrame(img, unit, i)
		if err != nil {
			return DecodedImage{}, err
		}

		if i == 0 {
			cols = c
		}

		frame.DelayInMS = int(images.Delay[i] * 1000) // Delay is in seconds
		decodedEmote.Images = append(decodedEmote.Images, frame)
	}

	decodedEmote.Cols = cols

	return decodedEmote, nil
}

func (d *DisplayManager) convertAnimatedGif(r io.Reader, unit DisplayUnit) (DecodedImage, error) {
	images, err := gif.DecodeAll(r)
	if err != nil {
		return DecodedImage{}, fmt.Errorf("failed to convert animated gif: %w", err)
	}

	// Get canvas dimensions from config, or fall back to first frame
	width, height := images.Config.Width, images.Config.Height
	if width == 0 || height == 0 {
		width = images.Image[0].Bounds().Dx()
		height = images.Image[0].Bounds().Dy()
	}

	// Create canvas for compositing frames
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))

	var cols int
	var decodedEmote DecodedImage
	for i, srcFrame := range images.Image {
		// Get disposal method for this frame
		disposal := byte(0)
		if i < len(images.Disposal) {
			disposal = images.Disposal[i]
		}

		// Draw frame onto canvas at its position
		draw.Draw(canvas, srcFrame.Bounds(), srcFrame, srcFrame.Bounds().Min, draw.Over)

		// Convert the composited canvas to a frame
		// We need to copy the canvas since we'll modify it for the next frame
		compositedFrame := image.NewRGBA(canvas.Bounds())
		draw.Draw(compositedFrame, compositedFrame.Bounds(), canvas, image.Point{}, draw.Src)

		frame, c, err := d.convertImageFrame(compositedFrame, unit, i)
		if err != nil {
			return DecodedImage{}, err
		}

		if i == 0 {
			cols = c
		}

		frame.DelayInMS = images.Delay[i] * 10 // Delay is in centiseconds (1/100s)
		decodedEmote.Images = append(decodedEmote.Images, frame)

		// Handle disposal for next frame
		switch disposal {
		case gif.DisposalBackground:
			// Clear the frame area to background/transparent
			draw.Draw(canvas, srcFrame.Bounds(), image.NewUniform(color.Transparent), image.Point{}, draw.Src)
		case gif.DisposalPrevious:
			// Restore to previous state - for simplicity, clear to transparent
			// A proper implementation would save/restore the canvas state
			draw.Draw(canvas, srcFrame.Bounds(), image.NewUniform(color.Transparent), image.Point{}, draw.Src)
			// DisposalNone (0) or DisposalNone (1): leave canvas as-is
		}
	}

	decodedEmote.Cols = cols

	return decodedEmote, nil
}

func (d *DisplayManager) convertAnimatedWebP(r io.Reader, unit DisplayUnit) (DecodedImage, error) {
	images, err := awebp.DecodeAll(r)
	if err != nil {
		return DecodedImage{}, fmt.Errorf("failed to convert animated webp: %w", err)
	}

	var cols int
	var decodedEmote DecodedImage
	for i, img := range images.Image {
		frame, c, err := d.convertImageFrame(img, unit, i)
		if err != nil {
			return DecodedImage{}, err
		}

		if i == 0 {
			cols = c
		}

		frame.DelayInMS = images.Delay[i] // Delay is already in milliseconds
		decodedEmote.Images = append(decodedEmote.Images, frame)
	}

	decodedEmote.Cols = cols

	return decodedEmote, nil
}

func (d *DisplayManager) convertDefault(r io.Reader, unit DisplayUnit) (DecodedImage, error) {
	img, format, err := image.Decode(r)
	if err != nil {
		log.Logger.Error().Err(err).Str("format", format).Send()
		return DecodedImage{}, fmt.Errorf("failed to convert %s: %w", format, err)
	}

	frame, cols, err := d.convertImageFrame(img, unit, 0)
	if err != nil {
		return DecodedImage{}, err
	}

	return DecodedImage{
		Cols: cols,
		Images: []DecodedImageFrame{
			frame,
		},
	}, nil
}

func (d *DisplayManager) convertImageFrame(img image.Image, unit DisplayUnit, offset int) (DecodedImageFrame, int, error) {
	bounds := img.Bounds()
	height := bounds.Dy()
	width := bounds.Dx()

	// Apply right padding if specified
	if unit.RightPadding > 0 {
		img = addRightPadding(img, unit.RightPadding)
		bounds = img.Bounds()
		width = bounds.Dx()
	}

	ratio := d.cellHeight / float32(height)
	width = int(math.Round(float64(float32(width) * ratio)))
	cols := int(math.Ceil(float64(float32(width) / d.cellWidth)))

	encodedBytes := imageToKittyBytes(img)
	p, err := d.saveKittyFormattedImage(encodedBytes, unit, offset)
	if err != nil {
		log.Logger.Err(err).Send()
		return DecodedImageFrame{}, 0, err
	}

	encodedPath := base64.StdEncoding.EncodeToString([]byte(p))

	return DecodedImageFrame{
		Width:       bounds.Dx(),
		Height:      bounds.Dy(),
		EncodedPath: encodedPath,
	}, cols, nil
}

func (d *DisplayManager) cacheDecodedImage(decoded DecodedImage, unit DisplayUnit) error {
	cacheDir, err := d.createGetCacheDirectory(unit.Directory)
	if err != nil {
		return err
	}

	metaImageFilePath := filepath.Join(cacheDir, fmt.Sprintf("%s.json", filepath.Clean(unit.ID)))
	//log.Logger.Info().Str("path", metaImageFilePath).Msg("trying to cache decoded")

	f, err := d.fs.Create(metaImageFilePath)
	if err != nil {
		return err
	}

	defer f.Close()

	encoded, err := easyjson.Marshal(decoded)
	if err != nil {
		return err
	}

	_, err = f.Write(encoded)
	if err != nil {
		return err
	}

	return nil
}

func (d *DisplayManager) saveKittyFormattedImage(buff []byte, unit DisplayUnit, offset int) (string, error) {
	cacheDir, err := d.createGetCacheDirectory(unit.Directory)
	if err != nil {
		return "", err
	}

	path := filepath.Join(cacheDir, fmt.Sprintf("%s.%d", filepath.Clean(unit.ID), offset))

	f, err := d.fs.Create(path)
	if err != nil {
		return "", err
	}

	defer f.Close()

	w := zlib.NewWriter(f)
	if _, err := w.Write(buff); err != nil {
		return "", fmt.Errorf("failed to write zlib compressed to %s: %w", path, err)
	}

	if err := w.Close(); err != nil {
		return "", fmt.Errorf("failed to close zlib compressed writer to %s: %w", path, err)
	}

	return f.Name(), nil
}

func (d *DisplayManager) openCached(unit DisplayUnit) (DecodedImage, bool, error) {
	dir, err := d.createGetCacheDirectory(unit.Directory)
	if err != nil {
		return DecodedImage{}, false, err
	}

	metaImageFilePath := filepath.Join(dir, fmt.Sprintf("%s.json", filepath.Clean(unit.ID)))

	//log.Logger.Info().Str("path", metaImageFilePath).Msg("trying to open")

	data, err := afero.ReadFile(d.fs, metaImageFilePath)
	if err != nil {
		if errors.Is(err, afero.ErrFileNotFound) {
			return DecodedImage{}, false, nil
		}

		return DecodedImage{}, false, err
	}

	var decoded DecodedImage
	if err := easyjson.Unmarshal(data, &decoded); err != nil {
		return decoded, false, err
	}

	return decoded, true, nil
}

func (d *DisplayManager) createGetCacheDirectory(dir string) (string, error) {
	path := filepath.Join(BaseImageDirectory, dir)

	if err := d.fs.MkdirAll(path, 0o755); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return path, nil
		}

		return "", nil
	}

	return path, nil
}

// addRightPadding creates a new image with transparent padding on the right side.
func addRightPadding(img image.Image, padding int) image.Image {
	bounds := img.Bounds()
	newWidth := bounds.Dx() + padding
	newHeight := bounds.Dy()

	padded := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	draw.Draw(padded, bounds, img, bounds.Min, draw.Src)

	return padded
}

func imageToKittyBytes(img image.Image) []byte {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	buff := make([]byte, width*height*4) // 4 bytes per pixel

	// Fast path for *image.RGBA (most common after compositing)
	if rgba, ok := img.(*image.RGBA); ok {
		i := 0
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			rowStart := (y-rgba.Rect.Min.Y)*rgba.Stride + (bounds.Min.X-rgba.Rect.Min.X)*4
			for x := 0; x < width; x++ {
				off := rowStart + x*4
				buff[i] = rgba.Pix[off]     // R
				buff[i+1] = rgba.Pix[off+1] // G
				buff[i+2] = rgba.Pix[off+2] // B
				buff[i+3] = rgba.Pix[off+3] // A
				i += 4
			}
		}
		return buff
	}

	// Fast path for *image.NRGBA
	if nrgba, ok := img.(*image.NRGBA); ok {
		i := 0
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			rowStart := (y-nrgba.Rect.Min.Y)*nrgba.Stride + (bounds.Min.X-nrgba.Rect.Min.X)*4
			for x := 0; x < width; x++ {
				off := rowStart + x*4
				// NRGBA stores non-premultiplied, kitty expects premultiplied
				a := nrgba.Pix[off+3]
				if a == 255 {
					buff[i] = nrgba.Pix[off]
					buff[i+1] = nrgba.Pix[off+1]
					buff[i+2] = nrgba.Pix[off+2]
				} else if a == 0 {
					// fully transparent
				} else {
					// premultiply
					buff[i] = uint8(uint16(nrgba.Pix[off]) * uint16(a) / 255)
					buff[i+1] = uint8(uint16(nrgba.Pix[off+1]) * uint16(a) / 255)
					buff[i+2] = uint8(uint16(nrgba.Pix[off+2]) * uint16(a) / 255)
				}
				buff[i+3] = a
				i += 4
			}
		}
		return buff
	}

	// Fallback for other image types (uses slower img.At interface)
	i := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			buff[i] = byte(r >> 8)
			buff[i+1] = byte(g >> 8)
			buff[i+2] = byte(b >> 8)
			buff[i+3] = byte(a >> 8)
			i += 4
		}
	}

	return buff
}
