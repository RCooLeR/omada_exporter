package cmd

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

var descPattern = regexp.MustCompile(`^Desc\{fqName: "([^"]+)", help: "([^"]*)", constLabels: \{\}, variableLabels: [\{\[]([^}\]]*)[\}\]]\}$`)

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
			name, help, labels := parseDesc(description.String())
			fmt.Fprintf(os.Stdout, "| %s | %s | %s |\n", name, help, labels)
		} else {
			break
		}
	}
}

func parseDesc(desc string) (string, string, string) {
	matches := descPattern.FindStringSubmatch(desc)
	if len(matches) != 4 {
		return desc, "", ""
	}

	labels := strings.TrimSpace(matches[3])
	if labels == "" {
		labels = "-"
	}
	return matches[1], matches[2], labels
}
