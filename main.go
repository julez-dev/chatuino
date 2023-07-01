package main

import (
	"fmt"
	"os"

	"github.com/dolmen-go/kittyimg"
	"github.com/julez-dev/chatuino/emote"
)

const logFileName = "log.txt"

func main() {
	// ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	// defer cancel()

	// f, err := setupLogFile()
	// if err != nil {
	// 	fmt.Printf("Error while opening log file: %v", err)
	// 	os.Exit(1)
	// }
	// defer f.Close()

	// logger := zerolog.New(f).With().
	// 	Timestamp().Logger()

	// p := tea.NewProgram(ui.New(ctx, logger), tea.WithContext(ctx), tea.WithAltScreen())
	// if _, err := p.Run(); err != nil {
	// 	fmt.Printf("Error while running application: %v", err)
	// 	os.Exit(1)
	// }

	f, err := os.Open("./testdata/pepeLaugh.webp")

	if err != nil {
		fmt.Println(err)
		return
	}

	img, err := emote.Decode(f)

	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Print("test ")
	err = kittyimg.Fprint(os.Stdout, img)

	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Print("a message")

	// str, err := emote.ImageToString(img)

	// if err != nil {
	// 	fmt.Println(err)
	// 	return
	// }

	// fmt.Println(str)
}

func setupLogFile() (*os.File, error) {
	f, err := os.OpenFile(logFileName, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}

	return f, nil
}
