package main

import (
	"fmt"
	"log"
	"os"

	"github.com/spacesailor24/node-brainer/clients"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "Node Brainer",
		Usage: "A terminal UI for managing ETH clients",
		Commands: []*cli.Command{
			{
				Name:  "download",
				Usage: "Downloads a specified ETH client",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "eth-client",
						Usage:    "Specifies what ETH client to download",
						Required: true,
					},
				},
				Action: download,
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func initClient(c *cli.Context) (clients.Client, error) {
	var client clients.Client
	var err error
	switch c.String("eth-client") {
	case "geth":
		client, err = clients.NewGethClient()
		if err != nil {
			return nil, err
		}
	case "lighthouse":
		client = clients.NewLighthouseClient()
	default:
		return nil, fmt.Errorf("unknown client: %s", c.String("eth-client"))
	}

	return client, nil
}

func download(c *cli.Context) error {
	client, err := initClient(c)
	if err != nil {
		return err
	}

	err = client.Download()
	if err != nil {
		return err
	}

	return nil
}
