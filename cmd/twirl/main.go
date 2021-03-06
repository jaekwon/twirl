package main

import (
	"os"
	"time"

	"github.com/codegangsta/cli"
	"github.com/jaekwon/twirl/node"
	cfg "github.com/tendermint/go-config"
	"github.com/tendermint/go-logger"
)

var (
	inputFlag = cli.StringFlag{
		Name:  "input",
		Value: "",
		Usage: "Path to file to share",
	}
	outputFlag = cli.StringFlag{
		Name:  "output",
		Value: "",
		Usage: "Output path for shared file",
	}
	seedsFlag = cli.StringFlag{
		Name:  "seeds",
		Value: "",
		Usage: "List of seeds to dial",
	}
	seedsLimitFlag = cli.StringFlag{
		Name:  "seeds-limit",
		Value: "",
		Usage: "Limit number of seeds to dial",
	}
)

func main() {
	app := cli.NewApp()
	app.Name = "twirl"
	app.Usage = "twirl [command] [args...]"
	app.Version = "0.0.0"
	app.Commands = []cli.Command{
		{
			Name:  "share",
			Usage: "Share a file",
			Action: func(c *cli.Context) error {
				cmdShare(c)
				return nil
			},
			Flags: []cli.Flag{inputFlag, outputFlag, seedsFlag, seedsLimitFlag},
		},
		{
			Name:  "shutdown",
			Usage: "Shutdown all seeds",
			Action: func(c *cli.Context) error {
				cmdShutdown(c)
				return nil
			},
			Flags: []cli.Flag{seedsFlag},
		},
	}
	app.Run(os.Args)
}

func getConfig(c *cli.Context) cfg.Config {
	config := cfg.NewMapConfig(nil)
	config.SetDefault("version", "0.0.0")
	config.SetDefault("network", "TWIRL")
	config.SetDefault("input", c.String("input"))
	config.SetDefault("output", c.String("output"))
	config.SetDefault("seeds", c.String("seeds"))
	config.SetDefault("seeds-limit", c.Int("seeds-limit"))
	config.SetDefault("node_laddr", "tcp://0.0.0.0:9999")
	config.SetDefault("skip_upnp", false)
	return config
}

func cmdShare(c *cli.Context) {
	logger.SetLogLevel("info") // TODO
	config := getConfig(c)
	node.RunNode(config, true)
}

func cmdShutdown(c *cli.Context) {
	logger.SetLogLevel("info") // TODO
	config := getConfig(c)
	config.Set("seeds-limit", 0) // don't use this for shutdown
	node := node.RunNode(config, false)
	time.Sleep(time.Second * 5)
	node.ShutdownPeers()
}
