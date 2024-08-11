package emote

import (
	"context"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gen2brain/avif"
	_ "github.com/gen2brain/avif"
	"github.com/rs/zerolog/log"
	_ "golang.org/x/image/webp"

	"github.com/julez-dev/chatuino/twitch/command"
)

type EmoteStore interface {
	GetByText(string, string) (Emote, bool)
}

const imageTmpPrefix = "chatuino.tty-graphics-protocol."

type DecodedEmote struct {
	id     int
	cols   int
	images []DecodedImage
}

func (d DecodedEmote) PrepareCommand() string {
	// not animated
	if len(d.images) == 1 {
		transmitCMD := fmt.Sprintf("\x1b_Gf=32,i=%d,t=t,q=2,s=%d,v=%d;%s\x1b\\", d.id, d.images[0].width, d.images[0].height, d.images[0].encodedPath)
		placementCMD := fmt.Sprintf("\x1b_Ga=p,i=%d,p=%d,q=2,U=1,r=1,c=%d\x1b\\", d.id, d.id, d.cols)
		return transmitCMD + placementCMD
	}

	// is animated

	var b strings.Builder

	// transmit first image
	fmt.Fprintf(&b, "\033_Gf=32,i=%d,t=t,q=2,s=%d,v=%d;%s\033\\", d.id, d.images[0].width, d.images[0].height, d.images[0].encodedPath)

	// send first frame
	fmt.Fprintf(&b, "\033_Ga=a,i=%d,r=1,z=%d,q=2;\033\\", d.id, d.images[0].delayinMS)

	// send each frame after first image
	for _, img := range d.images[1:] {
		fmt.Fprintf(&b, "\033_Ga=f,i=%d,t=t,f=32,s=%d,v=%d,z=%d,q=2;%s\033\\", d.id, img.width, img.height, img.delayinMS, img.encodedPath)
	}

	// start animation
	fmt.Fprintf(&b, "\033_Ga=a,i=%d,s=3,v=1,q=2;\033\\", d.id)

	// create virtual placement
	fmt.Fprintf(&b, "\x1b_Ga=p,i=%d,p=%d,q=2,U=1,r=1,c=%d\x1b\\", d.id, d.id, d.cols)

	return b.String()
}

func (d DecodedEmote) DisplayUnicodePlaceholder() string {
	return fmt.Sprintf("\033[38;5;%dm\033[58:5:%dm%s\033[59m\033[39m", d.id, d.id, strings.Repeat("\U0010EEEE", d.cols))
}

type DecodedImage struct {
	width       int
	height      int
	encodedPath string
	delayinMS   int
}

type Injector struct {
	store      EmoteStore
	httpClient *http.Client

	cellWidth, cellHeight float32
	placedEmotes          map[string]DecodedEmote

	lastImageID int
}

func NewInjector(httpClient *http.Client, store EmoteStore, cellWidth, cellHeight float32) *Injector {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Injector{
		cellWidth:    cellWidth,
		cellHeight:   cellHeight,
		store:        store,
		httpClient:   httpClient,
		placedEmotes: map[string]DecodedEmote{},
	}
}

func (i *Injector) Parse(msg *command.PrivateMessage) (string, string, error) {
	words := strings.Split(msg.Message, " ")

	var cmd strings.Builder
	for windex, word := range words {
		log.Logger.Info().Str("room-id", msg.RoomID).Str("word", word).Send()
		emote, isEmote := i.store.GetByText(msg.RoomID, word)

		if !isEmote {
			continue
		}

		// emote was already placed before, reuse virtual placement and replace emote text with unicode placeholders
		if decoded, is := i.placedEmotes[emote.Text]; is {
			words[windex] = decoded.DisplayUnicodePlaceholder()
			continue
		}

		// emote was not placed before
		//  - step1: Download emote
		imageBody, err := i.fetchEmote(context.Background(), emote.URL)
		if err != nil {
			return "", "", fmt.Errorf("failed fetching emote %s: %w", emote.Text, err)
		}

		defer imageBody.Close()

		//  - step2: Convert emote data to kittys fomart
		//  - step3: Save emote in cache directory
		decoded, err := i.convertEmote(emote, imageBody)
		if err != nil {
			return "", "", err
		}

		// add to cache
		i.placedEmotes[emote.Text] = decoded

		//  - step4: Create kitty CMD to transfer emote data
		//  - step5: Create Placement
		cmd.WriteString(decoded.PrepareCommand())

		//  - step6: Replace emotetext with placeholder
		words[windex] = decoded.DisplayUnicodePlaceholder()
	}

	return cmd.String(), strings.Join(words, " "), nil
}

