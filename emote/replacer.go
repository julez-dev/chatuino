package emote

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"math"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/adrg/xdg"
	"github.com/charmbracelet/lipgloss"
	"github.com/gen2brain/avif"
	"github.com/julez-dev/chatuino/save"
	"github.com/mailru/easyjson"
	"github.com/rs/zerolog/log"
	_ "golang.org/x/image/webp"
)

var errUnsupportedAnimatedFormat = errors.New("emote is animated but in non supported format")

//easyjson:json
type DecodedEmote struct {
	ID     int            `json:"-"`
	Cols   int            `json:"cols"`
	Images []DecodedImage `json:"images"`
}

//easyjson:json
type DecodedImage struct {
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	EncodedPath string `json:"encoded_path"`
	DelayInMS   int    `json:"delay_in_ms"`
}

func (d DecodedEmote) PrepareCommand() string {
	// not animated
	if len(d.Images) == 1 {
		transmitCMD := fmt.Sprintf("\x1b_Gf=32,i=%d,t=f,q=2,s=%d,v=%d;%s\x1b\\", d.ID, d.Images[0].Width, d.Images[0].Height, d.Images[0].EncodedPath)
		placementCMD := fmt.Sprintf("\x1b_Ga=p,i=%d,p=%d,q=2,U=1,r=1,c=%d\x1b\\", d.ID, d.ID, d.Cols)
		return transmitCMD + placementCMD
	}

	// is animated

	var b strings.Builder

	// transmit first image
	fmt.Fprintf(&b, "\033_Gf=32,i=%d,t=f,q=2,s=%d,v=%d;%s\033\\", d.ID, d.Images[0].Width, d.Images[0].Height, d.Images[0].EncodedPath)

	// send first frame
	fmt.Fprintf(&b, "\033_Ga=a,i=%d,r=1,z=%d,q=2;\033\\", d.ID, d.Images[0].DelayInMS)

	// send each frame after first image
	for img := range slices.Values(d.Images[1:]) {
		fmt.Fprintf(&b, "\033_Ga=f,i=%d,t=t,f=32,s=%d,v=%d,z=%d,q=2;%s\033\\", d.ID, img.Width, img.Height, img.DelayInMS, img.EncodedPath)
	}

	// start animation
	fmt.Fprintf(&b, "\033_Ga=a,i=%d,s=3,v=1,q=2;\033\\", d.ID)

	// create virtual placement
	fmt.Fprintf(&b, "\x1b_Ga=p,i=%d,p=%d,q=2,U=1,r=1,c=%d\x1b\\", d.ID, d.ID, d.Cols)

	return b.String()
}

func (d DecodedEmote) DisplayUnicodePlaceholder() string {
	r, g, b := intToRGB(d.ID)
	return fmt.Sprintf("\033[38;2;%d;%d;%dm%s\033[39m", r, g, b, strings.Repeat("\U0010EEEE", d.Cols))
}

func intToRGB(i int) (byte, byte, byte) {
	return byte(i >> 16), byte(i >> 8), byte(i)
}

type EmoteStore interface {
	GetByTextAllChannels(text string) (Emote, bool)
	GetByText(channelID, text string) (Emote, bool)
}

type Replacer struct {
	store          EmoteStore
	httpClient     *http.Client
	enableGraphics bool

	cellWidth, cellHeight float32

	// The documentation for sync.Map suggests that our use-case is perfect for a sync.Map instead of a mutex because:
	// A key is only written once
	// A entry is read multiple times
	// Only grow like cache
	placedEmotes *sync.Map

	openCached         func(Emote) (DecodedEmote, bool, error)
	saveCached         func(Emote, DecodedEmote) error
	createEncodedImage func(buff []byte, e Emote, offset int) (string, error)
	lastImageID        atomic.Int32

	stvStyle  lipgloss.Style
	ttvStyle  lipgloss.Style
	bttvStyle lipgloss.Style
}

func NewReplacer(httpClient *http.Client, store EmoteStore, enableGraphics bool, cellWidth, cellHeight float32, theme save.Theme) *Replacer {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Replacer{
		enableGraphics: enableGraphics,
		cellWidth:      cellWidth,
		cellHeight:     cellHeight,
		store:          store,
		httpClient:     httpClient,
		placedEmotes:   &sync.Map{},

		openCached:         fsOpenCached,
		saveCached:         SaveCache,
		createEncodedImage: saveKittyFormattedImage,

		stvStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color(theme.SevenTVEmoteColor)),
		ttvStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color(theme.TwitchTVEmoteColor)),
		bttvStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.BetterTTVEmoteColor)),
	}
}

