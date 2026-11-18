// rex-fingers shows letter hints over the paths, URLs, hashes, and other
// strings on a Rex pane, and copies the one you pick, like tmux-fingers.
//
// The program is in internal/cli. This file is the whole of package main,
// so that go install github.com/xeind/rex-fingers@TAG builds it.
package main

import "github.com/xeind/rex-fingers/internal/cli"

func main() { cli.Main() }