func (i *Injector) fetchEmote(ctx context.Context, reqURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := i.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code, got: %d", resp.StatusCode)
	}

	return resp.Body, nil
}

func (i *Injector) convertEmote(e Emote, r io.Reader) (DecodedEmote, error) {
	if path.Ext(e.URL) == ".avif" {
		return i.convertAnimatedAvif(r)
	}

	return i.convertDefault(r)
}

func (ij *Injector) convertDefault(r io.Reader) (DecodedEmote, error) {
	img, format, err := image.Decode(r)
	if err != nil {
		log.Logger.Error().Err(err).Str("format", format).Send()
		return DecodedEmote{}, err
	}

	bounds := img.Bounds()
	height := bounds.Dy()
	width := bounds.Dx()

	ratio := ij.cellHeight / float32(height)
	width = int(math.Round(float64(float32(width) * ratio)))
	cols := int(math.Ceil(float64(float32(width) / ij.cellWidth)))

	ij.lastImageID += 1

	encodedBytes := imageToKittyBytes(img)
	p, err := saveRawImage(encodedBytes, fmt.Sprintf("%d", ij.lastImageID))
	if err != nil {
		log.Logger.Err(err).Send()
		return DecodedEmote{}, err
	}

	log.Logger.Info().Str("path", p).Send()

	encodedPath := base64.StdEncoding.EncodeToString([]byte(p))

	return DecodedEmote{
		id:   ij.lastImageID,
		cols: cols,
		images: []DecodedImage{
			{
				width:       bounds.Dx(),
				height:      bounds.Dy(),
				encodedPath: encodedPath,
			},
		},
	}, nil

}

func (ij *Injector) convertAnimatedAvif(r io.Reader) (DecodedEmote, error) {
	images, err := avif.DecodeAll(r)
	if err != nil {
		return DecodedEmote{}, err
	}

	ij.lastImageID += 1

	var cols int
	var decodedEmote DecodedEmote
	for i, img := range images.Image {
		encodedBytes := imageToKittyBytes(img)
		p, err := saveRawImage(encodedBytes, fmt.Sprintf("%d.%d", ij.lastImageID, i))

		if err != nil {
			return DecodedEmote{}, err
		}

		bounds := img.Bounds()
		height := bounds.Dy()
		width := bounds.Dx()

		ratio := ij.cellHeight / float32(height)
		width = int(math.Round(float64(float32(width) * ratio)))

		encodedPath := base64.StdEncoding.EncodeToString([]byte(p))

		if i == 0 {
			cols = int(math.Ceil(float64(float32(width) / ij.cellWidth)))
		}

		decodedEmote.images = append(decodedEmote.images, DecodedImage{
			width:       bounds.Dx(),
			height:      bounds.Dy(),
			encodedPath: encodedPath,
			delayinMS:   int(images.Delay[i] * 1000),
		})
	}

	decodedEmote.cols = cols
	decodedEmote.id = ij.lastImageID

	return decodedEmote, nil
}

func imageToKittyBytes(img image.Image) []byte {
	bounds := img.Bounds()

	buff := make([]byte, 0, bounds.Dx()*bounds.Dy()*4) // 4 bytes per pixel

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			pixel := img.At(x, y)
			r, g, b, a := pixel.RGBA()

			r = r >> 8
			g = g >> 8
			b = b >> 8
			a = a >> 8

			buff = append(buff, byte(r), byte(g), byte(b), byte(a))
		}
	}

	return buff
}

func saveRawImage(buff []byte, id string) (string, error) {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("%s%s", imageTmpPrefix, id))

	f, err := os.Create(path)
	if err != nil {
		return "", err
	}

	defer f.Close()
	_, err = f.Write(buff)
	if err != nil {
		return "", err
	}

	return f.Name(), nil
}
