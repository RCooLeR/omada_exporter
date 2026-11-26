package app

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/RCooLeR/omada_exporter/internal/debugdump"
	"github.com/RCooLeR/omada_exporter/internal/model"
	"github.com/RCooLeR/omada_exporter/internal/omada"
	"github.com/RCooLeR/omada_exporter/internal/omada/session"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
)

// mspTargetIdentity names one scraped (customer, site) pair.
type mspTargetIdentity struct {
	CustomerID   string
	CustomerName string
	SiteID       string
	SiteName     string
}

func (id mspTargetIdentity) key() string {
	return id.CustomerID + "/" + id.SiteID
}

// customerLabels are added to every collector metric of the target. Collector
// metrics already carry site and site_id, so (customer_id, site_id) keeps the
// series of different targets apart.
func (id mspTargetIdentity) customerLabels() prometheus.Labels {
	return prometheus.Labels{"customer": id.CustomerName, "customer_id": id.CustomerID}
}

// targetLabels identify the target on exporter self-metrics, which have no
// site labels of their own.
func (id mspTargetIdentity) targetLabels() prometheus.Labels {
	return prometheus.Labels{"customer": id.CustomerName, "customer_id": id.CustomerID, "site": id.SiteName, "site_id": id.SiteID}
}

// mspTarget is one scraped customer site with its own API view and collectors.
type mspTarget struct {
	identity   mspTargetIdentity
	session    *session.Client
	collectors map[string]prometheus.Collector
	health     prometheus.Collector

	mu           sync.Mutex
	up           float64
	duration     float64
	lastAPIError float64
}

// mspExporter owns MSP discovery and the current target set. Targets are
// served through unchecked fan-out collectors, so adding or removing a target
// never touches the Prometheus registry.
type mspExporter struct {
	conf           *config.Config
	root           *session.Client
	api            *omada.Client
	collectorNames []string
	scrapeSlots    chan struct{}

	mu      sync.RWMutex
	targets map[string]*mspTarget

	discoveryMu          sync.Mutex // serializes discovery runs
	statsMu              sync.Mutex
	discoveryLastSuccess time.Time
	discoveryFailures    uint64
}

func newMSPExporter(conf *config.Config, root *session.Client) *mspExporter {
	concurrency := conf.MSPScrapeConcurrency
	if concurrency <= 0 {
		concurrency = config.DefaultMSPScrapeConcurrency
	}
	api := omada.NewClient(root)
	// Constructors only build descriptors; this enumerates which collectors a
	// target gets under the current configuration without contacting anything.
	names := slices.Sorted(maps.Keys(initCollectors(api)))
	return &mspExporter{
		conf:           conf,
		root:           root,
		api:            api,
		collectorNames: names,
		scrapeSlots:    make(chan struct{}, concurrency),
		targets:        map[string]*mspTarget{},
	}
}

func runMSP(ctx context.Context, conf *config.Config, root *session.Client) error {
	exporter := newMSPExporter(conf, root)
	if err := exporter.discover(); err != nil {
		return fmt.Errorf("MSP customer discovery failed: %w", err)
	}
	if conf.DumpResponsesDir != "" {
		if err := exporter.dumpResponses(ctx, conf.DumpResponsesDir); err != nil {
			return err
		}
		if conf.DumpResponsesOnly {
			return nil
		}
	}
	handler, health, err := newMSPExporterHandler(conf, exporter)
	if err != nil {
		return err
	}
	log.Info().Msg(fmt.Sprintf("listening on :%s (MSP mode)", conf.Port))
	server := newExporterHTTPServer(fmt.Sprintf(":%s", conf.Port), handler)
	// The discovery loop shares the background-task lifecycle of serve.
	return serve(ctx, server, health, exporter.runDiscoveryLoop)
}

// runDiscoveryLoop re-lists customers and sites until ctx is canceled.
func (e *mspExporter) runDiscoveryLoop(ctx context.Context) error {
	if e.conf.MSPDiscoveryInterval <= 0 {
		<-ctx.Done()
		return ctx.Err()
	}
	ticker := time.NewTicker(time.Duration(e.conf.MSPDiscoveryInterval) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := e.discover(); err != nil {
				log.Error().Err(err).Msg("MSP re-discovery failed; keeping the current targets")
			}
		}
	}
}

// discover lists customers and their sites, applies the filters and updates
// the target set. A failed customer listing keeps every current target; a
// failed site listing keeps the current targets of that customer only.
func (e *mspExporter) discover() error {
	e.discoveryMu.Lock()
	defer e.discoveryMu.Unlock()

	customers, err := e.api.MSPCustomers()
	if err != nil {
		e.recordDiscovery(false)
		return err
	}
	selected, unmatched := filterMSPCustomers(customers, e.conf.MSPCustomerFilter())
	for _, entry := range unmatched {
		log.Warn().Str("filter", entry).Msg("OMADA_MSP_CUSTOMERS entry matches no MSP customer")
	}

	siteFilter := e.conf.MSPSiteFilter()
	var found []mspTargetIdentity
	failedCustomers := map[string]bool{}
	for _, customer := range selected {
		sites, err := e.customerSites(customer.CustomerID)
		if err != nil {
			failedCustomers[customer.CustomerID] = true
			log.Error().Err(err).Str("customer", customer.CustomerName).Str("customer_id", customer.CustomerID).Msg("failed to list MSP customer sites; keeping its current targets")
			continue
		}
		matched := filterMSPSites(sites, siteFilter)
		if len(matched) == 0 {
			log.Info().Str("customer", customer.CustomerName).Int("sites", len(sites)).Msg("MSP customer has no site matching OMADA_SITE")
		}
		for _, site := range matched {
			found = append(found, mspTargetIdentity{
				CustomerID:   customer.CustomerID,
				CustomerName: customer.CustomerName,
				SiteID:       site.SiteID,
				SiteName:     site.Name,
			})
		}
	}

	added, removed, total, err := e.reconcile(found, failedCustomers)
	e.recordDiscovery(len(failedCustomers) == 0 && err == nil)
	logEvent := log.Info()
	if total == 0 {
		// Always visible: an exporter without targets is almost always a filter typo.
		logEvent = log.Log()
	}
	logEvent.Int("customers", len(customers)).Int("selected_customers", len(selected)).
		Int("targets", total).Int("added", added).Int("removed", removed).
		Msg("MSP discovery finished")
	if err != nil {
		return err
	}
	if len(failedCustomers) > 0 {
		return fmt.Errorf("site listing failed for %d of %d customers", len(failedCustomers), len(selected))
	}
	return nil
}

