package kittyimg

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/syncmap"
)

func TestDisplayManager_Convert_Cached(t *testing.T) {
	t.Parallel()

	fs := afero.NewMemMapFs()
	dm := NewDisplayManager(fs, CellSize{Width: 10, Height: 10}, DisplayOptions{Rows: 1})

	// Pre-cache a decoded image
	cachedImage := DecodedImage{
		Cols: 2,
		Images: []DecodedImageFrame{
			{
				Width:       10,
				Height:      10,
				EncodedPath: "L3BhdGgvdG8vY2FjaGVkLnBuZw==", // base64 encoded "/path/to/cached.png"
			},
		},
	}

	unit := DisplayUnit{
		ID:         "test-emote",
		Directory:  "emote",
		IsAnimated: false,
		Load: func() (io.ReadCloser, string, error) {
			t.Fatal("Load should not be called for cached image")
			return nil, "", nil
		},
	}

	// Manually cache the image
	err := dm.cacheDecodedImage(cachedImage, unit)
	require.NoError(t, err)

	// Convert should use cached version
	result, err := dm.Convert(unit)
	require.NoError(t, err)

	require.NotEmpty(t, result.PrepareCommand)
	require.Contains(t, result.ReplacementText, "\U0010eeee")
	require.Contains(t, result.PrepareCommand, "L3BhdGgvdG8vY2FjaGVkLnBuZw==")
}

func TestDisplayManager_Convert_SessionCache(t *testing.T) {
	// Reset global state for this test
	globalImagePlacementIDCounter.Store(0)
	globalPlacedImages = &syncmap.Map{}

	fs := afero.NewMemMapFs()
	dm := NewDisplayManager(fs, CellSize{Width: 10, Height: 10}, DisplayOptions{Rows: 1})

	emoteData, err := os.ReadFile("../emote/testdata/pepeLaugh.webp")
	require.NoError(t, err)

	unit := DisplayUnit{
		ID:         "test-emote",
		Directory:  "emote",
		IsAnimated: false,
		Load: func() (io.ReadCloser, string, error) {
			return io.NopCloser(bytes.NewReader(emoteData)), "image/webp", nil
		},
	}

	// First conversion - should load and cache
	result1, err := dm.Convert(unit)
	require.NoError(t, err)
	require.NotEmpty(t, result1.PrepareCommand)
	require.Contains(t, result1.ReplacementText, "\U0010eeee")

	// Second conversion - should use session cache (no prepare command)
	result2, err := dm.Convert(unit)
	require.NoError(t, err)
	require.Empty(t, result2.PrepareCommand, "should not resend placement command from session cache")
	require.Equal(t, result1.ReplacementText, result2.ReplacementText)
}

func TestDisplayManager_Convert_FreshDownload(t *testing.T) {
	// Reset global state for this test
	globalImagePlacementIDCounter.Store(0)
	globalPlacedImages = &syncmap.Map{}

	fs := afero.NewMemMapFs()
	dm := NewDisplayManager(fs, CellSize{Width: 400, Height: 400}, DisplayOptions{Rows: 1})

	emoteData, err := os.ReadFile("../emote/testdata/pepeLaugh.webp")
	require.NoError(t, err)

	unit := DisplayUnit{
		ID:         "fresh-emote",
		Directory:  "emote",
		IsAnimated: false,
		Load: func() (io.ReadCloser, string, error) {
			return io.NopCloser(bytes.NewReader(emoteData)), "image/webp", nil
		},
	}

	result, err := dm.Convert(unit)
	require.NoError(t, err)

	require.NotEmpty(t, result.PrepareCommand)
	require.Contains(t, result.PrepareCommand, "\x1b_Gf=32,i=1,t=f,q=2")
	require.Contains(t, result.PrepareCommand, "\x1b_Ga=p,i=1,p=1,q=2")
	require.Contains(t, result.ReplacementText, "\x1b[38;2;0;0;1m\U0010eeee\x1b[39m")
}

// TestDisplayManager_Convert_Codecs exercises the decoder paths backed by the
// third-party AVIF and animated-WebP modules (avif.DecodeAll, webp.DecodeAll).
// Those dependencies sit on 0.x versions, where a minor bump may change behavior
// without breaking the build, so a compile-only check is not enough: real image
// data has to go through the decoders.
func TestDisplayManager_Convert_Codecs(t *testing.T) {
	tests := []struct {
		name        string
		testdata    string
		contentType string
	}{
		{name: "avif", testdata: "../emote/testdata/test.avif", contentType: "image/avif"},
		{name: "animated webp", testdata: "../emote/testdata/animated.webp", contentType: "image/webp"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset global state for this test
			globalImagePlacementIDCounter.Store(0)
			globalPlacedImages = &syncmap.Map{}

			dm := NewDisplayManager(afero.NewMemMapFs(), CellSize{Width: 400, Height: 400}, DisplayOptions{Rows: 1})

			data, err := os.ReadFile(tt.testdata)
			require.NoError(t, err)
			require.NotEmpty(t, data)

			unit := DisplayUnit{
				ID:         tt.name,
				Directory:  "emote",
				IsAnimated: true,
				Load: func() (io.ReadCloser, string, error) {
					return io.NopCloser(bytes.NewReader(data)), tt.contentType, nil
				},
			}

			result, err := dm.Convert(unit)
			require.NoError(t, err)
			require.NotEmpty(t, result.PrepareCommand)
			require.NotEmpty(t, result.ReplacementText)
		})
	}
}

