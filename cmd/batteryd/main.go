package main

import (
	"fmt"
	"io"
	"os"
)

const usageText = "用法: batteryd <daemon|once|json>\n"

func printUsage(w io.Writer) {
	fmt.Fprint(w, usageText)
}

func main() {
	cmd := "daemon"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	var err error
	switch cmd {
	case "daemon":
		err = runDaemon()
	case "once":
		err = runOnce()
	case "json":
		err = runJson()
	default:
		printUsage(os.Stderr)
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runDaemon() error {
	return nil
}

func runOnce() error {
	return nil
}

func runJson() error {
	return nil
}
