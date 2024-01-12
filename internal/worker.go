package internal

/*
#cgo CFLAGS: -I/usr/include
#cgo LDFLAGS: -L/usr/lib -lgvc -lcgraph
#include <graphviz/gvc.h>
#include <graphviz/cgraph.h>
#include <stdlib.h>

void processDot(char* dot) {
	graph_t *g = agmemread(dot);
	if (g == NULL)
		return;

	GVC_t *gvc = gvContext();
	gvLayout(gvc, g, "dot");
	gvRender(gvc, g, "svg", NULL);

	gvFreeLayout(gvc, g);
	agclose(g);
	gvFreeContext(gvc);
}

*/
import "C"
import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"unsafe"
)

func RunAsWorker() {
	inReader := bufio.NewReader(os.Stdin)

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		os.Exit(0)
	}()

	processDot := func(goDot *string) {
		dot := C.CString(*goDot)

		C.processDot(dot)
		defer C.free(unsafe.Pointer(dot))

		fmt.Fprint(os.Stderr, "\x04")
		fmt.Print("\x04")
	}

	for {
		str, err := inReader.ReadString('\x04')
		if err != nil {
			fmt.Fprint(os.Stderr, "The processing worker was unable to get the dot", "\x04")
			fmt.Print("\x04")
		}

		str = strings.TrimSuffix(str, "\x04")
		processDot(&str)
		str = ""
	}
}
