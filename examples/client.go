package main

import (
	"flag"
	"fmt"

	"github.com/hypebeast/go-osc/osc"
)

func main() {
	p := flag.Int("p", 7746, "OSC server listening port.")
	a := flag.String("disa", "localhost", "OSC dis server listening address.")
	pp := flag.Int("disp", 7745, "OSC dis server listening port.")
	flag.Parse()

	d := osc.NewStandardDispatcher()
	d.AddMsgHandler("/di/next", func(msg *osc.Message) {
		osc.PrintMessage(msg)
	})
	server := &osc.Server{
		Addr:       fmt.Sprintf("localhost:%d", *p),
		Dispatcher: d,
	}

	// Send a message to the server to start receiving messages.
	client := osc.NewClient(*a, *pp)
	msg := osc.NewMessage("/di/stream")
	msg.Append("localhost")
	msg.Append(int32(*p))
	if err := client.Send(msg); err != nil {
		panic(err)
	}

	fmt.Printf("OSC server listening on %d\n", *p)
	server.ListenAndServe()
}