func (i *Replacer) Replace(channelID, content string) (string, string, error) {
	words := strings.Split(content, " ")

	var cmd strings.Builder
	for windex, word := range words {
		var (
			emote   Emote
			isEmote bool
		)

		if channelID == "" {
			emote, isEmote = i.store.GetByTextAllChannels(word)
		} else {
			emote, isEmote = i.store.GetByText(channelID, word)
		}

		if !isEmote {
			continue
		}

		// graphics not enabled, replace with colored emote
		if !i.enableGraphics {
			words[windex] = i.replaceEmoteColored(emote)
			continue
		}

		// emote was already placed before, reuse virtual placement and replace emote text with unicode placeholders

		if decoded, is := i.placedEmotes.Load(emote.Text); is {
			words[windex] = decoded.(DecodedEmote).DisplayUnicodePlaceholder()
			continue
		}

		incrementID := i.lastImageID.Add(1)

		cachedDecoded, isCached, err := i.openCached(emote)
		if err != nil {
			return "", "", fmt.Errorf("failed opening cache file for %s: %w", emote.Text, err)
		}

		if isCached {
			cachedDecoded.ID = int(incrementID)
			i.placedEmotes.Store(emote.Text, cachedDecoded)
			cmd.WriteString(cachedDecoded.PrepareCommand())
			words[windex] = cachedDecoded.DisplayUnicodePlaceholder()
			//log.Logger.Info().Any("id", cachedDecoded.ID).Msg("cache fs hit")
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
		decoded, err := i.ConvertEmote(emote, imageBody)
		if err != nil {
			log.Logger.Err(err).Any("emote", emote).Send()
			words[windex] = i.replaceEmoteColored(emote)
			continue
		}
		decoded.ID = int(incrementID)

		// add to cache
		i.placedEmotes.Store(emote.Text, decoded)

		//  - step4: Create kitty CMD to transfer emote data
		//  - step5: Create Placement
		cmd.WriteString(decoded.PrepareCommand())

		//  - step6: Replace emote text with placeholder
		words[windex] = decoded.DisplayUnicodePlaceholder()

		// save to filesystem cache if not already cached
		if err := i.saveCached(emote, decoded); err != nil {
			return "", "", fmt.Errorf("failed saving cache data for emote %s: %w", emote.Text, err)
		}
		log.Logger.Info().Any("emote", emote).Msg("saved new cache entry")
	}

	return cmd.String(), strings.Join(words, " "), nil
}

func (i *Replacer) ConvertEmote(e Emote, r io.Reader) (DecodedEmote, error) {
	if path.Ext(e.URL) == ".avif" {
		return i.convertAnimatedAvif(e, r)
	}

	if e.IsAnimated {
		return DecodedEmote{}, errUnsupportedAnimatedFormat
	}

	return i.convertDefault(e, r)
}

func (ij *Replacer) convertDefault(e Emote, r io.Reader) (DecodedEmote, error) {
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

	encodedBytes := imageToKittyBytes(img)
	p, err := ij.createEncodedImage(encodedBytes, e, 0)
	if err != nil {
		log.Logger.Err(err).Send()
		return DecodedEmote{}, err
	}

	encodedPath := base64.StdEncoding.EncodeToString([]byte(p))

	return DecodedEmote{
		Cols: cols,
		Images: []DecodedImage{
			{
				Width:       bounds.Dx(),
				Height:      bounds.Dy(),
				EncodedPath: encodedPath,
			},
		},
	}, nil
}

func (ij *Replacer) convertAnimatedAvif(e Emote, r io.Reader) (DecodedEmote, error) {
	images, err := avif.DecodeAll(r)
	if err != nil {
		return DecodedEmote{}, err
	}

	var cols int
	var decodedEmote DecodedEmote
	for i, img := range images.Image {
		encodedBytes := imageToKittyBytes(img)
		p, err := ij.createEncodedImage(encodedBytes, e, i)
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

		decodedEmote.Images = append(decodedEmote.Images, DecodedImage{
			Width:       bounds.Dx(),
			Height:      bounds.Dy(),
			EncodedPath: encodedPath,
			DelayInMS:   int(images.Delay[i] * 1000),
		})
	}

	decodedEmote.Cols = cols

	return decodedEmote, nil
}

func (i *Replacer) fetchEmote(ctx context.Context, reqURL string) (io.ReadCloser, error) {
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

func (i *Replacer) replaceEmoteColored(emote Emote) string {
	switch emote.Platform {
	case Twitch:
		return i.ttvStyle.Render(emote.Text)
	case SevenTV:
		return i.stvStyle.Render(emote.Text)
	case BTTV:
		return i.bttvStyle.Render(emote.Text)
	}

	return emote.Text
}

func EmoteCacheDir() (string, error) {
	path := filepath.Join(xdg.DataHome, "chatuino", "emote")

	if err := os.MkdirAll(path, 0o755); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return path, nil
		}

		return "", nil
	}

	return path, nil
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

func SaveCache(e Emote, dec DecodedEmote) error {
	dir, err := EmoteCacheDir()
	if err != nil {
		return err
	}

	fileName := strings.ToLower(fmt.Sprintf("%s.%s.json", e.Platform.String(), e.ID))
	filePath := filepath.Join(dir, fileName)

	f, err := os.Create(filePath)
	if err != nil {
		return err
	}

	defer f.Close()

	encoded, err := easyjson.Marshal(dec)
	if err != nil {
		return err
	}

	_, err = f.Write(encoded)
	if err != nil {
		return err
	}

	return nil
}

func fsOpenCached(e Emote) (DecodedEmote, bool, error) {
	dir, err := EmoteCacheDir()
	if err != nil {
		return DecodedEmote{}, false, err
	}

	metaFile := strings.ToLower(fmt.Sprintf("%s.%s.json", e.Platform.String(), e.ID))
	path := filepath.Join(dir, metaFile)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DecodedEmote{}, false, nil
		}

		return DecodedEmote{}, false, err
	}

	var decoded DecodedEmote
	if err := easyjson.Unmarshal(data, &decoded); err != nil {
		return decoded, false, err
	}

	return decoded, true, nil
}

func saveKittyFormattedImage(buff []byte, e Emote, offset int) (string, error) {
	emoteDir, err := EmoteCacheDir()
	if err != nil {
		return "", err
	}

	imagePath := strings.ToLower(fmt.Sprintf("%s.%s.%d", e.Platform.String(), e.ID, offset))
	path := filepath.Join(emoteDir, imagePath)

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
