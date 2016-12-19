package main

import (
	"os"

	"github.com/codegangsta/cli"
	"github.com/jaekwon/twirl/node"
	cfg "github.com/tendermint/go-config"
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
	}
	app.Run(os.Args)
}

func cmdShare(c *cli.Context) {
	config := cfg.NewMapConfig(nil)
	config.SetDefault("version", "0.0.0")
	config.SetDefault("network", "TWIRL")
	config.SetDefault("input", c.String("input"))
	config.SetDefault("output", c.String("output"))
	config.SetDefault("seeds", c.String("seeds"))
	config.SetDefault("seeds-limit", c.Int("seeds-limit"))
	config.SetDefault("node_laddr", "tcp://0.0.0.0:9999")
	config.SetDefault("skip_upnp", false)

	node.RunNode(config)
}
