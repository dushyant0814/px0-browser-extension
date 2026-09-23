// px0-extension-host is the Chrome Native Messaging host for the px0 browser
// extension. It keeps a bounded cache of forge checkouts and launches the
// user's unmodified px0 binary on them.
package main

import (
	"fmt"
	"os"
)

// version is set at release time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	// Chrome launches the host with the calling extension origin (and, on
	// Windows, a parent window handle) as arguments. Those are not ours to
	// parse; only an explicit -check selects a different mode.
	if len(os.Args) > 1 && (os.Args[1] == "-check" || os.Args[1] == "--check") {
		if err := runCheck(); err != nil {
			fmt.Fprintln(os.Stderr, "px0-extension-host:", err)
			os.Exit(1)
		}
		return
	}
	if err := runNativeHost(os.Stdin, os.Stdout); err != nil {
		// stdout is reserved for framed replies; Chrome logs stderr.
		fmt.Fprintln(os.Stderr, "px0-extension-host:", err)
		os.Exit(1)
	}
}

// runCheck lets the installer confirm the host can find and run px0 before
// Chrome ever starts it with a minimal environment.
func runCheck() error {
	bin, err := resolvePx0()
	if err != nil {
		return err
	}
	ver, err := px0Version(bin)
	if err != nil {
		return err
	}
	fmt.Printf("px0-extension-host %s\npx0: %s (%s)\n", version, bin, ver)
	return nil
}