func (e *mspExporter) customerSites(customerID string) ([]model.MSPSite, error) {
	view, err := e.root.ForTarget(customerID, "", "")
	if err != nil {
		return nil, err
	}
	return omada.NewClient(view).CustomerSites()
}

func (e *mspExporter) recordDiscovery(success bool) {
	e.statsMu.Lock()
	defer e.statsMu.Unlock()
	if success {
		e.discoveryLastSuccess = time.Now()
		return
	}
	e.discoveryFailures++
}

func (e *mspExporter) reconcile(found []mspTargetIdentity, failedCustomers map[string]bool) (added, removed, total int, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	next := make(map[string]*mspTarget, len(found))
	var buildErrs []error
	for _, identity := range found {
		key := identity.key()
		if _, duplicate := next[key]; duplicate {
			continue
		}
		if current, ok := e.targets[key]; ok && current.identity == identity {
			next[key] = current
			continue
		}
		target, buildErr := e.newTarget(identity)
		if buildErr != nil {
			buildErrs = append(buildErrs, buildErr)
			continue
		}
		next[key] = target
		added++
		log.Info().Str("customer", identity.CustomerName).Str("customer_id", identity.CustomerID).
			Str("site", identity.SiteName).Str("site_id", identity.SiteID).Msg("MSP target added")
	}
	for key, target := range e.targets {
		if _, kept := next[key]; kept {
			continue
		}
		if failedCustomers[target.identity.CustomerID] {
			next[key] = target
			continue
		}
		removed++
		log.Info().Str("customer", target.identity.CustomerName).Str("customer_id", target.identity.CustomerID).
			Str("site", target.identity.SiteName).Str("site_id", target.identity.SiteID).Msg("MSP target removed")
	}
	e.targets = next
	return added, removed, len(next), errors.Join(buildErrs...)
}

func (e *mspExporter) newTarget(identity mspTargetIdentity) (*mspTarget, error) {
	view, err := e.root.ForTarget(identity.CustomerID, identity.SiteID, identity.SiteName)
	if err != nil {
		return nil, err
	}
	health := newCollectorHealth()
	collectors := map[string]prometheus.Collector{}
	for name, collector := range initCollectors(omada.NewClient(view)) {
		instrumented := newInstrumentedCollector(name, collector, health)
		collectors[name] = prometheus.WrapCollectorWith(identity.customerLabels(), instrumented)
	}
	return &mspTarget{
		identity:   identity,
		session:    view,
		collectors: collectors,
		health:     prometheus.WrapCollectorWith(identity.targetLabels(), health),
	}, nil
}

// snapshot returns the current targets in a stable order.
func (e *mspExporter) snapshot() []*mspTarget {
	e.mu.RLock()
	targets := slices.Collect(maps.Values(e.targets))
	e.mu.RUnlock()
	slices.SortFunc(targets, func(a, b *mspTarget) int { return strings.Compare(a.identity.key(), b.identity.key()) })
	return targets
}

// dumpResponses writes one diagnostic dump per target. Each target gets its
// own private subdirectory below dir; manifest.json names the customer ID
// (omadaCid), site and site ID. Failures are reported after all targets ran.
func (e *mspExporter) dumpResponses(ctx context.Context, dir string) error {
	var errs []error
	for _, target := range e.snapshot() {
		if err := debugdump.DumpResponses(ctx, target.session, dir); err != nil {
			errs = append(errs, fmt.Errorf("dump customer %q site %q: %w", target.identity.CustomerName, target.identity.SiteName, err))
		}
	}
	return errors.Join(errs...)
}

// filterMSPCustomers keeps customers whose ID matches exactly or whose name
// matches case-insensitively. It also returns filter entries that matched nothing.
func filterMSPCustomers(customers []model.MSPCustomer, filter []string) ([]model.MSPCustomer, []string) {
	if len(filter) == 0 {
		return customers, nil
	}
	used := make([]bool, len(filter))
	var selected []model.MSPCustomer
	for _, customer := range customers {
		matched := false
		for i, entry := range filter {
			if entry == customer.CustomerID || strings.EqualFold(entry, customer.CustomerName) {
				used[i] = true
				matched = true
			}
		}
		if matched {
			selected = append(selected, customer)
		}
	}
	var unmatched []string
	for i, entry := range filter {
		if !used[i] {
			unmatched = append(unmatched, entry)
		}
	}
	return selected, unmatched
}

// filterMSPSites keeps sites whose ID matches exactly or whose name matches
// case-insensitively. An empty filter keeps every site.
func filterMSPSites(sites []model.MSPSite, filter []string) []model.MSPSite {
	if len(filter) == 0 {
		return sites
	}
	var selected []model.MSPSite
	for _, site := range sites {
		for _, entry := range filter {
			if entry == site.SiteID || strings.EqualFold(entry, site.Name) {
				selected = append(selected, site)
				break
			}
		}
	}
	return selected
}