func TestDisplayManager_Convert_AnimatedUnsupported(t *testing.T) {
	// Reset global state for this test
	globalImagePlacementIDCounter.Store(0)
	globalPlacedImages = &syncmap.Map{}

	fs := afero.NewMemMapFs()
	dm := NewDisplayManager(fs, CellSize{Width: 400, Height: 400}, DisplayOptions{Rows: 1})

	unit := DisplayUnit{
		ID:         "animated-webp",
		Directory:  "emote",
		IsAnimated: true, // png is not supported for animation
		Load: func() (io.ReadCloser, string, error) {
			return io.NopCloser(bytes.NewReader([]byte{})), "image/png", nil
		},
	}

	result, err := dm.Convert(unit)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnsupportedAnimatedFormat)
	require.Empty(t, result.PrepareCommand)
	require.Empty(t, result.ReplacementText)
}

func TestDecodedImage_PrepareCommand_Static(t *testing.T) {
	t.Parallel()

	decoded := DecodedImage{
		ID:   1,
		Cols: 2,
		Images: []DecodedImageFrame{
			{
				Width:       20,
				Height:      20,
				EncodedPath: "dGVzdHBhdGg=",
			},
		},
	}

	cmd := decoded.PrepareCommand()
	require.Contains(t, cmd, "\x1b_Gf=32,i=1,t=f,q=2,s=20,v=20,o=z;dGVzdHBhdGg=\x1b\\")
	require.Contains(t, cmd, "\x1b_Ga=p,i=1,p=1,q=2,U=1,r=1,c=2\x1b\\")
}

func TestDecodedImage_PrepareCommand_Animated(t *testing.T) {
	t.Parallel()

	decoded := DecodedImage{
		ID:   5,
		Cols: 3,
		Images: []DecodedImageFrame{
			{
				Width:       30,
				Height:      30,
				EncodedPath: "ZnJhbWUx",
				DelayInMS:   100,
			},
			{
				Width:       30,
				Height:      30,
				EncodedPath: "ZnJhbWUy",
				DelayInMS:   100,
			},
		},
	}

	cmd := decoded.PrepareCommand()
	// Should contain transmit command for first frame
	require.Contains(t, cmd, "\033_Gf=32,i=5,t=f,q=2,s=30,v=30,o=z;ZnJhbWUx\033\\")
	// Should contain animation start for first frame
	require.Contains(t, cmd, "\033_Ga=a,i=5,r=1,z=100,q=2;\033\\")
	// Should contain subsequent frame
	require.Contains(t, cmd, "\033_Ga=f,i=5,t=t,f=32,s=30,v=30,z=100,q=2,o=z;ZnJhbWUy\033\\")
	// Should start animation
	require.Contains(t, cmd, "\033_Ga=a,i=5,s=3,v=1,q=2;\033\\")
	// Should create placement
	require.Contains(t, cmd, "\x1b_Ga=p,i=5,p=5,q=2,U=1,r=1,c=3\x1b\\")
}

func TestDecodedImage_DisplayUnicodePlaceholder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		id       int32
		cols     int
		expected string
	}{
		{
			name:     "single column - ID 1",
			id:       1,
			cols:     1,
			expected: "\x1b[38;2;0;0;1m\U0010eeee\x1b[39m",
		},
		{
			name:     "two columns - ID 256",
			id:       256,
			cols:     2,
			expected: "\x1b[38;2;0;1;0m\U0010eeee\U0010eeee\x1b[39m",
		},
		{
			name:     "24-bit ID",
			id:       8235331, // 0x7DA943
			cols:     2,
			expected: "\x1b[38;2;125;169;67m\U0010eeee\U0010eeee\x1b[39m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoded := DecodedImage{
				ID:   tt.id,
				Cols: tt.cols,
			}
			result := decoded.DisplayUnicodePlaceholder()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestIntToRGB(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		id      int32
		r, g, b byte
	}{
		{
			name: "ID 1",
			id:   1,
			r:    0, g: 0, b: 1,
		},
		{
			name: "ID 256",
			id:   256,
			r:    0, g: 1, b: 0,
		},
		{
			name: "ID 255",
			id:   255,
			r:    0, g: 0, b: 255,
		},
		{
			name: "24-bit ID",
			id:   8235331, // 0x7DA943
			r:    125, g: 169, b: 67,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b := intToRGB(tt.id)
			require.Equal(t, tt.r, r)
			require.Equal(t, tt.g, g)
			require.Equal(t, tt.b, b)
		})
	}
}

