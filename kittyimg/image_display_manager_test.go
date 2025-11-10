package kittyimg

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/syncmap"
)

func TestDisplayManager_Convert_Cached(t *testing.T) {
	t.Parallel()

	fs := afero.NewMemMapFs()
	dm := NewDisplayManager(fs, 10, 10)

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

	assert.NotEmpty(t, result.PrepareCommand)
	assert.Contains(t, result.ReplacementText, "\U0010eeee")
	assert.Contains(t, result.PrepareCommand, "L3BhdGgvdG8vY2FjaGVkLnBuZw==")
}

func TestDisplayManager_Convert_SessionCache(t *testing.T) {
	t.Parallel()

	// Reset global state for this test
	globalImagePlacementIDCounter.Store(0)
	globalPlacedImages = &syncmap.Map{}

	fs := afero.NewMemMapFs()
	dm := NewDisplayManager(fs, 10, 10)

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
	assert.NotEmpty(t, result1.PrepareCommand)
	assert.Contains(t, result1.ReplacementText, "\U0010eeee")

	// Second conversion - should use session cache (no prepare command)
	result2, err := dm.Convert(unit)
	require.NoError(t, err)
	assert.Empty(t, result2.PrepareCommand, "should not resend placement command from session cache")
	assert.Equal(t, result1.ReplacementText, result2.ReplacementText)
}

func TestDisplayManager_Convert_FreshDownload(t *testing.T) {
	t.Parallel()

	// Reset global state for this test
	globalImagePlacementIDCounter.Store(0)
	globalPlacedImages = &syncmap.Map{}

	fs := afero.NewMemMapFs()
	dm := NewDisplayManager(fs, 400, 400)

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

	assert.NotEmpty(t, result.PrepareCommand)
	assert.Contains(t, result.PrepareCommand, "\x1b_Gf=32,i=1,t=f,q=2")
	assert.Contains(t, result.PrepareCommand, "\x1b_Ga=p,i=1,p=1,q=2")
	assert.Contains(t, result.ReplacementText, "\x1b[38;2;0;0;1m\U0010eeee\x1b[39m")
}

func TestDisplayManager_Convert_AnimatedUnsupported(t *testing.T) {
	t.Parallel()

	// Reset global state for this test
	globalImagePlacementIDCounter.Store(0)
	globalPlacedImages = &syncmap.Map{}

	fs := afero.NewMemMapFs()
	dm := NewDisplayManager(fs, 400, 400)

	emoteData, err := os.ReadFile("../emote/testdata/pepeLaugh.webp")
	require.NoError(t, err)

	unit := DisplayUnit{
		ID:         "animated-webp",
		Directory:  "emote",
		IsAnimated: true, // webp is not supported for animation
		Load: func() (io.ReadCloser, string, error) {
			return io.NopCloser(bytes.NewReader(emoteData)), "image/webp", nil
		},
	}

	result, err := dm.Convert(unit)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsupportedAnimatedFormat)
	assert.Empty(t, result.PrepareCommand)
	assert.Empty(t, result.ReplacementText)
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
	assert.Contains(t, cmd, "\x1b_Gf=32,i=1,t=f,q=2,s=20,v=20;dGVzdHBhdGg=\x1b\\")
	assert.Contains(t, cmd, "\x1b_Ga=p,i=1,p=1,q=2,U=1,r=1,c=2\x1b\\")
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
	assert.Contains(t, cmd, "\033_Gf=32,i=5,t=f,q=2,s=30,v=30;ZnJhbWUx\033\\")
	// Should contain animation start for first frame
	assert.Contains(t, cmd, "\033_Ga=a,i=5,r=1,z=100,q=2;\033\\")
	// Should contain subsequent frame
	assert.Contains(t, cmd, "\033_Ga=f,i=5,t=t,f=32,s=30,v=30,z=100,q=2;ZnJhbWUy\033\\")
	// Should start animation
	assert.Contains(t, cmd, "\033_Ga=a,i=5,s=3,v=1,q=2;\033\\")
	// Should create placement
	assert.Contains(t, cmd, "\x1b_Ga=p,i=5,p=5,q=2,U=1,r=1,c=3\x1b\\")
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
			assert.Equal(t, tt.expected, result)
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
			assert.Equal(t, tt.r, r)
			assert.Equal(t, tt.g, g)
			assert.Equal(t, tt.b, b)
		})
	}
}
