package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

// mdocs just spits out the metrics descriptions and exits
func mdocs() {

	// Describe wants to return descriptions via a channel, so make and fill a channel.
	dc := make(chan *prometheus.Desc)
	go func() {
		// collectors can't Collect without a client, but Describe doesn't need one.
		for _, c := range initCollectors(nil) {
			c.Describe(dc)
		}
		close(dc)
	}()

	fmt.Fprintln(os.Stdout, "| Name | Description | Labels |\n|--|--|--|")

	// drain the channel
	for {
		if description := <-dc; description != nil {
			// Sure would be nice if the prometheus.Desc wasn't so opaque. This is gross and fragile.
			d := description.String()
			d = strings.Replace(d, `Desc{fqName: "`, "| ", 1)
			d = strings.Replace(d, `", help: "`, " | ", 1)
			d = strings.Replace(d, `", constLabels: {}, variableLabels: [`, " | ", 1)
			d = strings.Replace(d, `]}`, " | ", 1)
			fmt.Fprintln(os.Stdout, d)
		} else {
			break
		}
	}
}
