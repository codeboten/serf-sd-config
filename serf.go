package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/hashicorp/serf/client"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/documentation/examples/custom-sd/adapter"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	a             = kingpin.New("sd adapter usage", "Tool to generate file_sd target files for unimplemented SD mechanisms.")
	outputFile    = a.Flag("output.file", "Output file for file_sd compatible file.").Default("custom_sd.json").String()
	listenAddress = a.Flag("listen.address", "The address that Serf is listening on for requests.").Default("localhost:7373").String()
	logger        log.Logger
)

var (
	// DefaultSDConfig is the default Serf SD configuration.
	DefaultSDConfig = SDConfig{
		Address: "localhost:7373",
	}
)

// SDConfig is the configuration for serf service discovery.
type SDConfig struct {
	Address         string
	TagSeparator    string
	RefreshInterval int
}

// discovery retrieves target information from a serf cluster
type discovery struct {
	address         string
	refreshInterval int
	tagSeparator    string
	logger          log.Logger
	oldSourceList   map[string]bool
	config          SDConfig
	client          *client.RPCClient
}

func (d *discovery) parseMember(member client.Member) *targetgroup.Group {
	tgroup := targetgroup.Group{
		Labels:  model.LabelSet{},
		Targets: make([]model.LabelSet, 0, 1),
	}
	addr := net.JoinHostPort(fmt.Sprintf("%s", member.Addr), fmt.Sprintf("%d", member.Port))
	target := model.LabelSet{model.AddressLabel: model.LabelValue(addr)}
	tgroup.Targets = append(tgroup.Targets, target)
	return &tgroup
}

func (d *discovery) initialize(ctx context.Context) error {
	d.config = DefaultSDConfig
	conf := client.Config{Addr: d.config.Address}
	// connect our client
	cli, err := client.ClientFromConfig(&conf)
	if err != nil {
		return err
	}
	d.client = cli
	return nil
}

// Run connects to the serf cluster and updates the target group with the addresses of its
// members regularly
func (d *discovery) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {

	for c := time.Tick(time.Duration(d.refreshInterval) * time.Second); ; {
		err := d.initialize(ctx)
		if err != nil {
			level.Error(d.logger).Log("msg", "Error initializing serf client", "err", err)
			time.Sleep(time.Duration(d.refreshInterval) * time.Second)
			continue
		}

		members, err := d.client.Members()
		if err != nil {
			level.Error(d.logger).Log("msg", "Error getting members list", "err", err)
			time.Sleep(time.Duration(d.refreshInterval) * time.Second)
			continue
		}

		var tgs []*targetgroup.Group
		newSourceList := make(map[string]bool)
		for _, member := range members {
			tg := d.parseMember(member)
			tgs = append(tgs, tg)
			newSourceList[tg.Source] = true
		}
		// When targetGroup disappear, send an update with empty targetList.
		for key := range d.oldSourceList {
			if !newSourceList[key] {
				tgs = append(tgs, &targetgroup.Group{
					Source: key,
				})
			}
		}
		d.oldSourceList = newSourceList
		if err == nil {
			// We're returning all members as a single targetgroup.
			ch <- tgs
		}
		// Wait for ticker or exit when ctx is closed.
		select {
		case <-c:
			continue
		case <-ctx.Done():
			return
		}
	}
}

func newDiscovery(conf SDConfig) (*discovery, error) {
	cd := &discovery{
		address:         conf.Address,
		refreshInterval: conf.RefreshInterval,
		tagSeparator:    conf.TagSeparator,
		logger:          logger,
		oldSourceList:   make(map[string]bool),
	}
	return cd, nil
}

func main() {
	a.HelpFlag.Short('h')

	_, err := a.Parse(os.Args[1:])
	if err != nil {
		fmt.Println("err: ", err)
		return
	}
	logger = log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	logger = log.With(logger, "ts", log.DefaultTimestampUTC, "caller", log.DefaultCaller)

	ctx := context.Background()

	// NOTE: create an instance of your new SD implementation here.
	cfg := SDConfig{
		TagSeparator:    ",",
		Address:         *listenAddress,
		RefreshInterval: 30,
	}

	disc, err := newDiscovery(cfg)
	if err != nil {
		fmt.Println("err: ", err)
	}
	sdAdapter := adapter.NewAdapter(ctx, *outputFile, "serfSD", disc, logger)
	sdAdapter.Run()

	<-ctx.Done()
}
