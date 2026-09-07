// Command nokia_logger is the entrypoint. All actual behavior lives in the
// nokialogger library package; see that package's docs for details.
package main

import "nokia_logger/src"

func main() {
	nokialogger.Run()
}
