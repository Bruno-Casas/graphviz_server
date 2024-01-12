package main

import (
	"os"

	"github.com/Bruno-Casas/graphviz_server/internal"
)

func main() {
	runAsWorker := len(os.Args) > 1 && os.Args[1] == "worker"

	if runAsWorker {
		internal.RunAsWorker()
	} else {
		internal.RunAsServer()
	}

}
