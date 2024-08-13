package emote

import (
	"context"
	"encoding/base64"
	"encoding/json"
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

	"github.com/charmbracelet/lipgloss"
	"github.com/gen2brain/avif"
	_ "github.com/gen2brain/avif"
	"github.com/rs/zerolog/log"
	_ "golang.org/x/image/webp"

	"github.com/julez-dev/chatuino/twitch/command"
)

const imageTmpPrefix = "chatuino.tty-graphics-protocol."

var (
	stvStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#0aa6ec"))
	ttvStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#a35df2"))
	bttvStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#d50014"))
)

type DecodedEmote struct {
	ID     int            `json:"-"`
	Cols   int            `json:"cols"`
	Images []DecodedImage `json:"images"`
}

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
	return fmt.Sprintf("\033[38;5;%dm\033[58:5:%dm%s\033[59m\033[39m", d.ID, d.ID, strings.Repeat("\U0010EEEE", d.Cols))
}

type EmoteStore interface {
	GetByText(string, string) (Emote, bool)
}

type Replacer struct {
	store          EmoteStore
	httpClient     *http.Client
	enableGraphics bool

	cellWidth, cellHeight float32
	placedEmotes          map[string]DecodedEmote

	lastImageID int
}

func NewReplacer(httpClient *http.Client, store EmoteStore, enableGraphics bool, cellWidth, cellHeight float32) *Replacer {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Replacer{
		enableGraphics: enableGraphics,
		cellWidth:      cellWidth,
		cellHeight:     cellHeight,
		store:          store,
		httpClient:     httpClient,
		placedEmotes:   map[string]DecodedEmote{},
	}
}

func (i *Replacer) Replace(msg *command.PrivateMessage) (string, string, error) {
	words := strings.Split(msg.Message, " ")

	var cmd strings.Builder
	for windex, word := range words {
		emote, isEmote := i.store.GetByText(msg.RoomID, word)

		if !isEmote {
			continue
		}

		// graphics not enabled, replace with colored emote
		if !i.enableGraphics {
			switch emote.Platform {
			case Twitch:
				words[windex] = ttvStyle.Render(word)
			case SevenTV:
				words[windex] = stvStyle.Render(word)
			case BTTV:
				words[windex] = bttvStyle.Render(word)
			default:
			}

			continue
		}

		// emote was already placed before, reuse virtual placement and replace emote text with unicode placeholders
		if decoded, is := i.placedEmotes[emote.Text]; is {
			words[windex] = decoded.DisplayUnicodePlaceholder()
			continue
		}

		// check if already in local files
		i.lastImageID++

		cachedDecoded, isCached, err := TryOpenCached(emote)
		if err != nil {
			return "", "", fmt.Errorf("failed opening cache file for %s: %w", emote.Text, err)
		}

		if isCached {
			cachedDecoded.ID = i.lastImageID
			i.placedEmotes[emote.Text] = cachedDecoded
			cmd.WriteString(cachedDecoded.PrepareCommand())
			words[windex] = cachedDecoded.DisplayUnicodePlaceholder()
			log.Logger.Info().Any("id", cachedDecoded.ID).Msg("cache fs hit")
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
			return "", "", err
		}
		decoded.ID = i.lastImageID

		// add to cache
		i.placedEmotes[emote.Text] = decoded

		//  - step4: Create kitty CMD to transfer emote data
		//  - step5: Create Placement
		cmd.WriteString(decoded.PrepareCommand())

		//  - step6: Replace emotetext with placeholder
		words[windex] = decoded.DisplayUnicodePlaceholder()

		// save to filesystem cache if not already cached
		if err := CacheEmote(emote, decoded); err != nil {
			return "", "", fmt.Errorf("failed saving cache data for emote %s: %w", emote.Text, err)
		}
		log.Logger.Info().Any("emote", emote).Msg("saved new cache entry")
	}

	return cmd.String(), strings.Join(words, " "), nil
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

func (i *Replacer) ConvertEmote(e Emote, r io.Reader) (DecodedEmote, error) {
	if path.Ext(e.URL) == ".avif" {
		return i.convertAnimatedAvif(e, r)
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

	encodedBytes := ImageToKittyBytes(img)
	p, err := SaveRawImage(encodedBytes, e, 0)
	if err != nil {
		log.Logger.Err(err).Send()
		return DecodedEmote{}, err
	}

	encodedPath := base64.StdEncoding.EncodeToString([]byte(p))

	return DecodedEmote{
		ID:   ij.lastImageID,
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
		encodedBytes := ImageToKittyBytes(img)
		p, err := SaveRawImage(encodedBytes, e, i)
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
	decodedEmote.ID = ij.lastImageID

	return decodedEmote, nil
}

func ImageToKittyBytes(img image.Image) []byte {
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

func SaveRawImage(buff []byte, e Emote, offset int) (string, error) {
	emoteDir, err := EnsureEmoteDirExists()
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

func EnsureEmoteDirExists() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	emoteDir := filepath.Join(home, "chatuino", "emote")
	err = os.MkdirAll(emoteDir, 0o755)
	if err != nil {
		if !errors.Is(err, fs.ErrExist) {
			return "", err
		}
	}

	return emoteDir, nil
}

func TryOpenCached(e Emote) (DecodedEmote, bool, error) {
	// ensure emote dir exists
	emoteDir, err := EnsureEmoteDirExists()
	if err != nil {
		return DecodedEmote{}, false, err
	}

	metaFile := strings.ToLower(fmt.Sprintf("%s.%s.json", e.Platform.String(), e.ID))
	path := filepath.Join(emoteDir, metaFile)

	metafileData, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DecodedEmote{}, false, nil
		}

		return DecodedEmote{}, false, err
	}

	var decoded DecodedEmote
	if err := json.Unmarshal(metafileData, &decoded); err != nil {
		return decoded, false, err
	}

	return decoded, true, nil
}

func CacheEmote(e Emote, decoded DecodedEmote) error {
	emoteDir, err := EnsureEmoteDirExists()
	if err != nil {
		return err
	}

	metaFile := strings.ToLower(fmt.Sprintf("%s.%s.json", e.Platform.String(), e.ID))
	metaFilePath := filepath.Join(emoteDir, metaFile)

	f, err := os.Create(metaFilePath)
	if err != nil {
		return err
	}

	defer f.Close()

	encodedEmoteData, err := json.Marshal(decoded)
	if err != nil {
		return err
	}

	_, err = f.Write(encodedEmoteData)
	if err != nil {
		return err
	}

	return nil
}
