package app

import (
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

var descPattern = regexp.MustCompile(`^Desc\{fqName: "([^"]+)", help: "([^"]*)", (unit: "[^"]*", )?constLabels: \{\}, variableLabels: [\{\[]([^}\]]*)[\}\]]\}$`)

// WriteMetricDocs writes all collector descriptors in stable metric-name order.
// It does not connect to a controller and reports output errors to its caller.
func WriteMetricDocs(writer io.Writer) error {
	descriptors := make(chan *prometheus.Desc)
	go func() {
		for _, collector := range initCollectors(nil) {
			collector.Describe(descriptors)
		}
		close(descriptors)
	}()

	// Drain descriptions before writing so an output error cannot strand the producer.
	var rows []string
	for descriptor := range descriptors {
		name, help, labels := parseDesc(descriptor.String())
		rows = append(rows, fmt.Sprintf("| %s | %s | %s |\n", name, help, labels))
	}
	slices.Sort(rows)
	if _, err := io.WriteString(writer, "| Name | Description | Labels |\n|--|--|--|\n"); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := io.WriteString(writer, row); err != nil {
			return err
		}
	}
	return nil
}

func parseDesc(desc string) (string, string, string) {
	matches := descPattern.FindStringSubmatch(desc)
	if len(matches) != 5 {
		return desc, "", ""
	}

	labels := strings.TrimSpace(matches[4])
	if labels == "" {
		labels = "-"
	}
	return matches[1], matches[2], labels
}