func TestWrapPassthrough(t *testing.T) {
	t.Parallel()

	require.Empty(t, wrapPassthrough(""))

	command := "\x1b_Ga=D\x1b\\"
	wrapped := wrapPassthrough(command)
	require.Equal(t, "\x1bPtmux;\x1b\x1b_Ga=D\x1b\x1b\\\x1b\\", wrapped)
	require.Equal(t, command, strings.ReplaceAll(wrapped[len(passthroughPrefix):len(wrapped)-len(passthroughSuffix)], "\x1b\x1b", "\x1b"))

	// Every ESC inside the payload is doubled so tmux cannot terminate the
	// passthrough at the kitty command's own string terminator.
	payload := wrapped[len(passthroughPrefix) : len(wrapped)-len(passthroughSuffix)]
	for i := 0; i < len(payload); i++ {
		if payload[i] != 0x1b {
			continue
		}
		require.Equal(t, byte(0x1b), payload[i+1], "unpaired ESC at %d", i)
		i++
	}
}

func TestDisplayManager_PassthroughWrapsEveryGraphicsCommand(t *testing.T) {
	globalPlacedImages = &syncmap.Map{}
	t.Cleanup(func() { globalPlacedImages = &syncmap.Map{} })

	dm := NewDisplayManager(afero.NewMemMapFs(), CellSize{Width: 10, Height: 10}, DisplayOptions{Rows: 1, Passthrough: true})
	require.Equal(t, wrapPassthrough("\x1b_Ga=D\x1b\\"), dm.CleanupAllImagesCommand())

	globalPlacedImages.Store("stale", DecodedImage{ID: 7, lastUsed: time.Now().Add(-time.Hour)})
	require.Equal(t, wrapPassthrough("\x1b_Ga=D,i=7,q=2\x1b\\"), dm.CleanupOldImagesCommand(time.Minute))

	plain := NewDisplayManager(afero.NewMemMapFs(), CellSize{Width: 10, Height: 10}, DisplayOptions{Rows: 1})
	require.Equal(t, "\x1b_Ga=D\x1b\\", plain.CleanupAllImagesCommand())
	require.NotContains(t, plain.CleanupAllImagesCommand(), "Ptmux")
}

func TestDisplayManager_ConvertWrapsPrepareCommand(t *testing.T) {
	globalImagePlacementIDCounter.Store(0)
	globalPlacedImages = &syncmap.Map{}
	t.Cleanup(func() { globalPlacedImages = &syncmap.Map{} })

	fs := afero.NewMemMapFs()
	dm := NewDisplayManager(fs, CellSize{Width: 10, Height: 10}, DisplayOptions{Rows: 1, Passthrough: true})
	cachedImage := DecodedImage{
		Cols:   2,
		Images: []DecodedImageFrame{{Width: 10, Height: 10, EncodedPath: "L3BhdGgvdG8vY2FjaGVkLnBuZw=="}},
	}
	unit := DisplayUnit{
		ID:        "test-emote",
		Directory: "emote",
		Load: func() (io.ReadCloser, string, error) {
			t.Fatal("Load should not be called for a cached image")
			return nil, "", nil
		},
	}
	require.NoError(t, dm.cacheDecodedImage(cachedImage, unit))

	result, err := dm.Convert(unit)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(result.PrepareCommand, passthroughPrefix), "graphics command must be wrapped")
	require.NotContains(t, result.ReplacementText, "Ptmux", "placeholder cells are text and must never be wrapped")
	require.Contains(t, result.ReplacementText, "\U0010eeee")
}

func TestDisplayManager_RefreshCellSize(t *testing.T) {
	t.Parallel()

	size := CellSize{Width: 10, Height: 20}
	dm := NewDisplayManager(afero.NewMemMapFs(), size, DisplayOptions{
		Rows: 1,
		CellSize: func() (CellSize, error) {
			return size, nil
		},
	})
	require.Equal(t, 2, dm.colsFor(100, 100), "100px wide at 10px cells")

	size = CellSize{Width: 25, Height: 20}
	dm.RefreshCellSize()
	require.Equal(t, 1, dm.colsFor(100, 100), "same image at 25px cells")

	size = CellSize{}
	dm.RefreshCellSize()
	require.Equal(t, 1, dm.colsFor(100, 100), "an invalid size keeps the last usable one")

	failing := NewDisplayManager(afero.NewMemMapFs(), CellSize{Width: 10, Height: 10}, DisplayOptions{
		Rows: 1,
		CellSize: func() (CellSize, error) {
			return CellSize{}, errors.New("no tty")
		},
	})
	require.Equal(t, 1, failing.colsFor(100, 100))
	failing.RefreshCellSize()
	require.Equal(t, 1, failing.colsFor(100, 100), "a failed refresh keeps the previous size")
}
